// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of the python based virtualenv
// runtime for studioML workloads

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/resources"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	hostname string
)

func init() {
	hostname, _ = os.Hostname()
}

// VirtualEnv encapsulated the context that a python virtual environment is to be
// instantiated from including items such as the list of pip installables that should
// be loaded and shell script to run.
//
type VirtualEnv struct {
	Request   *request.Request
	Script    string
	workDir   string
	uniqueID  string
	venvID    string
	venvEntry *VirtualEnvEntry
	ResponseQ chan<- *runnerReports.Report
}

// NewVirtualEnv builds the VirtualEnv data structure from data received across the wire
// from a studioml client.
//
func NewVirtualEnv(rqst *request.Request, dir string, uniqueID string, responseQ chan<- *runnerReports.Report) (env *VirtualEnv, err kv.Error) {

	if errGo := os.MkdirAll(filepath.Join(dir, "_runner"), 0700); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return &VirtualEnv{
		Request:   rqst,
		Script:    filepath.Join(dir, "_runner", "runner.sh"),
		workDir:   dir,
		uniqueID:  uniqueID,
		ResponseQ: responseQ,
	}, nil
}

// gpuEnv is used to pull out of the allocated GPU roster the needed environment variables for running
// the python environment
func gpuEnv(alloc *resources.Allocated) (envs []string) {
	if len(alloc.GPU) != 0 {
		gpuSettings := map[string][]string{}
		for _, resource := range alloc.GPU {
			for k, v := range resource.Env {
				if k == "CUDA_VISIBLE_DEVICES" || k == "NVIDIA_VISIBLE_DEVICES" {
					if setting, isPresent := gpuSettings[k]; isPresent {
						gpuSettings[k] = append(setting, v)
					} else {
						gpuSettings[k] = []string{v}
					}
				} else {
					envs = append(envs, k+"="+v)
				}
			}
		}
		for k, v := range gpuSettings {
			envs = append(envs, k+"="+strings.Join(v, ","))
		}
	} else {
		// Force CUDA GPUs offline manually rather than leaving this undefined
		envs = append(envs, "CUDA_VISIBLE_DEVICES=\"-1\"")
		envs = append(envs, "NVIDIA_VISIBLE_DEVICES=\"-1\"")
	}
	return envs
}

// Make is used to write a script file that is generated for the specific TF tasks studioml has sent.
// It also receives Python virtual environment ID
// for environment to be used for running given evaluation task.
//
func (p *VirtualEnv) Make(ctx context.Context, alloc *resources.Allocated, e interface{}) (err kv.Error, evalDone bool) {

	// Get Python virtual environment ID:
	if p.venvEntry, err = virtEnvCache.getEntry(ctx, p.Request, alloc, p.workDir); err != nil {
		return err.With("stack", stack.Trace().TrimRuntime()).With("workDir", p.workDir), false
	}

	venvID, venvValid := p.venvEntry.addClient(p.uniqueID)
	p.venvID = venvID

	defer func() {
	    if err != nil {
	    	p.venvEntry.removeClient(p.uniqueID)
		}
	}()

	if !venvValid {
		err = kv.NewError("venv is invalid").With("venv", venvID, "stack", stack.Trace().TrimRuntime()).With("workDir", p.workDir)
		return err, true
	}

	// The tensorflow versions 1.5.x and above all support cuda 9 and 1.4.x is cuda 8,
	// c.f. https://www.tensorflow.org/install/install_sources#tested_source_configurations.
	// Insert the appropriate version explicitly into the LD_LIBRARY_PATH before other paths
	cudaDir := "/usr/local/cuda-10.0/lib64"

	params := struct {
		AllocEnv  []string
		E         interface{}
		VEnvID    string
		CudaDir   string
		Hostname  string
		Env       map[string]string
	}{
		AllocEnv:  []string{},
		E:         e,
		VEnvID:    p.venvID,
		CudaDir:   cudaDir,
		Hostname:  hostname,
		Env:       p.Request.Config.Env,
	}

	if alloc.CPU != nil {
		if alloc.CPU.Cores > 1 {
			params.AllocEnv = append(params.AllocEnv, "OPENMP=True")
			params.AllocEnv = append(params.AllocEnv, "MKL_NUM_THREADS="+strconv.Itoa(int(alloc.CPU.Cores)-1))
			params.AllocEnv = append(params.AllocEnv, "GOTO_NUM_THREADS="+strconv.Itoa(int(alloc.CPU.Cores)-1))
			params.AllocEnv = append(params.AllocEnv, "OMP_NUM_THREADS="+strconv.Itoa(int(alloc.CPU.Cores)-1))
		}
	}

	// Add GPU environment variables to the python process environment table
	params.AllocEnv = append(params.AllocEnv, gpuEnv(alloc)...)

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, errGo := template.New("pythonRunner").Parse(
		`#!/bin/bash -x
echo "{\"studioml\": {\"log\": [{\"ts\": \"` + "`" + `date -u -Ins` + "`" + `\", \"msg\":\"Init\"},{\"ts\":\"0\", \"msg\":\"\"}]}}" | jq -c '.'
sleep 2
# Credit https://github.com/fernandoacorreia/azure-docker-registry/blob/master/tools/scripts/create-registry-server
function fail {
  echo $1 >&2
  exit 1
}

trap 'fail "The execution was aborted because a command exited with an error status code."' ERR

function retry {
  local n=0
  local max=3
  local delay=10
  while true; do
    "$@" && break || {
      if [[ $n -lt $max ]]; then
        ((n++))
        echo "Command failed. Attempt $n/$max:"
        sleep $delay;
      else
        fail "The command has failed after $n attempts."
      fi
    }
  done
}

set -v
date
date -u
which python3
export LC_ALL=en_US.utf8
locale
hostname
set -e
echo "{\"studioml\": {\"load_time\": \"` + "`" + `date '+%FT%T.%N%:z'` + "`" + `\"}}" | jq -c '.'
echo "{\"studioml\": {\"host\": \"{{.Hostname}}\"}}" | jq -c '.'
echo "Using env"
{{if .Env}}
{{range $key, $value := .Env}}
export {{$key}}="{{$value}}"
{{end}}
{{end}}
echo "Done env"
export LD_LIBRARY_PATH={{.CudaDir}}:$LD_LIBRARY_PATH:/usr/local/cuda/lib64/:/usr/lib/x86_64-linux-gnu:/lib/x86_64-linux-gnu/
mkdir -p {{.E.RootDir}}/blob-cache
mkdir -p {{.E.RootDir}}/queue
mkdir -p {{.E.RootDir}}/artifact-mappings
mkdir -p {{.E.RootDir}}/artifact-mappings/{{.E.Request.Experiment.Key}}
export PATH=/runner/.pyenv/bin:$PATH
export PYENV_VERSION={{.E.Request.Experiment.PythonVer}}
IFS=$'\n'; arr=( $(pyenv versions --bare | grep -v venv-runner || true) )
for i in ${arr[@]} ; do
    if [[ "$i" == ${PYENV_VERSION}* ]]; then
		export PYENV_VERSION=$i
		echo $PYENV_VERSION
	fi
done
eval "$(pyenv init --path)"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
pyenv activate {{.VEnvID}}
set -e
export STUDIOML_EXPERIMENT={{.E.ExprSubDir}}
export STUDIOML_HOME={{.E.RootDir}}
{{if .AllocEnv}}
{{range .AllocEnv}}
export {{.}}
{{end}}
{{end}}
export
cd {{.E.ExprDir}}/workspace
python3 -m pip freeze
python3 -m pip -V
set -x
set -e
echo "{\"studioml\": { \"experiment\" : {\"key\": \"{{.E.Request.Experiment.Key}}\"}}}" | jq -c '.'
echo "{\"studioml\": { \"experiment\" : {\"project\": \"{{.E.Request.Experiment.Project}}\"}}}" | jq -c '.'
{{range $key, $value := .E.Request.Experiment.Artifacts}}
echo "{\"studioml\": { \"artifacts\" : {\"{{$key}}\": \"{{$value.Qualified}}\"}}}" | jq -c '.'
{{end}}
echo "{\"studioml\": {\"start_time\": \"` + "`" + `date '+%FT%T.%N%:z'` + "`" + `\"}}" | jq -c '.'
nvidia-smi 2>/dev/null || true
nvidia-smi -mig 1 || true
nvidia-smi  mig -i 0 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 1 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 2 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 3 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 4 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 5 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 6 -cgi 14,14,14 -C || true
nvidia-smi  mig -i 7 -cgi 14,14,14 -C || true
nvidia-smi 2>/dev/null || true
echo "[{\"op\": \"add\", \"path\": \"/studioml/log/-\", \"value\": {\"ts\": \"` + "`" + `date -u -Ins` + "`" + `\", \"msg\":\"Start\"}}]" | jq -c '.'
stdbuf -oL -eL python {{.E.Request.Experiment.Filename}} {{range .E.Request.Experiment.Args}}{{.}} {{end}}
result=$?
echo $result
set +e
echo "[{\"op\": \"add\", \"path\": \"/studioml/log/-\", \"value\": {\"ts\": \"` + "`" + `date -u -Ins` + "`" + `\", \"msg\":\"Stop\"}}]" | jq -c '.'
cd -
locale
pyenv deactivate || true
# pyenv virtualenv-delete -f studioml-{{.E.ExprSubDir}} || true
date
date -u
nvidia-smi 2>/dev/null || true
echo "{\"studioml\": {\"stop_time\": \"` + "`" + `date '+%FT%T.%N%:z'` + "`" + `\"}}" | jq -c '.'
exit $result
`)

	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()), false
	}

	content := new(bytes.Buffer)
	if errGo = tmpl.Execute(content, params); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()), false
	}

	if errGo = ioutil.WriteFile(p.Script, content.Bytes(), 0700); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", p.Script), false
	}
	return nil, false
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts.  Run is called by the processor
// runScript receiver.
//
func (p *VirtualEnv) Run(ctx context.Context, refresh map[string]request.Artifact) (err kv.Error) {

	// Prepare an output file into which the command line stdout and stderr will be written
	outputFN := filepath.Join(p.workDir, "output")
	if errGo := os.Mkdir(outputFN, 0600); errGo != nil {
		perr, ok := errGo.(*os.PathError)
		if ok {
			if !errors.Is(perr.Err, os.ErrExist) {
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	outputFN = filepath.Join(outputFN, "output")
	fOutput, errGo := os.Create(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer fOutput.Close()

	err = RunScript(ctx, p.Script, fOutput, p.ResponseQ, p.Request.Experiment.Key, p.uniqueID)
	p.venvEntry.removeClient(p.uniqueID)
	return err
}

// Close is used to close any resources which the encapsulated VirtualEnv may have consumed.
//
func (*VirtualEnv) Close() (err kv.Error) {
	return nil
}
