// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/leaf-ai/studio-go-runner/internal/request"
	pkgResources "github.com/leaf-ai/studio-go-runner/internal/resources"
	"github.com/leaf-ai/studio-go-runner/internal/task"
	"runtime/debug"
)

type venvCreator struct {
	Group      string            `json:"group"` // A caller specific grouping for work that can share sensitive resources
	RootDir    string            `json:"root_dir"`
	ExprDir    string            `json:"expr_dir"`
	ExprSubDir string            `json:"expr_sub_dir"`
	ExprEnvs   map[string]string `json:"expr_envs"`
	Request    *request.Request  `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
	Executor   Executor
	evalDone   bool // true, if evaluation should be processed as completed
}

// newProcessor will parse the inbound message and then validate that there are
// sufficient resources to run an experiment and then create a new working directory.
//
func newVEnvCreator(ctx context.Context, qt *task.QueueTask, req *request.Request, accessionID string) (proc TaskProcessor, hardError bool, err kv.Error) {

	temp, err := makeCWD()
	if err != nil {
		return nil, false, err
	}

	// Processors share the same root directory and use acccession numbers on the experiment key
	// to avoid collisions
	//
	task_proc := &venvCreator{
		RootDir:  temp,
		Group:    qt.Subscription,
		Request:  req,
		evalDone: false,
	}

	//if task_proc.Executor, err = runner.NewVirtualEnv(task_proc.Request, task_proc.ExprDir, task_proc.AccessionID, logger); err != nil {
	//	return nil, true, err
	//}
	return task_proc, false, nil
}

func (proc *venvCreator) GetRequest() *request.Request {
	return proc.Request
}

func (proc *venvCreator) SetRequest(req *request.Request) {
	proc.Request = req
}

func (proc *venvCreator) GetRootDir() string {
	return proc.RootDir
}

// Process is the main function where task processing occurs.
//
// This function is invoked by the cmd/runner/handle.go:HandleMsg function and blocks.
//
func (p *venvCreator) Process(ctx context.Context) (ack bool, err kv.Error) {

	// Set up a function to use
	// a panic handler to catch issues related to, or unrelated to the runner
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
	}()

	//if warns, err := p.deployAndRun(ctx, alloc, p.AccessionID); err != nil {
	//	logger.Debug("VENV-CREATE failed", "error:", err.Error())
	//	for inx, warn := range warns {
	//		logger.Debug("Warning: ", inx, " msg: ", warn.Error())
	//	}
	//	return p.evalDone, err
	//}
	return true, nil
}

// deployAndRun is called to execute the work unit by the Process receiver
//
func (p *venvCreator) deployAndRun(ctx context.Context, alloc *pkgResources.Allocated, accessionID string) (warns []kv.Error, err kv.Error) {

	//defer func(ctx context.Context) {
	//	if r := recover(); r != nil {
	//		logger.Warn("panic", "panic", fmt.Sprintf("%#+v", r), "stack", string(debug.Stack()))
	//
	//		// Modify the return values to include details about the panic
	//		err = kv.NewError("panic running studioml script").With("panic", fmt.Sprintf("%#+v", r)).With("stack", stack.Trace().TrimRuntime())
	//	}
	//
	//	termination := "deployAndRun ctx abort"
	//	if ctx != nil {
	//		select {
	//		case <-ctx.Done():
	//		default:
	//			termination = "deployAndRun stopping"
	//		}
	//	}
	//
	//	logger.Info(termination, "experiment_id", p.Request.Experiment.Key)
	//
	//	// We should always upload results even in the event of an error to
	//	// help give the experimenter some clues as to what might have
	//	// failed if there is a problem.  The original ctx could have expired
	//	// so we simply create and use a new one to do our upload.
	//	//
	//	timeout, origCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	//	cancel := runner.GetCancelWrapper(origCancel, "final artifacts upload", logger)
	//	if rerr := p.returnAll(timeout, accessionID, err); rerr != nil {
	//		if err == nil {
	//			err = rerr
	//		}
	//	}
	//	cancel()
	//
	//	if !*debugOpt {
	//		defer os.RemoveAll(p.ExprDir)
	//	}
	//}(ctx)
	//
	//// Update and apply environment variables for the experiment
	//p.applyEnv(alloc)
	//
	//if *debugOpt {
	//	// The following log can expose passwords etc.  As a result we do not allow it unless the debug
	//	// non production flag is explicitly set
	//	logger.Trace(fmt.Sprintf("experiment → %v → %s → %#v", p.Request.Experiment, p.ExprDir, *p.Request))
	//}
	//
	//// The standard output file for studio jobs, is used here in the event that a catastrophic error
	//// occurs before the job starts
	////
	//outputFN := filepath.Join(p.ExprDir, "output", "output")
	//
	//// fetchAll when called will have access to the environment variables used by the experiment in order that
	//// credentials can be used
	//if err = p.fetchAll(ctx); err != nil {
	//	// A failure here should result in a warning being written to the processor
	//	// output file in the hope that it will be returned.  Likewise further on down in
	//	// this function
	//	//
	//	if errO := outputErr(outputFN, err, "failed when downloading user data\n"); errO != nil {
	//		warns = append(warns, errO)
	//	}
	//	return warns, err
	//}
	//
	//// Blocking call to run the task
	//if err = p.run(ctx, alloc, accessionID); err != nil {
	//	// TODO: We could push work back onto the queue at this point if needed
	//	// TODO: If the failure was related to the healthcheck then requeue and backoff the queue
	//	logger.Info("task run exit error", "experiment_id", p.Request.Experiment.Key, "error", err.Error())
	//	if errO := outputErr(outputFN, err, "failed when running user task\n"); errO != nil {
	//		warns = append(warns, errO)
	//	}
	//}
	//
	//logger.Info("deployAndRun stopping", "experiment_id", p.Request.Experiment.Key)

	return warns, err
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
func (p *venvCreator) applyEnv() {

	//p.ExprEnvs = extractValidEnv()
	//
	//// Expand %...% pairs by iterating the env table for the process and explicitly replacing on each line
	//re := regexp.MustCompile(`(?U)(?:\%(.*)*\%)+`)
	//
	//// Environment variables need to be applied here to assist in unpacking S3 files etc
	//for k, v := range p.Request.Config.Env {
	//	for _, match := range re.FindAllString(v, -1) {
	//		if envV := os.Getenv(match[1 : len(match)-1]); len(envV) != 0 {
	//			v = strings.Replace(v, match, envV, -1)
	//		}
	//	}
	//	// Update the processor env table with the resolved value
	//	p.Request.Config.Env[k] = v
	//
	//	p.ExprEnvs[k] = v
	//}
	//
	//// create the map into which customer environment variables will be added to
	//// the experiment script
	////
	//p.ExprEnvs["AWS_SDK_LOAD_CONFIG"] = "1"
}

func (p *venvCreator) Close() (err error) {
	return nil
}
