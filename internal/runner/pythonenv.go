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

// VirtualEnv encapsulated the context that a python virtual environment is to be
// instantiated from including items such as the list of pip installables that should
// be loaded and shell script to run.
//
type VirtualEnv struct {
	Request *Request
	Script  string
}

// NewVirtualEnv builds the VirtualEnv data structure from data received across the wire
// from a studioml client.
//
func NewVirtualEnv(rqst *Request, dir string) (env *VirtualEnv, err errors.Error) {

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
func pythonModules(rqst *Request, alloc *Allocated) (general []string, configured []string, studioML string, tfVer string) {

	hasGPU := len(alloc.GPU) != 0

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

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVer = spec[1]
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

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVer = spec[1]
				}
				fmt.Printf("modified tensorflow in configured %+v \n", pkg)
			}
		}
		configured = append(configured, pkg)
	}

	return general, configured, studioML, tfVer
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (p *VirtualEnv) Make(alloc *Allocated, e interface{}) (err errors.Error) {

	pips, cfgPips, studioPIP, tfVer := pythonModules(p.Request, alloc)

	// The tensorflow versions 1.5.x and above all support cuda 9 and 1.4.x is cuda 8,
	// c.f. https://www.tensorflow.org/install/install_sources#tested_source_configurations.
	// Insert the appropriate version explicitly into the LD_LIBRARY_PATH before other paths
	cudaDir := "/usr/local/cuda-9.0/lib64"
	if strings.HasPrefix(tfVer, "1.4") {
		cudaDir = "/usr/local/cuda-8.0/lib64"
	}

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
		CudaDir   string
	}{
		E:         e,
		Pips:      pips,
		CfgPips:   cfgPips,
		StudioPIP: studioPIP,
		CudaDir:   cudaDir,
	}

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("pythonRunner").Parse(
		`#!/bin/bash -x
set -v
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
export LD_LIBRARY_PATH={{.CudaDir}}:$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/
mkdir {{.E.RootDir}}/blob-cache
mkdir {{.E.RootDir}}/queue
mkdir {{.E.RootDir}}/artifact-mappings
mkdir {{.E.RootDir}}/artifact-mappings/{{.E.Request.Experiment.Key}}
virtualenv -p ` + "`" + `which python{{.E.Request.Experiment.PythonVer}}` + "`" + ` .
source bin/activate
pip install pip==9.0.3 --force-reinstall
{{if .StudioPIP}}
pip install -I {{.StudioPIP}}
{{end}}
{{if .Pips}}
{{range .Pips}}
echo "installing project pip {{.}}"
pip install {{.}}
{{end}}
{{end}}
echo "finished installing project pips"
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

	// Create a new TMPDIR because the python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", p.Request.Experiment.Key)
	if errGo != nil {
		return errors.Wrap(errGo).With("experimentKey", p.Request.Experiment.Key).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(tmpDir)

	// Move to starting the process that we will monitor with the experiment running within
	// it
	//
	cmd := exec.Command("/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+p.Script)
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

	// From this point errors will be placed into the return parameter, err
	// and kept until processing ceases when it will be returned
	errMutex := sync.Mutex{}

	go func() {
		for {
			select {
			case <-ctx.Done():
				if errGo := cmd.Process.Kill(); errGo != nil {
					errMutex.Lock()
					defer errMutex.Unlock()
					err = errors.Wrap(errGo, "could not kill process after maximum lifetime reached").
						With("project_id", p.Request.Config.Database.ProjectId, "experiment_key", p.Request.Experiment.Key).
						With("stack", stack.Trace().TrimRuntime())
					return
				}

				errMutex.Lock()
				defer errMutex.Unlock()
				err = errors.New("process killed, or maximum lifetime reached").
					With("project_id", p.Request.Config.Database.ProjectId, "experiment_key", p.Request.Experiment.Key).
					With("stack", stack.Trace().TrimRuntime())
				return
			case <-stopCP:
				return
			}
		}
	}()

	done.Wait()
	close(stopCP)

	if errGo = cmd.Wait(); errGo != nil {
		errMutex.Lock()
		if err != nil {
			err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		errMutex.Unlock()
	}

	return err
}

// Close is used to close any resources which the encapsulated VirtualEnv may have consumed.
//
func (ve *VirtualEnv) Close() (err errors.Error) {
	return nil
}
