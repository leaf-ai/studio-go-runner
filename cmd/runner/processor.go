// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/valyala/fastjson"
	"golang.org/x/crypto/ssh"

	farm "github.com/dgryski/go-farm"
	humanize "github.com/dustin/go-humanize"
	shortid "github.com/leaf-ai/studio-go-runner/pkg/go-shortid"
	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	pkgResources "github.com/leaf-ai/studio-go-runner/internal/resources"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/task"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

type processor struct {
	Group      string            `json:"group"` // A caller specific grouping for work that can share sensitive resources
	RootDir    string            `json:"root_dir"`
	ExprDir    string            `json:"expr_dir"`
	ExprSubDir string            `json:"expr_sub_dir"`
	ExprEnvs   map[string]string `json:"expr_envs"`
	Request    *request.Request  `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
	QueueCreds string            `json:"credentials_file"`
	Artifacts  *runner.ArtifactCache
	Executor   Executor
	status     chan string // Used by the processor to get notifications about external status changes
	// for currently executed workload
	AccessionID string      // A unique identifier for this task
	ResponseQ   chan string // A response queue the runner can employ to send progress updates on
	evalDone    bool        // true, if evaluation should be processed as completed
}

type tempSafe struct {
	dir string
	sync.Mutex
}

var (
	// Used to store machine resource prfile
	machineResources = &pkgResources.Resources{}

	// tempRoot is used to store information about the root directory uses by the
	// runner
	tempRoot = tempSafe{}

	// A shared cache for all projects exists that is used by processors
	artifactCache = runner.NewArtifactCache()

	// Used to initialize a logger sink into which the artifact caching code in the runner
	// can send error messages for the application to determine what action is taken with
	// caching kv.that might be short lived
	cacheReport sync.Once

	// Guards against multiple threads of processing claiming a single directory
	guardExprDir sync.Mutex

	statusArtifactName = "_results"
	statusFileName     = "status.json"
)

const (
	// ExecUnknown is an unused guard value
	ExecUnknown = iota
	// ExecPythonVEnv indicates we are using the python virtualenv packaging
	ExecPythonVEnv

	fmtAddLog = `[{"op": "add", "path": "/studioml/log/-", "value": {"ts": "%s", "msg":"%s"}}]`
)

func init() {
	res, err := pkgResources.NewResources(*tempOpt)
	if err != nil {
		logger.Fatal("could not initialize disk space tracking", "err", err.Error())
	}
	machineResources = res

	// A cache exists on linux for cuda lets remove it as it
	// can cause issues
	errGo := os.RemoveAll("$HOME/.nv")
	if errGo != nil {
		logger.Fatal("could not clear the $HOME/.nv cache", "err", errGo.Error())
	}
}

func cacheReporter(ctx context.Context) {
	for {
		select {
		case err := <-artifactCache.ErrorC:
			logger.Info("artifact cache error", "error", err, "stack", stack.Trace().TrimRuntime())
		case <-ctx.Done():
			return
		}
	}
}

// Executor is an interface that defines a job handling worker implementation.  Each variant of a worker
// conforms to a standard processor interface
//
type Executor interface {

	// Make is used to allow a script to be generated for the specific run strategy being used
	Make(ctx context.Context, alloc *pkgResources.Allocated, e interface{}) (err kv.Error, evalDone bool)

	// Run will execute the worker task used by the experiment
	Run(ctx context.Context, refresh map[string]request.Artifact) (err kv.Error)

	// Close can be used to tidy up after an experiment has completed
	Close() (err kv.Error)
}

// Singleton style initialization to instantiate and overridding directory
// for the entire server working area
//
func makeCWD() (temp string, err kv.Error) {
	tempRoot.Lock()
	defer tempRoot.Unlock()

	if tempRoot.dir == "" {
		id, errGo := shortid.Generate()
		if errGo != nil {
			return "", kv.Wrap(errGo, "temp file id generation failed").With("stack", stack.Trace().TrimRuntime())
		}
		if tempRoot.dir, errGo = ioutil.TempDir(*tempOpt, "gorun_"+id); errGo != nil {
			return "", kv.Wrap(errGo, "temp file create failed").With("stack", stack.Trace().TrimRuntime())
		}
	}
	return tempRoot.dir, nil

}

// newProcessor will parse the inbound message and then validate that there are
// sufficient resources to run an experiment and then create a new working directory.
//
func newProcessor(ctx context.Context, qt *task.QueueTask, accessionID string) (proc *processor, hardError bool, err kv.Error) {

	// When a processor is initialized make sure that the logger is enabled first time through
	//
	cacheReport.Do(func() {
		go cacheReporter(ctx)
	})

	temp, err := makeCWD()
	if err != nil {
		return nil, false, err
	}

	// Processors share the same root directory and use acccession numbers on the experiment key
	// to avoid collisions
	//
	proc = &processor{
		RootDir:     temp,
		Group:       qt.Subscription,
		QueueCreds:  qt.Credentials[:],
		AccessionID: accessionID,
		ResponseQ:   qt.ResponseQ,
		evalDone:    false,
	}

	// Extract processor information from the message received on the wire, includes decryption etc
	if hardError, err = proc.unpackMsg(qt); hardError == true || err != nil {
		return proc, hardError, err
	}

	// Recheck the alloc using the encrypted resource description
	if _, err = proc.allocate(false); err != nil {
		return proc, false, err
	}

	if _, err = proc.mkUniqDir(); err != nil {
		return proc, false, err
	}

	// Determine the type of execution that is needed for this job by
	// inspecting the artifacts specified
	//
	mode := ExecUnknown
	for group := range proc.Request.Experiment.Artifacts {
		if len(group) == 0 {
			continue
		}
		switch group {
		case "workspace":
			if mode == ExecUnknown {
				mode = ExecPythonVEnv
			}
		}
	}

	switch mode {
	case ExecPythonVEnv:
		if proc.Executor, err = runner.NewVirtualEnv(proc.Request, proc.ExprDir, proc.AccessionID, logger); err != nil {
			return nil, true, err
		}
	default:
		return nil, true, kv.NewError("unable to determine execution class from artifacts").With("stack", stack.Trace().TrimRuntime()).
			With("mode", mode, "project", proc.Request.Config.Database.ProjectId).With("experiment", proc.Request.Experiment.Key)
	}
	return proc, false, nil
}

// unpackMsg will use the message payload inside the queueTask (qt) and transform it into a payload
// inside the processor, handling any validation and decryption needed
//
func (proc *processor) unpackMsg(qt *task.QueueTask) (hardError bool, err kv.Error) {

	// Check to see if we have an encrypted or signed request
	if isEnvelope, _ := defense.IsEnvelope(qt.Msg); isEnvelope {

		if qt.Wrapper == nil {
			return false, kv.NewError("encrypted msg support not enabled").With("stack", stack.Trace().TrimRuntime())
		}

		// First load in the clear text portion of the message and test its resource request
		// against available resources before decryption
		envelope, err := defense.UnmarshalEnvelope(qt.Msg)
		if err != nil {
			return true, err
		}
		if _, err = allocResource(&envelope.Message.Resource, "", false); err != nil {
			return false, err
		}

		if len(envelope.Message.Signature) == 0 {
			return false, kv.NewError("encrypted payload has no signature").With("stack", stack.Trace().TrimRuntime())
		}

		if len(envelope.Message.Fingerprint) == 0 {
			return false, kv.NewError("payload signature has no fingerprint").With("stack", stack.Trace().TrimRuntime())
		}

		// Now check the signature by getting the queue name and then looking for the applicable
		// public key inside the signature store
		pubKey, fp, err := GetRqstSigs().SelectSSH(qt.ShortQName)
		if err != nil {
			return false, err
		}
		if fp != envelope.Message.Fingerprint {
			logger.Info("payload signature has an unmatched fingerprint", "fingerprint", fp, "message.Fingerprint", envelope.Message.Fingerprint)
		}

		sigBin, errGo := base64.StdEncoding.DecodeString(envelope.Message.Signature)
		if errGo != nil {
			return false, kv.Wrap(errGo).With("signature", envelope.Message.Signature).With("stack", stack.Trace().TrimRuntime())
		}

		err = nil
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = kv.Wrap(r.(error)).With("stack", stack.Trace().TrimRuntime())
				}
			}()

			// First try for the RFC format using the parser
			sig, errSig := defense.ParseSSHSignature(sigBin)
			if errSig != nil {
				// We could have 64 byte blob so just try to use that
				if len(sigBin) == 64 {
					sig = &ssh.Signature{
						Format: "ssh-ed25519",
						Blob:   sigBin,
					}
				} else {
					err = errSig
					return
				}
			}
			if err == nil {
				if errGo := pubKey.Verify([]byte(envelope.Message.Payload), sig); errGo != nil {
					err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
				}
			}
		}()
		if err != nil {
			return false, err
		}

		// Decrypt, using the wrapper, the master request structure and assign it to our task
		if proc.Request, err = qt.Wrapper.Request(envelope); err != nil {
			return true, err
		}

	} else {
		if !*acceptClearTextOpt {
			return true, kv.NewError("unencrypted messages not enabled").With("stack", stack.Trace().TrimRuntime())
		}
		// restore the msg into the processing data structure from the JSON queue payload
		if proc.Request, err = request.UnmarshalRequest(qt.Msg); err != nil {
			return true, err
		}
	}
	return hardError, nil
}

// Close will release all resources and clean up the work directory that
// was used by the studioml work
//
func (p *processor) Close() (err error) {
	if *debugOpt || 0 == len(p.ExprDir) {
		return nil
	}

	return os.RemoveAll(p.ExprDir)
}

// fetchAll is used to retrieve from the storage system employed by studioml any and all available
// artifacts and to unpack them into the experiment directory. fetchAll is called by the deployAndRun
// receiver.

// This function will try to constrain artifacts fetched to the disk size specified by the requested
// disk space that was defined in the experiments resource request.  This is used tp defang
// zip bombs to some extent and tries to prevent disk space exhaustion.
//
func (p *processor) fetchAll(ctx context.Context) (err kv.Error) {

	tm := time.Now()
	logger.Info(fmt.Sprintf("fetchAll start: exp: %s\n", p.Request.Experiment.Key))

	defer func() {
		tmnow := time.Now()
		logger.Info(fmt.Sprintf("fetchAll end: exp: %s == %v millisec\n", p.Request.Experiment.Key, tmnow.Sub(tm).Milliseconds()))
	}()

	diskBytes, errGo := humanize.ParseBytes(p.Request.Experiment.Resource.Hdd)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	diskBudget := int64(diskBytes)

	for group, artifact := range p.Request.Experiment.Artifacts {

		// Artifacts that have no qualified location will be ignored
		if 0 == len(artifact.Qualified) {
			continue
		}

		// Extract all available artifacts into subdirectories of the main experiment directory.
		//
		// The current convention is that the archives include the directory name under which
		// the files are unpacked in their table of contents
		//
		size, warns, err := artifactCache.Fetch(ctx, artifact.Clone(), p.Request.Config.Database.ProjectId, group, diskBudget, p.ExprEnvs, p.ExprDir)
		diskBudget -= size

		if diskBudget < 0 {
			err = kv.NewError("disk budget exhausted")
		}

		if err != nil {
			msg := "artifact fetch failed"
			msgDetail := []interface{}{
				"group", group,
				"Experiment", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime(),
				"err", err,
			}
			if artifact.Mutable {
				logger.Debug(msg, msgDetail)
			} else {
				logger.Warn(msg, msgDetail)
			}
			msgDetail[len(msgDetail)-2] = "warning"
			for _, warn := range warns {
				msgDetail[len(msgDetail)-1] = warn
				if artifact.Mutable {
					logger.Debug(msg, msgDetail)
				} else {
					logger.Warn(msg, msgDetail)
				}
			}

			// Mutable artifacts can be create-only items that don't yet exist on the storage platform
			if !artifact.Mutable {
				return err.With(msgDetail...)
			}
		}
	}
	return nil
}

func jsonEscape(unescaped string) (escaped string, errGo error) {
	b, errGo := json.Marshal(unescaped)
	if errGo != nil {
		return escaped, errGo
	}
	escaped = string(b)
	return escaped[1 : len(escaped)-1], nil
}

// copyToMetaData is used to copy a file to the meta data area using the file naming semantics
// of the metadata layout
func (p *processor) copyToMetaData(src string, jsonDest string) (err kv.Error) {

	if !*generateMetaData {
		logger.Debug("meta-data scraping skipped", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
		return nil
	}

	logger.Debug("copying start", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	defer logger.Debug("copying done", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())

	fStat, errGo := os.Stat(src)
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}

	if !fStat.Mode().IsRegular() {
		return kv.NewError("not a regular file").With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}

	source, errGo := os.Open(filepath.Clean(src))
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	defer source.Close()

	// If there is no need to scan the file look for json data to scrape from it
	// simply copy the file and return
	if len(jsonDest) == 0 {
		return kv.NewError("the json destination is missing").With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}

	// If we need to scrape the file then we should scan it line by line
	jsonDestination, errGo := os.OpenFile(jsonDest, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		jsonDestination.Close()

		// Uploading a zero length json file is pointless as we do have a record of
		// the presence of an experiment left by the metadata file
		if fileInfo, errGo := os.Stat(jsonDest); errGo == nil {
			if fileInfo.Size() == 0 {
				_ = os.Remove(jsonDest)
				logger.Debug("removing empty scrape file", "scrape_file", jsonDest)
			}
		}
	}()

	// Store any discovered json fragments for generating experiment documents as a single collection
	jsonDirectives := []string{}
	autoCapture := *captureOutputMD

	// Checkmarx code checking note. Checkmarx is for Web applications and is not a good fit general purpose server code.
	// It is also worth mentioning that if you are reading this message that Checkmarx does not understand Go package structure
	// and does not appear to use the Go AST to validate code so is not able to perform path and escape analysis which
	// means that more than 95% of warning are for unvisited code.
	//
	// The following will raise a 'Denial Of Service Resource Exhaustion' message but this is bogus.
	// The scanner in go is space limited intentially to prevent resource exhaustion.
	logger.Debug("scrape start", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	s := bufio.NewScanner(source)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if len(line) <= 2 {
			continue
		}

		// See if this line is a sensible json fragment
		if !((line[0] == '{' && line[len(line)-1] == '}') ||
			(line[0] == '[' && line[len(line)-1] == ']')) {
			// If we dont have a fragment we check to see if in the line should be formatted as
			// json and inserted using a command line switch
			if !autoCapture {
				continue
			}
			line, errGo = jsonEscape(line)
			if errGo != nil {
				logger.Trace("output json filter failed", "error", errGo, "line", line, "stack", stack.Trace().TrimRuntime())
				continue
			}
			line = fmt.Sprintf(fmtAddLog, time.Now().UTC().Format("2006-01-02T15:04:05.999999999-0700"), line)
		}

		// After each line is scanned the json fragment is merged into a collection of all detected patches and merges that
		// have been output by the experiment
		if errGo = fastjson.Validate(line); errGo != nil {
			if logger.IsTrace() {
				logger.Trace("output json filter failed", "error", errGo, "line", line, "stack", stack.Trace().TrimRuntime())
			}
			continue
		}
		jsonDirectives = append(jsonDirectives, line)
		if logger.IsTrace() {
			logger.Trace("json filter added", "line", line, "stack", stack.Trace().TrimRuntime())
		}
	}
	logger.Debug("scrape stop", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())

	if len(jsonDirectives) == 0 {
		logger.Debug("no json directives found", "stack", stack.Trace().TrimRuntime())
		return nil
	}

	// Zero copy prepend
	jsonDirectives = append(jsonDirectives, " ")
	copy(jsonDirectives[1:], jsonDirectives[0:])
	jsonDirectives[0] = `{"studioml": {"log": [{"ts": "0", "msg":"Init"},{"ts":"1", "msg":""}]}}`

	logger.Debug("JSONEditor start", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	defer logger.Debug("JSONEditor end", "source", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	result, err := runner.JSONEditor("", jsonDirectives)
	if err != nil {
		return err
	}

	if _, errGo = fmt.Fprintln(jsonDestination, result); errGo != nil {
		return kv.Wrap(errGo).With("src", src, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// updateMetaData is used to update files and artifacts related to the experiment
// that reside in the meta data area
//
func (p *processor) updateMetaData(group string, artifact request.Artifact, accessionID string) (err kv.Error) {

	metaDir := filepath.Join(p.ExprDir, "_metadata")
	if _, errGo := os.Stat(metaDir); os.IsNotExist(errGo) {
		os.MkdirAll(metaDir, 0700)
	}

	switch group {
	case "output":
		src := filepath.Join(p.ExprDir, "output", "output")
		jsonDest := filepath.Join(metaDir, "scrape-host-"+accessionID+".json")
		return p.copyToMetaData(src, jsonDest)
	default:
		return kv.NewError("group unrecognized").With("group", group, "stack", stack.Trace().TrimRuntime())
	}
}

func visError(err error) (result string) {
	if err != nil {
		return err.Error()
	}
	return "none"
}

// returnOne is used to upload a single artifact to the data store specified by the experimenter
//
func (p *processor) returnOne(ctx context.Context, group string, artifact request.Artifact, accessionID string) (uploaded bool, warns []kv.Error, err kv.Error) {

	// Meta data is specialized
	if len(accessionID) != 0 {
		switch group {
		case "output":
			if err = p.updateMetaData(group, artifact, accessionID); err != nil {
				logger.Warn("output artifact could not be used for metadata", "project_id", p.Request.Config.Database.ProjectId,
					"Experiment_id", p.Request.Experiment.Key, "error", err.Error())
			}
		}
	}

	if isEmpty, errGo := p.artifactIsEmpty(group); isEmpty {
		logger.Debug("upload skipped (empty)", "group", group,
			"experiment_id", p.Request.Experiment.Key, "file", filepath.Join(p.ExprDir, group), "error", visError(errGo))
		return false, warns, nil
	}

	//logger.Debug("uploading artifact", "experiment_id", p.Request.Experiment.Key, "file", filepath.Join(p.ExprDir, group))
	defer logger.Debug("upload artifact done", "group", group, "experiment_id", p.Request.Experiment.Key, "file", filepath.Join(p.ExprDir, group))
	return artifactCache.Restore(ctx, &artifact, p.Request.Config.Database.ProjectId, group, p.ExprEnvs, p.ExprDir)
}

func (p *processor) artifactIsEmpty(group string) (result bool, err error) {
	//logger.Debug("checking artifact is empty:", "group", group)
	//defer logger.Debug("checked artifact is empty:", "group", group, "result", result, "err", visError(err))

	artDir := filepath.Join(p.ExprDir, group)
	if _, errGo := os.Stat(artDir); errGo != nil {
		return true, errGo
	}
	artRoot, errGo := os.Open(artDir)
	if errGo != nil {
		return true, errGo
	}
	defer artRoot.Close()

	listInfo, errGo := artRoot.Readdir(-1)
	return errGo != nil || len(listInfo) == 0, errGo
}

type resultArtifact struct {
	Name string `json:"name"`
}

type resultArtifacts struct {
	ExitMsg      string                    `json:"exit_msg"`
	ExperimentID string                    `json:"experiment_id"`
	Host         string                    `json:"host"`
	Artifacts    map[string]resultArtifact `json:"artifacts"`
}

func (p *processor) uploadResultArtifact(ctx context.Context, results *resultArtifacts, accessionID string) (err kv.Error) {
	if resArtifact, isPresent := p.Request.Experiment.Artifacts[statusArtifactName]; isPresent {
		// Write out local file with returned artifacts info
		localDir := filepath.Join(p.ExprDir, statusArtifactName)
		if errGo := os.MkdirAll(localDir, 0700); errGo != nil {
			return kv.Wrap(errGo).With("dir", localDir).With("stack", stack.Trace().TrimRuntime())
		}
		buf, errGo := json.MarshalIndent(results, "", "  ")
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		localFN := filepath.Join(localDir, statusArtifactName+".json")
		f, errGo := os.OpenFile(localFN, os.O_CREATE|os.O_WRONLY, 0600)
		if errGo != nil {
			return kv.Wrap(errGo).With("file", localFN).With("stack", stack.Trace().TrimRuntime())
		}
		defer f.Close()
		_, errGo = f.Write(buf)
		if errGo != nil {
			return kv.Wrap(errGo).With("file", localFN).With("stack", stack.Trace().TrimRuntime())
		}
		resArtifact.Local = localFN
		if _, _, err := p.returnOne(ctx, statusArtifactName, resArtifact, accessionID); err != nil {
			return err.With("local", localFN).With("remote", resArtifact.Qualified).With("stack", stack.Trace().TrimRuntime())
		}
		return nil
	}
	return kv.NewError("FAILED to find status artifact").With("name", statusArtifactName).With("stack", stack.Trace().TrimRuntime())
}

// returnAll creates tar archives of the experiments artifacts and then puts them
// back to the studioml shared storage
//
func (p *processor) returnAll(ctx context.Context, accessionID string, err kv.Error) (rerr kv.Error) {

	logger.Debug("returnAll called", "stack", stack.Trace().TrimRuntime())

	returned := make([]string, 0, len(p.Request.Experiment.Artifacts))

	// Accessioning can modify the system artifacts and so the order we traverse
	// is important, we want the _metadata artifact after the _output
	// artifact which can be done using a descending sort which places underscores
	// before lowercase letters
	//
	keys := make([]string, 0, len(p.Request.Experiment.Artifacts))
	for group := range p.Request.Experiment.Artifacts {
		if group != statusArtifactName {
			keys = append(keys, group)
		}
	}
	sort.Strings(keys)

	// Data structure to capture status of final experiment artifacts returned to client.
	exitMsg := "ok"
	if err != nil {
		exitMsg = err.Error()
	}

	finalArtStatus := resultArtifacts{}
	finalArtStatus.ExitMsg = exitMsg
	finalArtStatus.ExperimentID = p.Request.Experiment.Key
	finalArtStatus.Host, _ = os.Hostname()
	finalArtStatus.Artifacts = make(map[string]resultArtifact)

	for _, group := range keys {
		if artifact, isPresent := p.Request.Experiment.Artifacts[group]; isPresent && artifact.Mutable {
			uploaded, warns, err := p.returnOne(ctx, group, artifact, accessionID)
			if err != nil {
				logger.Info("returnAll error", "group", group, "error", err.Error())
			} else {
				if uploaded {
					returned = append(returned, group)
				}
				if isEmpty, _ := p.artifactIsEmpty(group); !isEmpty && group != "_metadata" {
					finalArtStatus.Artifacts[group] = resultArtifact{Name: group}
				}
			}
			for _, warn := range warns {
				logger.Debug("returnAll warning", "group", group, "warning", warn.Error())
			}
		}
	}

	if len(returned) != 0 {
		logger.Info("project returned", "result", strings.Join(returned, ", "))
	}

	if err == nil || p.evalDone {
		logger.Debug("GENERATING results artifact")
		if errRes := p.uploadResultArtifact(ctx, &finalArtStatus, accessionID); errRes != nil {
			// If we failed to upload final results artifact,
			// this workload will not be detected as completed by a client,
			// so we will try to resubmit the job:
			p.evalDone = false
			logger.Error("Failed to upload results artifact - eval not done", errRes.Error())
			return errRes
		}
	} else {
		logger.Error("NOT GENERATING results artifact", "error: ", err.Error())
	}
	return nil
}

// allocate is used to reserve the resources on the local host needed to handle the entire job as
// a high water mark.
//
// The returned alloc structure should be used with the deallocate function otherwise resource
// leaks will occur.
//
func (p *processor) allocate(liveRun bool) (alloc *pkgResources.Allocated, err kv.Error) {
	return allocResource(&p.Request.Experiment.Resource, p.Request.Experiment.Key, liveRun)
}

// deallocate first releases resources and then triggers a ready channel to notify any listener that the
func (p *processor) deallocate(alloc *pkgResources.Allocated, id string) {
	deallocResource(alloc, id)
}

// Process is the main function where experiment processing occurs.
//
// This function is invoked by the cmd/runner/handle.go:HandleMsg function and blocks.
//
func (p *processor) Process(ctx context.Context) (ack bool, err kv.Error) {

	// Call the allocation function to get access to resources and get back
	// the allocation we received
	alloc, err := p.allocate(true)
	if err != nil {
		return false, kv.Wrap(err, "allocation failed").With("stack", stack.Trace().TrimRuntime())
	}

	// Setup a function to release resources that have been allocated and
	// use a panic handler to catch issues related to, or unrelated to the runner
	//
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("panic", "panic", fmt.Sprintf("%#+v", r), "stack", string(debug.Stack()))

			if err != nil {
				// Modify the return values to include details about the panic, but be sure not to
				// obscure earlier failures
				err = kv.NewError("panic").With("panic", fmt.Sprintf("%#+v", r)).With("stack", stack.Trace().TrimRuntime())
			}
		}
		p.deallocate(alloc, p.Request.Experiment.Key)
	}()

	// The ResponseQ is a means of sending informative messages to a listener
	// acting as an experiment orchestration agent while experiments are running
	if p.ResponseQ != nil {
		select {
		case p.ResponseQ <- "":
		default:
			logger.Warn("unresponsive response queue channel")
		}
	}

	// The allocation details are passed in to the runner to allow the
	// resource reservations to become known to the running applications.
	// This call will block until the task stops processing.

	if warns, err := p.deployAndRun(ctx, alloc, p.AccessionID); err != nil {
		if p.ResponseQ != nil {
			select {
			case p.ResponseQ <- "":
			default:
				logger.Warn("unresponsive response queue channel")
			}
		}
		logger.Debug("DEPLOY-RUN failed", "error:", err.Error())
		for inx, warn := range warns {
			logger.Debug("Warning: ", inx, " msg: ", warn.Error())
		}
		return p.evalDone, err
	}

	if p.ResponseQ != nil {
		select {
		case p.ResponseQ <- "":
		default:
			logger.Warn("unresponsive response queue channel")
		}
	}
	return true, nil
}

// getHash produces a very simple and short hash for use in generating directory names from
// the experiment IDs assign by users to shorten the names and defang them
//
func getHash(text string) string {
	//	hasher := md5.New()
	//	hasher.Write([]byte(text))
	//	return hex.EncodeToString(hasher.Sum(nil))
	//
	// The stadtx hash could improve on this, see https://github.com/dgryski/go-stadtx.  However
	// it appears the impl was never set in stone and the author has disappeared from github
	//
	return fmt.Sprintf("%x", farm.Hash64([]byte(text)))
}

// mkUniqDir will create a working directory for an experiment
// using the file system calls appropriately so as to make sure
// no other instance of the same experiment is using it.  It is
// only being used by the caller and for which no race conditions
// during creation would have occurred.
//
// A new UUID could have been used to do this but that makes
// diagnosis of failed experiements very difficult so we keep a meaningful
// name for the new directory and use an index on the end of the experiment
// id so that during diagnosis we know exactly which attempts came first.
//
// There are lots of easier methods to create unique directories of course,
// but most involve using long unique identifies.
//
// This function will fill in the name being used into the structure being
// used for the method scope on success.
//
// The name of a created working directory will be returned that can be used
// for the dynamic portions of processing such as for creating
// the python virtual environment and also script files used by the runner.  This
// isolates experimenter supplied files from the runners working files and
// can be prevent uploading artifacts needlessly.
//
func (p *processor) mkUniqDir() (dir string, err kv.Error) {

	_ = os.MkdirAll(filepath.Join(p.RootDir, "experiments"), 0700)

	// Shorten any excessively massively long names supplied by users
	expDir := getHash(p.Request.Experiment.Key)

	inst := 0
	direct := ""

	guardExprDir.Lock()
	defer guardExprDir.Unlock()

	// Loop until we fail to find a directory with the prefix
	for {
		direct = filepath.Join(p.RootDir, "experiments", expDir+"."+strconv.Itoa(inst))

		// Create the next directory in sequence with another directory containing our signature
		errGo := os.Mkdir(direct, 0700)
		switch {
		case errGo == nil:
			p.ExprDir = direct
			p.ExprSubDir = expDir + "." + strconv.Itoa(inst)

			return "", nil
		case os.IsExist(errGo):
			inst++
			continue
		}
		err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		logger.Warn("failure creating working dir", "directory", direct, "error", err)
		return "", err
	}

}

// extractValidEnv is used to convert the environment variables of the current process
// into a map removing any names that dont translate to valid user environment variables,
// such as names that start with underscores etc
//
func extractValidEnv() (envs map[string]string) {

	envs = map[string]string{}
	for _, v := range os.Environ() {
		// After the first equal keep everything else together
		pair := strings.SplitN(v, "=", 2)
		// Extract the first unicode rune and test that it is a valid character for an env name
		envName := []rune(pair[0])
		if len(pair) == 2 && (unicode.IsLetter(envName[0]) || unicode.IsDigit(envName[0])) {
			pair[1] = strings.Replace(pair[1], "\"", "\\\"", -1)
			envs[pair[0]] = pair[1]
		} else {
			// The underscore is always present and represents the CWD so dont print messages about it
			if envName[0] != '_' {
				logger.Debug(fmt.Sprintf("env var %s (%c) (%d) dropped due to conformance", pair[0], envName[0], len(pair)))
			}
		}
	}
	return envs
}

// applyEnv is used to apply the contents of the env block specified by the studioml client into the
// runners environment table.
//
// this function is also used to examine the contents of the processor request environment variables and
// to resolve locally any environment variables that are present indicated by the %...% pairs.
// If the enclosed value is not an environment variable within the context of the runner then the
// text will be left untouched.
//
// This behavior is specific to the go runner at this time.
//
func (p *processor) applyEnv(alloc *pkgResources.Allocated) {

	p.ExprEnvs = extractValidEnv()

	// Expand %...% pairs by iterating the env table for the process and explicitly replacing on each line
	re := regexp.MustCompile(`(?U)(?:\%(.*)*\%)+`)

	// Checkmarx code checking note. Checkmarx is for Web applications and is not a good fit general purpose server code.
	// It is also worth mentioning that if you are reading this message that Checkmarx does not understand Go package structure
	// and does not appear to use the Go AST  to validate code so is not able to perform path and escape analysis which
	// means that more than 95% of warning are for unvisited code.
	//
	// The following will raise a 'Denial Of Service Resource Exhaustion' message but this is bogus.
	// The data used to generate the p.Request values comes from a managed json message that is validated.

	// Environment variables need to be applied here to assist in unpacking S3 files etc
	for k, v := range p.Request.Config.Env {

		for _, match := range re.FindAllString(v, -1) {
			if envV := os.Getenv(match[1 : len(match)-1]); len(envV) != 0 {
				v = strings.Replace(v, match, envV, -1)
			}
		}
		// Update the processor env table with the resolved value
		p.Request.Config.Env[k] = v

		p.ExprEnvs[k] = v
	}

	// create the map into which customer environment variables will be added to
	// the experiment script
	//
	p.ExprEnvs["AWS_SDK_LOAD_CONFIG"] = "1"

	// Although we copy the env values to the runners env table through they done get
	// automatically included into the script this is done via the Make being given
	// a set of env variables as an array that will be written into the script using the receiever
	// contents.
	//
	for _, gpu := range alloc.GPU {
		for env, gpuVar := range gpu.Env {
			if len(gpuVar) != 0 {
				if expVar, isPresent := p.ExprEnvs[env]; isPresent {
					p.ExprEnvs[env] = expVar + "," + gpuVar
				} else {
					p.ExprEnvs[env] = gpuVar
				}
			}
		}
	}
}

func (p *processor) calcTimeLimit() (maxDuration time.Duration) {
	// Determine when the life time of the experiment is over and then check it before starting
	// the experiment.  when running this function also checks to ensure the lifetime has not expired
	//
	maxDuration = time.Duration(96 * time.Hour)

	defer func() {
		logger.Debug("maxDuration computed: ", maxDuration.String(), "experiment_id", p.Request.Experiment.Key,
			"stack", stack.Trace().TrimRuntime())
	}()

	if len(p.Request.Config.Lifetime) != 0 {
		limit, errGo := time.ParseDuration(p.Request.Config.Lifetime)
		if errGo != nil {
			logger.Warn("maximum life time ignored", "error", errGo,
				"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime())
		} else {
			if p.Request.Experiment.TimeAdded > 10.0 {
				limit = time.Until(time.Unix(int64(p.Request.Experiment.TimeAdded), 0).Add(limit))
				if limit <= 0 {
					logger.Warn("maximum life time reached",
						"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
						"stack", stack.Trace().TrimRuntime())
					return 0
				}
				if limit < maxDuration {
					maxDuration = limit
				}
			}
		}
	}

	// Determine the maximum run duration for any single attempt to run the experiment
	if len(p.Request.Experiment.MaxDuration) != 0 {
		limit, errGo := time.ParseDuration(p.Request.Experiment.MaxDuration)
		if errGo != nil {
			logger.Warn("maximum duration ignored", "error", errGo,
				"experiment_id", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime())
		}
		logger.Debug("limit computed: ", limit.String(), "experiment_id", p.Request.Experiment.Key,
			"stack", stack.Trace().TrimRuntime())
		if limit < maxDuration {
			maxDuration = limit
			logger.Debug("result computed: ", maxDuration.String(), "experiment_id", p.Request.Experiment.Key)
		}
	}
	return maxDuration
}

func doCheckpoint(group string, art request.Artifact) bool {
	return group != statusArtifactName && art.Mutable && art.SaveFreq > 0
}

func (p *processor) checkpointStart(ctx context.Context, accessionID string, refresh map[string]request.Artifact, saveTimeout time.Duration) (doneC chan struct{}) {
	doneC = make(chan struct{}, 1)

	// On a regular basis we will flush the log and compress it for uploading to
	// AWS or Google Cloud Storage etc, use the interval specified in the meta data for the job
	//
	saveArtCnt := 0
	for group, artifact := range refresh {
		if doCheckpoint(group, artifact) {
			saveArtCnt++
		}
	}

	if saveArtCnt > 0 {
		doneArtC := make(chan string, saveArtCnt)
		for group, artifact := range refresh {
			if doCheckpoint(group, artifact) {
				go p.artifactCheckpointer(ctx, saveTimeout, accessionID, group, artifact, doneArtC)
			}
		}

		go func() {
			for saveArtCnt > 0 {
				select {
				case artName := <-doneArtC:
					logger.Debug("Done checkpointing ", "artifact: ", artName)
					saveArtCnt--
				}
			}
			close(doneC)
		}()
	} else {
		close(doneC)
	}

	return doneC
}

func (p *processor) artifactCheckpointer(ctx context.Context, saveTimeout time.Duration, accessionID string,
	group string, artifact request.Artifact, doneC chan string) {

	// assume SaveFreq > 0
	checkpoint := time.NewTicker(time.Duration(artifact.SaveFreq) * time.Second)
	defer checkpoint.Stop()

	for {
		select {
		case <-checkpoint.C:
			// The context that is supplied by the caller relates to the experiment itself, however what we dont want
			// to happen is for the uploading of artifacts to be terminated until they complete so we build a new context
			// for the uploads and use the ctx supplied as a lifecycle indicator
			uploadCtx, origCancel := context.WithTimeout(context.Background(), saveTimeout)
			uploadCancel := runner.GetCancelWrapper(origCancel, "checkpoint artifacts", logger)

			// Here a regular checkpoint of the artifacts is being done.  Before doing this
			// we should copy meta data related files from the output directory and other
			// locations into the _metadata artifact area
			logger.Debug("Checkpointing start ", "artifact: ", group)
			if _, _, err := p.returnOne(uploadCtx, group, artifact, accessionID); err != nil {
				logger.Warn("artifact not returned", "experiment_id", p.Request.Experiment.Key, "artifact", group, "error", err.Error())
			}
			logger.Debug("Checkpointing end ", "artifact: ", group)
			uploadCancel()

		case <-ctx.Done():
			// The context that is supplied by the caller relates to the experiment itself,
			// and after experiment is done,
			// in any case we are running final artifacts saving/upload.
			// So here we can just quietly stop and exit.
			doneC <- group
			return
		}
	}
}

// runScript is used to start a script execution along with an artifact checkpointer that both remain running until the
// experiment is done.  refresh contains a list of the artifacts that require checkpointing
//
func (p *processor) runScript(ctx context.Context, accessionID string, refresh map[string]request.Artifact, refreshTimeout time.Duration) (err kv.Error) {

	// Create a context that can be cancelled within the runScript so that the checkpointer
	// and the executor are aligned on the termination of a job either from the base
	// context that would normally be a timeout or explicit cancellation, or the task
	// completes normally and terminates by returning
	runCtx, origCancel := context.WithCancel(context.Background())
	runCancel := runner.GetCancelWrapper(origCancel, "run script context", logger)

	// Start a checkpointer for our output files and pass it the context used
	// to notify when it is to stop.  Save a reference to the channel used to
	// indicate when the checkpointer has flushed files etc.
	//
	// This function also ensures that the queue related to the work being
	// processed is still present, if not the task should be terminated.
	//
	doneC := p.checkpointStart(runCtx, accessionID, refresh, refreshTimeout)

	cancelReason := ""

	// Now wait on the context supplied by the caller to be done,
	// or our own internal one to be done and then when either happens
	// make sure we terminate our own internal context
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Info("recovered", "recover", r)
			}
		}()

		// Wait for any one of two contexts to cancel,
		// or for external request to abort this workload to come in:
		select {
		case <-ctx.Done():
			cancelReason = "global context"
			if ctx.Err() == context.DeadlineExceeded {
				cancelReason = "external timeout"
				// For external workload timeout,
				// we consider it done and don't re-submit for execution
				p.evalDone = true
			}
		case st := <-p.status:
			cancelReason = "external request: " + st
			p.evalDone = true
		case <-runCtx.Done():
			cancelReason = "script context"
			return
		}

		// When the runner itself stops then we can cancel the context which will signal the checkpointer
		// to do one final save of the experiment data and return after closing its own doneC channel
		runCancel()
	}()

	// Blocking call to run the process that uses the ctx for timeouts etc
	err = p.Executor.Run(runCtx, refresh)
	if "" != cancelReason {
		err = err.With("was cancelled by", cancelReason)
	}

	// Make sure that if a panic occurs when cancelling a context already cancelled
	// or some other error we continue with termination processing
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Info("recovered", "recover", r)

				if err != nil {
					// Modify the return values to include details about the panic, but be sure not to
					// obscure earlier failures
					err = kv.NewError("panic").With("panic", fmt.Sprintf("%#+v", r)).With("stack", stack.Trace().TrimRuntime())
				}
			}
		}()
		// When the runner itself stops then we can cancel the context which will signal the checkpointer
		// to do one final save of the experiment data and return after closing its own doneC channel
		runCancel()
	}()

	// Make sure any checkpointing is done before continuing to handle results
	// and artifact uploads
	select {
	case <-doneC:
		logger.Debug("runScript artifact checkpointer stopped", " experiment_id", p.Request.Experiment.Key)
	case <-time.After(5 * time.Minute):
		logger.Debug("runScript artifact checkpointer unresponsive", " experiment_id", p.Request.Experiment.Key)
	}

	return err
}

type StatusInfo struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

func (p *processor) getWorkloadStatus(ctx context.Context) string {
	statusGroup := "_status"
	artifact, isPresent := p.Request.Experiment.Artifacts[statusGroup]
	if !isPresent || 0 == len(artifact.Qualified) {
		return "no <status> artifact"
	}

	diskBudget := int64(100 * 1024 * 1024)
	_, _, err := artifactCache.Fetch(ctx, artifact.Clone(), p.Request.Config.Database.ProjectId, statusGroup, diskBudget, p.ExprEnvs, p.ExprDir)
	if err != nil {
		logger.Info("failed to read status", "artifact", artifact.Qualified, "experiment_id", p.Request.Experiment.Key, "error", err.Error())
		return "failed to read"
	}

	fpath := filepath.Join(p.ExprDir, statusGroup, statusFileName)
	data, errGo := ioutil.ReadFile(fpath)
	if errGo != nil {
		logger.Info("failed to read status file", "path", fpath, "experiment_id", p.Request.Experiment.Key, "error", errGo.Error())
		return "failed to read file"
	}

	expData := &StatusInfo{}
	errGo = json.Unmarshal(data, expData)
	if errGo != nil {
		logger.Info("failed to parse status file", "path", fpath, "experiment_id", p.Request.Experiment.Key, "error", errGo.Error())
		return "failed to parse file"
	}

	return expData.Status
}

func (p *processor) startStatusNotifications(ctx context.Context) (cancel context.CancelFunc, err kv.Error) {
	p.status = make(chan string)
	statusCtx, origCancel := context.WithCancel(ctx)
	statusCancel := runner.GetCancelWrapper(origCancel, "status updater context", logger)

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(10 * time.Second):
				st := strings.ToLower(p.getWorkloadStatus(ctx))
				logger.Debug("received workload status", "status", st, "experiment_id", p.Request.Experiment.Key)

				if "stopped" == st {
					select {
					case p.status <- st:
					case <-time.After(5 * time.Second):
					}
				}
			}
		}

	}(statusCtx)

	return statusCancel, nil
}

func (p *processor) run(ctx context.Context, alloc *pkgResources.Allocated, accessionID string) (err kv.Error) {

	// Now figure out the absolute time that the experiment is limited to
	maxDuration := p.calcTimeLimit()
	startedAt := time.Now()
	terminateAt := time.Now().Add(maxDuration)

	// Start externally provided notifications for this workload (experiment) status.
	// Listening to p.status channel we could be notified, for example,
	// that client wants to immediately cancel execution for this experiment.
	statusCancel, _ := p.startStatusNotifications(ctx)
	defer statusCancel()

	// Now we have the files locally stored we can begin the work
	if err, evalDone := p.Executor.Make(ctx, alloc, p); err != nil {
		if evalDone {
			p.evalDone = true
		}
		return err
	}

	refresh := make(map[string]request.Artifact, len(p.Request.Experiment.Artifacts))
	for k, v := range p.Request.Experiment.Artifacts {
		if v.Mutable {
			refresh[k] = v
		}
	}
	// This value is not yet parameterized but should eventually be
	//
	// Each checkpoint or artifact upload back to the s3 servers etc needs a timeout that is
	// separate from the go context for the experiment in order that when timeouts occur on
	// the experiment they dont trash artifact uploads which are permitted to run after the
	// experiment has terminated/stopped/killed etc
	//
	refreshTimeout := 5 * time.Minute

	// Recheck the expiry time as the make step can be time consuming
	if terminateAt.Before(time.Now()) {
		return kv.NewError("already expired").
			With("experiment_id", p.Request.Experiment.Key,
				"started_at", startedAt, "max_duration", maxDuration.String(),
				"stack", stack.Trace().TrimRuntime())
	}

	// Setup a time limit for the work we are doing
	runCtx, origCancel := context.WithTimeout(ctx, maxDuration)
	msg := fmt.Sprintf("workload run for %s tmout %s", p.Request.Experiment.Key, maxDuration.String())
	runCancel := runner.GetCancelWrapper(origCancel, msg, logger)
	defer runCancel()

	if logger.IsInfo() {

		deadline, _ := runCtx.Deadline()

		logger.Info("run starting",
			"experiment_id", p.Request.Experiment.Key,
			"lifetime_duration", p.Request.Config.Lifetime,
			"started_at", startedAt,
			"max_duration", p.Request.Experiment.MaxDuration,
			"actual_max_duration", maxDuration.String(),
			"deadline", deadline,
			"stack", stack.Trace().TrimRuntime())
		defer logger.Debug("run stopping",
			"experiment_id", p.Request.Experiment.Key,
			"started_at", startedAt,
			"stack", stack.Trace().TrimRuntime())
	}

	// Blocking call to run the script and only return when done.  Cancellation is done
	// if needed using the cancel function created by the context, runCtx
	//
	return p.runScript(runCtx, accessionID, refresh, refreshTimeout)
}

func outputErr(fn string, inErr kv.Error, msg string) (err kv.Error) {
	if inErr == nil {
		return nil
	}
	f, errGo := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()
	f.WriteString(msg)
	f.WriteString(inErr.Error())
	return nil
}

// deployAndRun is called to execute the work unit by the Process receiver
//
func (p *processor) deployAndRun(ctx context.Context, alloc *pkgResources.Allocated, accessionID string) (warns []kv.Error, err kv.Error) {

	defer func(ctx context.Context) {
		if r := recover(); r != nil {
			logger.Warn("panic", "panic", fmt.Sprintf("%#+v", r), "stack", string(debug.Stack()))

			// Modify the return values to include details about the panic
			err = kv.NewError("panic running studioml script").With("panic", fmt.Sprintf("%#+v", r)).With("stack", stack.Trace().TrimRuntime())
		}

		termination := "deployAndRun ctx abort"
		if ctx != nil {
			select {
			case <-ctx.Done():
			default:
				termination = "deployAndRun stopping"
			}
		}

		logger.Info(termination, "experiment_id", p.Request.Experiment.Key)

		// We should always upload results even in the event of an error to
		// help give the experimenter some clues as to what might have
		// failed if there is a problem.  The original ctx could have expired
		// so we simply create and use a new one to do our upload.
		//
		timeout, origCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		cancel := runner.GetCancelWrapper(origCancel, "final artifacts upload", logger)
		if rerr := p.returnAll(timeout, accessionID, err); rerr != nil {
			if err == nil {
				err = rerr
			}
		}
		cancel()

		if !*debugOpt {
			defer os.RemoveAll(p.ExprDir)
		}
	}(ctx)

	// Update and apply environment variables for the experiment
	p.applyEnv(alloc)

	if *debugOpt {
		// The following log can expose passwords etc.  As a result we do not allow it unless the debug
		// non production flag is explicitly set
		logger.Trace(fmt.Sprintf("experiment → %v → %s → %#v", p.Request.Experiment, p.ExprDir, *p.Request))
	}

	// The standard output file for studio jobs, is used here in the event that a catastrophic error
	// occurs before the job starts
	//
	outputFN := filepath.Join(p.ExprDir, "output", "output")

	// fetchAll when called will have access to the environment variables used by the experiment in order that
	// credentials can be used
	if err = p.fetchAll(ctx); err != nil {
		// A failure here should result in a warning being written to the processor
		// output file in the hope that it will be returned.  Likewise further on down in
		// this function
		//
		if errO := outputErr(outputFN, err, "failed when downloading user data\n"); errO != nil {
			warns = append(warns, errO)
		}
		return warns, err
	}

	// Blocking call to run the task
	if err = p.run(ctx, alloc, accessionID); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		// TODO: If the failure was related to the healthcheck then requeue and backoff the queue
		logger.Info("task run exit error", "experiment_id", p.Request.Experiment.Key, "error", err.Error())
		if errO := outputErr(outputFN, err, "failed when running user task\n"); errO != nil {
			warns = append(warns, errO)
		}
	}

	logger.Info("deployAndRun stopping", "experiment_id", p.Request.Experiment.Key)

	return warns, err
}
