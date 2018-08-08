package runner

// This file contains the implementation of functions related to AWS.
//
// Especially functions related to the credentials file handling

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type AWSCred struct {
	Project string
	Region  string
	Creds   *credentials.Credentials
}

func AWSExtractCreds(filenames []string) (cred *AWSCred, err errors.Error) {

	cred = &AWSCred{
		Project: fmt.Sprintf("aws_%s", filepath.Base(filepath.Dir(filenames[0]))),
	}

	credsDone := false

	// AWS Does not read the region automatically from the config so lets read it here
	for _, aFile := range filenames {
		wasConfig := func() bool {
			f, err := os.Open(aFile)
			if err != nil {
				return false
			}
			if len(cred.Region) == 0 {
				scan := bufio.NewScanner(f)
				for scan.Scan() {
					line := scan.Text()
					if strings.HasPrefix(strings.ToLower(line), "region=") {
						tokens := strings.SplitN(line, "=", 2)
						cred.Region = tokens[1]
						return true
					}
				}
			}

			f.Close()
			return false
		}()

		if !credsDone && !wasConfig {
			cred.Creds = credentials.NewSharedCredentials(aFile, "default")
			credsDone = true
		}
	}

	if len(cred.Region) == 0 {
		return nil, errors.New("none of the supplied files defined a region").With("stack", stack.Trace().TrimRuntime()).With("files", filenames)
	}

	if !credsDone {
		return nil, errors.New("credentials never loaded").With("stack", stack.Trace().TrimRuntime()).With("files", filenames)
	}
	return cred, nil
}
