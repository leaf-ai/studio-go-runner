package runner

// This file contains the implementation of an execution module for singularity
// within the studioML go runner
//

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type Singularity struct {
	Request    *Request
	BaseDir    string
	Definition string
}

func NewSingularity(rqst *Request, dir string) (*Singularity, errors.Error) {

	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return &Singularity{
		Request:    rqst,
		BaseDir:    dir,
		Definition: filepath.Join(dir, "_runner", "runner.def"),
	}, nil
}

// getImage will examine the contents of the _singularity directory looking for a single img.
// The function will return to the caller the absolute location for the img file.
//
func (p *Singularity) getImage(dir string) (fn string, err errors.Error) {

	errGo := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) (errGo error) {
		// return on directories do not descend
		if !fi.Mode().IsRegular() {
			return nil
		}
		if strings.HasSuffix(file, ".img") || strings.HasSuffix(file, ".simg") {
			if len(fn) != 0 {
				return errors.New("more than one .img and/or .simg images present").With("stack", stack.Trace().TrimRuntime())
			}
			fn = file
		}
		return nil
	})
	if errGo == nil {
		return fn, nil
	}

	return "", errGo.(errors.Error)
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (s *Singularity) Make(e interface{}) (err errors.Error) {

	// Locate the img
	img, err := s.getImage(s.BaseDir)
	if err != nil {
		return err
	}

	type param struct {
		E interface{}
		S *Singularity
		I string
	}
	params := &param{
		E: e,
		S: s,
		I: img,
	}

	// Take the original image and generate a new base image

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("singularityRunner").Parse(
		`Bootstrap: localimage
From: {{.I}}

%labels
ai.sentient.maintainer Karl Mutch
ai.sentient.version 0.0

%post
{{range $key, $value := .E.Request.Config.Env}}
    echo 'export {{$key}}="{{$value}}"' >> $SINGULARITY_ENVIRONMENT
{{end}}
{{range $key, $value := .E.ExprEnvs}}
    echo 'export {{$key}}="{{$value}}"' >> $SINGULARITY_ENVIRONMENT
{{end}}
    echo 'export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/' >> $SINGULARITY_ENVIRONMENT
	echo 'export STUDIOML_EXPERIMENT={{.E.ExprSubDir}}' >> $SINGULARITY_ENVIRONMENT
	echo 'export STUDIOML_HOME={{.E.RootDir}}' >> $SINGULARITY_ENVIRONMENT
	mkdir {{.E.RootDir}}/blob-cache
	mkdir {{.E.RootDir}}/queue
	mkdir {{.E.RootDir}}/artifact-mappings
	mkdir {{.E.RootDir}}/artifact-mappings/{{.E.Request.Experiment.Key}}
	{{range .E.Request.Experiment.Pythonenv}}
	pip install {{if ne . "studioml=="}}{{.}} {{end}}{{end}}
	pip install {{range .E..Request.Config.Pip}}{{.}} {{end}}
	if [ "` + "`" + `echo ../workspace/dist/studioml-*.tar.gz` + "`" + `" != "../workspace/dist/studioml-*.tar.gz" ]; then
		pip install ../workspace/dist/studioml-*.tar.gz
	else
		pip install studioml --upgrade
	fi
	pip install pyopenssl --upgrade
	pip freeze

%runscript
	cd {{.E.ExprDir}}/workspace
	python {{.E.Request.Experiment.Filename}} {{range .E.Request.Experiment.Args}}{{.}} {{end}}
	date
`)

	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	errGo = tmpl.Execute(content, params)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(s.Definition, content.Bytes(), 0700); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts
//
func (s *Singularity) Run(ctx context.Context, refresh map[string]Modeldir) (err errors.Error) {

	// Move to starting the process that we will monitor with the experiment running within
	// it
	//
	cmd := exec.Command("/bin/bash", "-c", s.Definition)
	cmd.Dir = path.Dir(s.Definition)

	stdout, errGo := cmd.StdoutPipe()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	stderr, errGo := cmd.StderrPipe()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	outC := make(chan []byte)
	defer close(outC)
	errC := make(chan string)
	defer close(errC)

	outputFN := filepath.Join(cmd.Dir, "..", "output", "output")
	f, errGo := os.Create(outputFN)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	stopCP := make(chan bool)

	go func(f *os.File, outC chan []byte, errC chan string, stopWriter chan bool) {
		defer f.Close()
		outLine := []byte{}

		refresh := time.NewTicker(2 * time.Second)
		defer refresh.Stop()

		for {
			select {
			case <-refresh.C:
				f.WriteString(string(outLine))
				outLine = []byte{}
			case <-stopWriter:
				f.WriteString(string(outLine))
				return
			case r := <-outC:
				outLine = append(outLine, r...)
				if !bytes.Contains([]byte{'\n'}, r) {
					continue
				}
				f.WriteString(string(outLine))
				outLine = []byte{}
			case errLine := <-errC:
				f.WriteString(errLine + "\n")
			}
		}
	}(f, outC, errC, stopCP)

	InfoSlack(s.Request.Config.Runner.SlackDest, fmt.Sprintf("logging %s", outputFN), []string{})

	if errGo = cmd.Start(); err != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	done := sync.WaitGroup{}
	done.Add(2)

	go func() {
		defer done.Done()
		time.Sleep(time.Second)
		s := bufio.NewScanner(stdout)
		s.Split(bufio.ScanRunes)
		for s.Scan() {
			outC <- s.Bytes()
		}
	}()

	go func() {
		defer done.Done()
		time.Sleep(time.Second)
		s := bufio.NewScanner(stderr)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			errC <- s.Text()
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				if errGo := cmd.Process.Kill(); errGo != nil {
					msg := fmt.Sprintf("%s %s could not be killed, maximum life time reached, due to %v", s.Request.Config.Database.ProjectId, s.Request.Experiment.Key, errGo)
					WarningSlack(s.Request.Config.Runner.SlackDest, msg, []string{})
					return
				}

				msg := fmt.Sprintf("%s %s killed, maximum life time reached", s.Request.Config.Database.ProjectId, s.Request.Experiment.Key)
				WarningSlack(s.Request.Config.Runner.SlackDest, msg, []string{})
				return
			case <-stopCP:
				return
			}
		}
	}()

	done.Wait()
	close(stopCP)

	if errGo = cmd.Wait(); err != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func (*Singularity) Close() (err errors.Error) {
	return nil
}
