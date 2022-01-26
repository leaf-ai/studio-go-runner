// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of the python based virtualenv
// runtime cache for studioML workloads

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/leaf-ai/go-service/pkg/log"
	"hash/fnv"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/resources"

	"github.com/karlmutch/go-shortid"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

const (
	entryReady int = iota
	entryGenerating
	entryInvalid
	entryStale
)

var (
	virtEnvCache VirtualEnvCache
)

type VirtualEnvEntry struct {
	uniqueID   string
	status     int
	created    time.Time
	lastUsed   time.Time
	numUsed    int
	numClients int
	host       *VirtualEnvCache
	sync.Mutex
}

type VirtualEnvCache struct {
	entries map[string] *VirtualEnvEntry
	logger              *log.Logger
	rootDir             string
	maxUnusedPeriod     time.Duration
	sync.Mutex
}

func init() {
	logger := log.NewLogger("venvcache")
	rootDir, errGo := ioutil.TempDir("", "venvcache")
	if errGo != nil {
		logger.Error("FAILED to create root directory for venvcache. Using '.'")
		rootDir = "."
	}
	logger.Info("Root directory for VEnv cache", "path:", rootDir)
	virtEnvCache = VirtualEnvCache{
		entries:         map[string]*VirtualEnvEntry{},
		logger:          logger,
		rootDir:         rootDir,
		maxUnusedPeriod: time.Duration(2) * time.Hour,
	}
}

func (entry *VirtualEnvEntry) create(ctx context.Context, rqst *request.Request, general []string, configured []string, expDir string) (err kv.Error) {
	// This venv entry is already locked:
	defer entry.Unlock()

	entry.status = entryInvalid
	entry.uniqueID, err = entry.host.getVirtEnvID()
	if err != nil {
	    return err
	}

	scriptPath := filepath.Join(entry.host.rootDir, fmt.Sprintf("genvenv-%s.sh", entry.uniqueID))
	if err = entry.host.generateScript(rqst.Config.Env, rqst.Experiment.PythonVer, general, configured, entry.uniqueID, scriptPath); err != nil {
		return err
	}

	// Script to build virtual environment is generated, let's run it:
	// Prepare an output file into which the command line stdout and stderr will be written
	outputFN := filepath.Join(expDir, "output")
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
	outputFN = filepath.Join(outputFN, "outputPEnv")
	fOutput, errGo := os.Create(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer fOutput.Close()

	if err = RunScript(ctx, scriptPath, fOutput, nil, entry.uniqueID, entry.uniqueID); err != nil {
		return err.With("script", scriptPath).With("stack", stack.Trace().TrimRuntime())
	}

	entry.numClients = -1
	entry.touch()
	entry.status = entryReady
	return nil
}

func (entry *VirtualEnvEntry) delete(ctx context.Context) (err kv.Error) {
	entry.status = entryInvalid

	scriptPath := filepath.Join(entry.host.rootDir, fmt.Sprintf("rmvenv-%s.sh", entry.uniqueID))
	if err = entry.host.generateRemoveScript(entry.uniqueID, scriptPath); err != nil {
		return err
	}

	// Script to delete virtual environment is generated, let's run it:
	// Prepare an output file into which the command line stdout and stderr will be written
	outputFN := filepath.Join(entry.host.rootDir, "rmvenv")
	if errGo := os.Mkdir(outputFN, 0700); errGo != nil {
		perr, ok := errGo.(*os.PathError)
		if ok {
			if !errors.Is(perr.Err, os.ErrExist) {
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	outputFN = filepath.Join(outputFN, fmt.Sprintf("output-%s", entry.uniqueID))
	fOutput, errGo := os.Create(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer fOutput.Close()

	if err = RunScript(ctx, scriptPath, fOutput, nil, entry.uniqueID, entry.uniqueID); err != nil {
		return err.With("script", scriptPath).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func (entry *VirtualEnvEntry) touch() {
	entry.lastUsed = time.Now()
}

func (entry *VirtualEnvEntry) toString() string {
	return fmt.Sprintf("id: %s created: %v last used: %v clients: %d used: %d",
		       entry.uniqueID, entry.created, entry.lastUsed, entry.numClients, entry.numUsed)
}

func (entry *VirtualEnvEntry) addClient(clientID string) (envID string, valid bool) {
	entry.Lock()
	defer entry.Unlock()

	if entry.status == entryReady {
		if entry.numClients < 0 {
			entry.numClients = 1
		} else {
			entry.numClients++
		}
		entry.numUsed++
		entry.touch()
		entry.host.logger.Info("Added client", "client:", clientID, "venv:", entry.uniqueID)
		return entry.uniqueID, true
	}
	entry.host.logger.Info("Attempt to add client to invalid venv", "client:", clientID, "venv:", entry.uniqueID)
	return entry.uniqueID, false
}

func (entry *VirtualEnvEntry) removeClient(clientID string) {
	entry.Lock()
	defer entry.Unlock()

	if entry.status == entryReady {
		if entry.numClients > 0 {
			entry.numClients--
		}
		entry.host.logger.Info("Removed client", "client:", clientID, "venv:", entry.uniqueID)
		return
	}
	entry.host.logger.Info("Attempt to remove client from invalid venv", "client:", clientID, "venv:", entry.uniqueID)
}

func (cache *VirtualEnvCache) getEntry(ctx context.Context,
	                                   rqst *request.Request,
	                                   alloc *resources.Allocated, expDir string) (entry *VirtualEnvEntry, err kv.Error) {
	// Get request dependencies
	general, configured, _ := pythonModules(rqst, alloc)

	// Unique ID (hash) for virtual environment we need:
	hashEnv := getHashPythonEnv(rqst.Experiment.PythonVer, general, configured)

	cache.Lock()
	defer cache.Unlock()

	if entry, isPresent := cache.entries[hashEnv]; isPresent {
		cache.logger.Info("Found virtual env: reused", "envID: ", entry.uniqueID)
		entry.touch()
		return entry, nil
	}

	// Construct new venv object:
	newEntry := &VirtualEnvEntry{
		uniqueID: "none",
		status:   entryGenerating,
		created:  time.Now(),
		numUsed:  0,
		host:     cache,
	}
	newEntry.Lock()
	cache.entries[hashEnv] = newEntry

	go newEntry.create(ctx, rqst, general, configured, expDir)

	return newEntry, nil
}

func ServiceVirtualEnvCache(ctx context.Context) {
	virtEnvCache.cleaner(ctx)
}

func (cache *VirtualEnvCache) cleaner(ctx context.Context) {
	checkpoint := time.NewTicker(time.Duration(5) * time.Minute)
	defer checkpoint.Stop()

	for {
		select {
		case <-checkpoint.C:
            cache.cleanupUnused(ctx)

		case <-ctx.Done():
			// If higher-level context is done, just quietly stop and exit.
			return
		}
	}
}

func (cache *VirtualEnvCache) cleanupUnused(ctx context.Context) {
	cache.Lock()
	defer cache.Unlock()

	for key, entry := range cache.entries {
		entry.Lock()
		cache.logger.Debug("VEnv cache entry:", "key: ", key, "value: ", entry.toString())
		// Has this entry become stale?
		last := entry.lastUsed
		if entry.numClients == 0 && last.Add(cache.maxUnusedPeriod).Before(time.Now()) {
            delete(cache.entries, key)
		    cache.logger.Debug("Deleting stale cache entry:", "id: ", entry.uniqueID)
            if err := entry.delete(ctx); err != nil {
            	cache.logger.Info("failed to delete stale VEnv", "err:", err.Error(), "venv:", entry.uniqueID)
			}
		}
		entry.Unlock()
	}
}

func (cache *VirtualEnvCache) generateScript(workEnv map[string]string, pythonVer string, general []string, configured []string,
	                                         envName string, scriptPath string) (err kv.Error) {

	params := struct {
		PythonVer  string
		EnvName    string
		Pips       []string
		CfgPips    []string
		Env        map[string]string
	}{
		PythonVer:  pythonVer,
		EnvName:    envName,
		Pips:       general,
		CfgPips:    configured,
		Env:        workEnv,
	}

	// Create a shell script that will do everything needed
	// to create required virtual python environment
	tmpl, errGo := template.New("virtEnvCreator").Parse(
		`#!/bin/bash -x
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
export LC_ALL=en_US.utf8
locale
hostname
set -e
export PATH=/runner/.pyenv/bin:$PATH
{{if .Env}}
{{range $key, $value := .Env}}
export {{$key}}="{{$value}}"
{{end}}
{{end}}
echo "Done env"
export PYENV_VERSION={{.PythonVer}}
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
pyenv doctor
pyenv virtualenv-delete -f {{.EnvName}} || true
pyenv virtualenv $PYENV_VERSION {{.EnvName}}
pyenv activate {{.EnvName}}  
set +e
retry python3 -m pip install "pip==21.3.1" "setuptools==59.2.0" "wheel==0.37.0"
python3 -m pip freeze --all
{{if .Pips}}
echo "installing project pip {{ .Pips }}"
retry python3 -m pip install {{range .Pips }} {{.}}{{end}}
{{end}}
echo "finished installing project pips"
retry python3 -m pip install pyopenssl==20.0.1 pipdeptree==2.0.0
{{if .CfgPips}}
echo "installing cfg pips"
retry python3 -m pip install {{range .CfgPips}} {{.}}{{end}}
echo "finished installing cfg pips"
{{end}}
set -e
python3 -m pip freeze
python3 -m pip -V
set -x
cd - || true
locale
pyenv deactivate || true
date
date -u
exit 0
`)

	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	if errGo = tmpl.Execute(content, params); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(scriptPath, content.Bytes(), 0700); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", scriptPath)
	}
	return nil
}

func (cache *VirtualEnvCache) generateRemoveScript(envName string, scriptPath string) (err kv.Error) {

	params := struct {
		EnvName    string
	}{
		EnvName:    envName,
	}

	// Create a shell script that will do everything needed
	// to delete specified virtual python environment
	tmpl, errGo := template.New("virtEnvDeleter").Parse(
		`#!/bin/bash -x
sleep 2
set -e
export PATH=/runner/.pyenv/bin:$PATH
eval "$(pyenv init --path)"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
pyenv virtualenv-delete -f {{.EnvName}} || true
date -u
exit 0
`)

	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	content := new(bytes.Buffer)
	if errGo = tmpl.Execute(content, params); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(scriptPath, content.Bytes(), 0700); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("script", scriptPath)
	}
	return nil
}

func (cache *VirtualEnvCache) getVirtEnvID() (id string, err kv.Error) {
	sid, errGo := shortid.Generate()
	if errGo != nil {
		return "", kv.Wrap(errGo, "venv id generation failed").With("stack", stack.Trace().TrimRuntime())
	}
	return fmt.Sprintf("venv-runner-%s", sid), nil
}

// pythonModules is used to scan the pip installables
//
func pythonModules(rqst *request.Request, alloc *resources.Allocated) (general []string, configured []string, tfVer string) {
	hasGPU := len(alloc.GPU) != 0
	gpuSeen := false

	general, tfVer, gpuSeen = scanPythonModules(rqst.Experiment.Pythonenv, hasGPU, gpuSeen, "general")
	configured, tfVer, gpuSeen = scanPythonModules(rqst.Config.Pip, hasGPU, gpuSeen, "configured")

	return general, configured, tfVer
}

func scanPythonModules(pipList []string, hasGPU bool, gpuSeen bool, name string) (result []string, tfVersion string, sawGPU bool) {
	result = []string{}
	sawGPU = gpuSeen
	for _, pkg := range pipList {
		// https://bugs.launchpad.net/ubuntu/+source/python-pip/+bug/1635463
		//
		// Groom out bogus package from ubuntu
		if strings.HasPrefix(pkg, "pkg-resources") {
			continue
		}
		if strings.HasPrefix(pkg, "tensorflow_gpu") {
			sawGPU = true
		}

		if hasGPU && !sawGPU {
			if strings.HasPrefix(pkg, "tensorflow==") || pkg == "tensorflow" {
				spec := strings.Split(pkg, "==")

				if len(spec) < 2 {
					pkg = "tensorflow_gpu"
				} else {
					pkg = "tensorflow_gpu==" + spec[1]
					tfVersion = spec[1]
				}
				fmt.Printf("modified tensorflow in %s %+v \n", name, pkg)
			}
		}
		result = append(result, pkg)
	}
	return result, tfVersion, sawGPU
}

func getHashPythonEnv(pythonVer string, general []string, configured []string) string {
	hasher := fnv.New64()
	hasher.Reset()

	hasher.Write([]byte(pythonVer))
	for _, elem := range general {
		hasher.Write([]byte(elem))
	}
	for _, elem := range configured {
		hasher.Write([]byte(elem))
	}
	return strconv.FormatUint(hasher.Sum64(), 10)
}

