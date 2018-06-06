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
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type VirtualEnv struct {
	Request *Request
	Script  string
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

// pythonModules is used to scan the pip installables and to groom them based upon a
// local distribution of studioML also being included inside the workspace
//
func pythonModules(rqst *Request, alloc *Allocated) (general []string, configured []string, studioML string) {

	hasGPU := alloc.GPU != nil && alloc.GPU.slots > 0

	general = []string{}

	gpuSeen := false
	for _, pkg := range rqst.Experiment.Pythonenv {
		if strings.HasPrefix(pkg, "studioml==") {
			studioML = pkg
			continue
		}
		// https://bugs.launchpad.net/ubuntu/+source/python-pip/+bug/1635463
		//
		// Groom out bogus package from ubuntu
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			gpuSeen = true
		}

		if hasGPU && !gpuSeen {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) == 0 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
				}
				fmt.Printf("modified tensorflow in general %+v \n", pkg)
			}
		}
		general = append(general, pkg)
	}

	configured = []string{}
	for _, pkg := range rqst.Config.Pip {
		if strings.HasPrefix(pkg, "studioml==") {
			studioML = pkg
			continue
		}
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			gpuSeen = true
		}
		if hasGPU && !gpuSeen {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) == 0 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
				}
				fmt.Printf("modified tensorflow in configured %+v \n", pkg)
			}
		}
		configured = append(configured, pkg)
	}

	return general, configured, studioML
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (p *VirtualEnv) Make(alloc *Allocated, e interface{}) (err errors.Error) {

	pips, cfgPips, studioPIP := pythonModules(p.Request, alloc)

	// If the studioPIP was specified but we have a dist directory then we need to clear the
	// studioPIP, otherwise leave it there
	pth, errGo := filepath.Abs(filepath.Join(path.Dir(p.Script), "..", "workspace", "dist", "studioml-*.tar.gz"))
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", pth)
	}
	matches, errGo := filepath.Glob(pth)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", pth)
	}
	if len(matches) != 0 {
		// Extract the most recent version of studioML from the dist directory
		sort.Strings(matches)
		studioPIP = matches[len(matches)-1]
	}

	params := struct {
		E         interface{}
		Pips      []string
		CfgPips   []string
		StudioPIP string
	}{
		E:         e,
		Pips:      pips,
		CfgPips:   cfgPips,
		StudioPIP: studioPIP,
	}

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("pythonRunner").Parse(
		`#!/bin/bash -x
export LC_ALL=en_US.utf8
locale
date
{
{{range $key, $value := .E.Request.Config.Env}}
export {{$key}}="{{$value}}"
{{end}}
{{range $key, $value := .E.ExprEnvs}}
export {{$key}}="{{$value}}"
{{end}}
} &> /dev/null
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/
mkdir {{.E.RootDir}}/blob-cache
mkdir {{.E.RootDir}}/queue
mkdir {{.E.RootDir}}/artifact-mappings
mkdir {{.E.RootDir}}/artifact-mappings/{{.E.Request.Experiment.Key}}
virtualenv -p /usr/bin/python2.7 .
source bin/activate
pip install pip==9.0.3 --force-reinstall
{{if .StudioPIP}}
pip install -I {{.StudioPIP}}
{{end}}
{{if .Pips}}
echo "installing project pips"
pip install {{range .Pips}} {{.}}{{end}}
echo "finished installing project pips"
{{end}}
pip install pyopenssl --upgrade
{{if .CfgPips}}
echo "installing cfg pips"
pip install {{range .CfgPips}} {{.}}{{end}}
echo "finished installing cfg pips"
{{end}}
export STUDIOML_EXPERIMENT={{.E.ExprSubDir}}
export STUDIOML_HOME={{.E.RootDir}}
cd {{.E.ExprDir}}/workspace
pip freeze
python {{.E.Request.Experiment.Filename}} {{range .E.Request.Experiment.Args}}{{.}} {{end}}
cd -
locale
deactivate
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

	if errGo = ioutil.WriteFile(p.Script, content.Bytes(), 0700); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", p.Script)
	}
	return nil
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts
//
func (p *VirtualEnv) Run(ctx context.Context, refresh map[string]Artifact) (err errors.Error) {

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

				msg := fmt.Sprintf("%s %s killed, maximum life time reached, or explicitly stopped", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key)
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
