package main

// The file contains code for handling google certificates and
// refreshing a directory containing these certificates and using
// these to process work sent to pubsub queues that get forwarded
// to subscriptions made by the runner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"golang.org/x/net/context"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"

	"github.com/SentientTechnologies/studio-go-runner"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	jsonMatch = regexp.MustCompile(`\.json$`)
)

type googleCred struct {
	CredType string `json:"type"`
	Project  string `json:"project_id"`
}

func validateCred(ctx context.Context, filename string, scopes []string) (project string, err errors.Error) {

	b, errGo := ioutil.ReadFile(filename)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	cred := &googleCred{}
	if errGo = json.Unmarshal(b, cred); errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	if len(cred.Project) == 0 {
		return "", errors.New("bad file format for credentials").With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	client, errGo := pubsub.NewClient(ctx, cred.Project, option.WithCredentialsFile(filename))
	if errGo != nil {
		return "", errors.Wrap(errGo, "could not verify credentials").With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}
	client.Close()

	return cred.Project, nil
}

func refreshGoogleCerts(dir string) (found map[string]string) {

	found = map[string]string{}

	filepath.Walk(dir, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			if jsonMatch.MatchString(f.Name()) {
				// Check if this is a genuine credential
				ctx, cancel := context.WithTimeout(context.Background(), *pubsubTimeoutOpt)
				defer cancel()

				project, err := validateCred(ctx, path, []string{})
				if err != nil {
					logger.Warn(fmt.Sprintf("%#v", err))
					return nil
				}

				// If so include it
				found[project] = path
			}
		}
		return nil
	})
	return found
}

type Projects struct {
	projects map[string]chan bool
	sync.Mutex
}

func servicePubsub(quitC chan bool) {

	live := &Projects{projects: map[string]chan bool{}}

	// Place useful messages into the slack monitoring channel if available
	host := runner.GetHostName()

	// first time through make sure the credentials are checked immediately
	credCheck := time.Duration(time.Second)

	for {
		select {
		case <-quitC:

			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				close(quiter)
			}
			return

		case <-time.After(credCheck):
			credCheck = time.Duration(15 * time.Second)

			found := refreshGoogleCerts(*certDirOpt)

			// If projects have disappeared from the credentials then kill then from the
			// running set of projects if they are still running
			live.Lock()
			for proj, quiter := range live.projects {
				if _, isPresent := found[proj]; !isPresent {
					close(quiter)
					delete(live.projects, proj)
					logger.Info(fmt.Sprintf("credentials no longer available for %s", proj))
				}
			}
			live.Unlock()

			// Having checked for projects that have been dropped look for new projects
			for proj, cred := range found {
				live.Lock()
				if _, isPresent := live.projects[proj]; !isPresent {

					// Now start processing the queues that exist within the project in the background
					qr, err := NewQueuer(proj, cred)
					if err != nil {
						logger.Warn(fmt.Sprintf("%#v", err))
						live.Unlock()
						continue
					}
					quiter := make(chan bool)
					live.projects[proj] = quiter

					// Start the projects runner and let it go off and do its thing until it dies
					// for no longer has a matching credentials file
					go func() {
						msg := fmt.Sprintf("started project %s on %s", proj, host)
						logger.Info(msg)

						runner.InfoSlack("", msg, []string{})
						if err := qr.run(quiter); err != nil {
							runner.WarningSlack("", fmt.Sprintf("terminating project %s on %s due to %#v", proj, host, err), []string{})
						} else {
							runner.WarningSlack("", fmt.Sprintf("stopping project %s on %s", proj, host), []string{})
						}

						live.Lock()
						delete(live.projects, proj)
						live.Unlock()
					}()
				}
				live.Unlock()
			}
		}
	}
}
