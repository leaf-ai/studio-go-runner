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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type Singularity struct {
	Request   *Request
	BaseDir   string
	BaseImage string
}

func NewSingularity(rqst *Request, dir string) (sing *Singularity, err errors.Error) {

	sing = &Singularity{
		Request: rqst,
		BaseDir: dir,
	}

	art, isPresent := rqst.Experiment.Artifacts["_singularity"]
	if !isPresent {
		return nil, errors.New("_singularity artifact is missing").With("stack", stack.Trace().TrimRuntime())
	}

	// Look for the singularity artifact and extract the base image name
	// that will be used from shub://sentient-singularity
	//
	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	os.MkdirAll(filepath.Join(dir, "..", "blob-cache"), 0700)
	os.MkdirAll(filepath.Join(dir, "..", "queue"), 0700)
	os.MkdirAll(filepath.Join(dir, "..", "artifact-mappings", rqst.Experiment.Key), 0700)

	sing.BaseImage = art.Qualified
	switch {
	case strings.HasPrefix(art.Qualified, "shub://sentient-singularity/"):
	case strings.HasPrefix(art.Qualified, "dockerhub://tensorflow/"):
	default:
		return nil, errors.New("untrusted image specified").With("stack", stack.Trace().TrimRuntime()).With("artifact", art)
	}
	return sing, nil
}

func (s *Singularity) makeDef(alloc *Allocated, e interface{}) (fn string, err errors.Error) {

	// Extract all of the python variables into two collections with the studioML extracted out
	// Ignore the tensorflow version as the container is responsible for cuda
	pips, cfgPips, studioPIP, _ := pythonModules(s.Request, alloc)

	// If the studioPIP was specified but we have a dist directory then we need to clear the
	// studioPIP, otherwise leave it there
	pth, errGo := filepath.Abs(filepath.Join(s.BaseDir, "workspace", "dist", "studioml-*.tar.gz"))
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	matches, _ := filepath.Glob(pth)
	if len(matches) != 0 {
		// Extract the most recent version of studioML from the dist directory
		sort.Strings(matches)
		studioPIP = matches[len(matches)-1]
	}

	params := struct {
		E         interface{}
		S         *Singularity
		I         string
		Dir       string
		Pips      []string
		CfgPips   []string
		StudioPIP string
		ImgType   string
	}{
		E:         e,
		S:         s,
		I:         s.BaseImage,
		Dir:       filepath.Join(s.BaseDir, "_runner"),
		Pips:      pips,
		CfgPips:   cfgPips,
		StudioPIP: studioPIP,
	}

	switch {
	case strings.HasPrefix(params.I, "shub://singularity-hub/sentient-singularity"):
		params.ImgType = "debootstrap"
	case strings.HasPrefix(params.I, "dockerhub://tensorflow/"):
		params.ImgType = "docker"
		params.I = strings.Replace(params.I, "dockerhub://", "", 1)
	}

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("singularityRunner").Parse(
		`Bootstrap: {{.ImgType}}
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
	pip install virtualenv
	virtualenv {{.Dir}}
	chmod +x {{.Dir}}/bin/activate
	{{.Dir}}/bin/activate
	pip freeze
	{{if .StudioPIP}}
	pip install -I {{.StudioPIP}}
	{{end}}
	{{if .Pips}}
	pip install -I {{range .Pips}} {{.}}{{end}}
	{{end}}
	pip install pyopenssl --upgrade
	{{if .CfgPips}}
	pip install {{range .CfgPips}} {{.}}{{end}}
	{{end}}
	pip freeze

%runscript
	{{.Dir}}/bin/activate
	cd {{.E.ExprDir}}/workspace
	python {{.E.Request.Experiment.Filename}} {{range .E.Request.Experiment.Args}}{{.}} {{end}}
	date
`)

	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	errGo = tmpl.Execute(content, params)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	fn = filepath.Join(s.BaseDir, "_runner", "Singularity.def")
	if errGo = ioutil.WriteFile(fn, content.Bytes(), 0600); errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return fn, nil
}

func (s *Singularity) makeBuildScript(e interface{}) (fn string, err errors.Error) {

	fn = filepath.Join(s.BaseDir, "_runner", "build.sh")

	params := struct {
		Dir       string
		BaseImage string
	}{
		Dir:       filepath.Join(s.BaseDir, "_runner"),
		BaseImage: s.BaseImage,
	}

	tmpl, errGo := template.New("singularityRunner").Parse(
		`#!/bin/bash -x
sudo singularity build {{.Dir}}/runner.img {{.Dir}}/Singularity.def
`)

	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	errGo = tmpl.Execute(content, params)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo := ioutil.WriteFile(fn, content.Bytes(), 0700); errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return fn, nil
}

func (s *Singularity) runBuildScript(script string) (err errors.Error) {

	ctx := context.Background()
	outputFN := filepath.Join(s.BaseDir, "output", "output")

	// Move to starting the process that we will monitor with the experiment running within
	// it
	//

	reporterC := make(chan *string)
	defer close(reporterC)

	go func() {
		for {
			select {
			case msg := <-reporterC:
				if msg == nil {
					return
				}
			}
		}
	}()

	return runWait(ctx, script, filepath.Join(s.BaseDir, "_runner"), outputFN, reporterC)
}

func (s *Singularity) makeExecScript(e interface{}) (fn string, err errors.Error) {

	fn = filepath.Join(s.BaseDir, "_runner", "exec.sh")

	params := struct {
		Dir string
	}{
		Dir: filepath.Join(s.BaseDir, "_runner"),
	}

	tmpl, errGo := template.New("singularityRunner").Parse(
		`#!/bin/bash -x
singularity run --home {{.Dir}} -B /tmp:/tmp -B /usr/local/cuda:/usr/local/cuda -B /usr/lib/nvidia-384:/usr/lib/nvidia-384 --nv {{.Dir}}/runner.img
`)

	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	errGo = tmpl.Execute(content, params)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo := ioutil.WriteFile(fn, content.Bytes(), 0700); errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return fn, nil
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (s *Singularity) Make(alloc *Allocated, e interface{}) (err errors.Error) {

	_, err = s.makeDef(alloc, e)
	if err != nil {
		return err
	}

	script, err := s.makeBuildScript(e)
	if err != nil {
		return err
	}

	if err = s.runBuildScript(script); err != nil {
		return err
	}

	if _, err = s.makeExecScript(e); err != nil {
		return err
	}

	return nil
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts
//
func (s *Singularity) Run(ctx context.Context, refresh map[string]Artifact) (err errors.Error) {

	outputFN := filepath.Join(s.BaseDir, "output", "output")
	script := filepath.Join(s.BaseDir, "_runner", "exec.sh")

	reporterC := make(chan *string)
	defer close(reporterC)

	go func() {
		for {
			select {
			case msg := <-reporterC:
				if msg == nil {
					return
				}
			}
		}
	}()

	return runWait(ctx, script, filepath.Join(s.BaseDir, "_runner"), outputFN, reporterC)
}

func runWait(ctx context.Context, script string, dir string, outputFN string, errorC chan *string) (err errors.Error) {

	stopCP := make(chan struct{})
	// defers are stacked in LIFO order so closing this channel is the last
	// thing this function will do
	defer close(stopCP)

	// Move to starting the process that we will monitor with the experiment running within
	// it
	//
	cmd := exec.Command("/bin/bash", "-c", script)
	cmd.Dir = dir

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

	f, errGo := os.Create(outputFN)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("outputFN", outputFN)
	}

	go procOutput(f, outC, errC, stopCP)

	if errGo = cmd.Start(); err != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	waitOnIO := sync.WaitGroup{}
	waitOnIO.Add(2)

	go func() {
		defer waitOnIO.Done()
		time.Sleep(time.Second)
		s := bufio.NewScanner(stdout)
		s.Split(bufio.ScanRunes)
		for s.Scan() {
			outC <- s.Bytes()
		}
		if errGo := s.Err(); errGo != nil {
			if err != nil {
				err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	go func() {
		defer waitOnIO.Done()
		time.Sleep(time.Second)
		s := bufio.NewScanner(stderr)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			errC <- s.Text()
		}
		if errGo := s.Err(); errGo != nil {
			if err != nil {
				err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				if errGo := cmd.Process.Kill(); errGo != nil {
					msg := fmt.Sprintf("could not be killed, maximum life time reached, due to %v", errGo)
					select {
					case errorC <- &msg:
					default:
					}
					return
				}
				msg := "killed, maximum life time reached"
				select {
				case errorC <- &msg:
				default:
				}
				return
			case <-stopCP:
				return
			}
		}
	}()

	if errGo = cmd.Wait(); err != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	waitOnIO.Wait()

	if err == nil && ctx.Err() != nil {
		err = errors.Wrap(ctx.Err()).With("stack", stack.Trace().TrimRuntime())
	}

	return err
}

func (*Singularity) Close() (err errors.Error) {
	return nil
}
