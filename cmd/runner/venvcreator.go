// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/task"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
)

type venvCreator struct {
	Group       string            `json:"group"` // A caller specific grouping for work that can share sensitive resources
	RootDir     string            `json:"root_dir"`
	ExprDir     string            `json:"expr_dir"`
	ExprSubDir  string            `json:"expr_sub_dir"`
	ExprEnvs    map[string]string `json:"expr_envs"`
	AccessionID string
	Request     *request.Request `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
	logger      *log.Logger
	evalDone    bool // true, if evaluation should be processed as completed
}

// newVEnvCreator
//
func newVEnvCreator(ctx context.Context, qt *task.QueueTask, req *request.Request, accessionID string) (proc TaskProcessor, hardError bool, err kv.Error) {

	_ = ctx
	temp, err := makeCWD()
	if err != nil {
		return nil, false, err
	}

	// Processors share the same root directory and use acccession numbers on the experiment key
	// to avoid collisions
	//
	task_proc := &venvCreator{
		RootDir:     temp,
		Group:       qt.Subscription,
		AccessionID: accessionID,
		Request:     req,
		evalDone:    true,
		logger:      log.NewLogger("venv-creator"),
	}

	task_proc.logger.Info("Starting processing by newVEnvCreator for id: ", accessionID)

	if _, err = task_proc.mkUniqDir(); err != nil {
		return proc, false, err
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

func (p *venvCreator) mkUniqDir() (dir string, err kv.Error) {
	dir, subDir, err := MakeUniqDir(p.RootDir, p.Request.Experiment.Key)
	p.ExprDir = dir
	p.ExprSubDir = subDir
	return dir, err
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

	err = p.venvCreateAndRun(ctx)
	// We always consider VEnv creation task "done",
	// whatever the result.
	// That is, we will never re-submit this task for execution
	return true, err
}

// deployAndRun is called to execute the work unit by the Process receiver
//
func (p *venvCreator) venvCreateAndRun(ctx context.Context) (err kv.Error) {

	defer func(ctx context.Context) {
		if r := recover(); r != nil {
			logger.Warn("panic", "panic", fmt.Sprintf("%#+v", r), "stack", string(debug.Stack()))

			// Modify the return values to include details about the panic
			err = kv.NewError("panic running venv creation script").With("panic", fmt.Sprintf("%#+v", r)).With("stack", stack.Trace().TrimRuntime())
		}

		termination := "venvCreateAndRun ctx abort"
		if ctx != nil {
			select {
			case <-ctx.Done():
			default:
				termination = "venvCreateAndRun stopping"
			}
		}

		logger.Info(termination, "experiment_id", p.Request.Experiment.Key)

		if !*debugOpt {
			defer os.RemoveAll(p.ExprDir)
		}
	}(ctx)

	// Update and apply environment variables for the experiment
	p.applyEnv()

	// Get Python virtual environment ID:
	var venvEntry *runner.VirtualEnvEntry = nil
	if venvEntry, err = runner.GetVirtualEnvEntry(ctx, p.Request, nil, p.ExprDir); err != nil {
		return err.With("stack", stack.Trace().TrimRuntime()).With("workDir", p.ExprDir)
	}

	venvID, venvValid := venvEntry.AddClient(p.AccessionID)

	defer func() {
		venvEntry.RemoveClient(p.AccessionID)
	}()

	if !venvValid {
		err = kv.NewError("venv is invalid").With("venv", venvID, "stack", stack.Trace().TrimRuntime()).With("workDir", p.ExprDir)
		return err
	}
	return nil
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
	p.ExprEnvs = extractValidEnv()

	// Expand %...% pairs by iterating the env table for the process and explicitly replacing on each line
	re := regexp.MustCompile(`(?U)(?:\%(.*)*\%)+`)

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
}

func (p *venvCreator) Close() (err error) {
	if *debugOpt || 0 == len(p.ExprDir) {
		return nil
	}
	return os.RemoveAll(p.ExprDir)
}
