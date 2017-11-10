package runner

// This file contains the implementation of the python based virtualenv
// runtime for studioML workloads

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
	"sync"
	"text/template"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type VirtualEnv struct {
	Script  string
	Request *Request
}

func NewVirtualEnv(rqst *Request, dir string) (*VirtualEnv, errors.Error) {

	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return &VirtualEnv{
		Request: rqst,
		Script:  filepath.Join(dir, "_runner", "runner.sh"),
	}, nil
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (p *VirtualEnv) Make(e interface{}) (err errors.Error) {

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("pythonRunner").Parse(
		`#!/bin/bash -x
date
{
{{range $key, $value := .Request.Config.Env}}
export {{$key}}="{{$value}}"
{{end}}
{{range $key, $value := .ExprEnvs}}
export {{$key}}="{{$value}}"
{{end}}
} &> /dev/null
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/
mkdir {{.RootDir}}/blob-cache
mkdir {{.RootDir}}/queue
mkdir {{.RootDir}}/artifact-mappings
mkdir {{.RootDir}}/artifact-mappings/{{.Request.Experiment.Key}}
virtualenv --system-site-packages -p /usr/bin/python2.7 .
source bin/activate
{{range .Request.Experiment.Pythonenv}}
pip install {{if ne . "studioml=="}}{{.}} {{end}}{{end}}
pip install {{range .Request.Config.Pip}}{{.}} {{end}}
if [ "` + "`" + `echo ../workspace/dist/studioml-*.tar.gz` + "`" + `" != "../workspace/dist/studioml-*.tar.gz" ]; then
    pip install ../workspace/dist/studioml-*.tar.gz
else
    pip install studioml --upgrade
fi
pip install pyopenssl --upgrade
export STUDIOML_EXPERIMENT={{.ExprSubDir}}
export STUDIOML_HOME={{.RootDir}}
cd {{.ExprDir}}/workspace
pip freeze
python {{.Request.Experiment.Filename}} {{range .Request.Experiment.Args}}{{.}} {{end}}
cd -
deactivate
date
`)

	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	errGo = tmpl.Execute(content, e)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(p.Script, content.Bytes(), 0744); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts
//
func (p *VirtualEnv) Run(ctx context.Context, refresh map[string]Modeldir) (err errors.Error) {

	// Move to starting the process that we will monitor with the experiment running within
	// it
	//
	cmd := exec.Command("/bin/bash", "-c", p.Script)
	cmd.Dir = path.Dir(p.Script)

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

	InfoSlack(p.Request.Config.Runner.SlackDest, fmt.Sprintf("logging %s", outputFN), []string{})

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
					msg := fmt.Sprintf("%s %s could not be killed, maximum life time reached, due to %v", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, errGo)
					WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
					return
				}

				msg := fmt.Sprintf("%s %s killed, maximum life time reached", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key)
				WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
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

func (ve *VirtualEnv) Close() (err errors.Error) {
	return nil
}
