// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bufio"
	"context"
	"flag"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/andreidenissov-cog/go-service/pkg/server"
)

// This file contains the implementation of configuration updater
// for task queues match/mismatch regular expressions.

var (
	queueMatch             = flag.String("queue-match", "^(rmq|sqs|local)_.*$", "User supplied regular expression that needs to match a queues name to be considered for work")
	queueMismatch          = flag.String("queue-mismatch", "", "User supplied regular expression that must not match a queues name to be considered for work")
	queueMatchConfigKey    = "QUEUE_MATCH"
	queueMismatchConfigKey = "QUEUE_MISMATCH"
)

type queueMatcherType struct {
	match          string
	matchRegExp    *regexp.Regexp
	mismatch       string
	mismatchRegExp *regexp.Regexp
	updater        chan server.K8sConfigUpdate
	logger         *log.Logger
	sync.Mutex
}

var (
	queueMatcher = queueMatcherType{
		match:    *queueMatch,
		mismatch: *queueMismatch,
		updater:  make(chan server.K8sConfigUpdate, 1),
	}
)

func (qm queueMatcherType) updatePatterns(match string, mismatch string) (errs []kv.Error) {
	errs = []kv.Error{}
	matcherReg, errGo := regexp.Compile(match)
	if errGo != nil {
		if len(match) != 0 {
			err := kv.Wrap(errGo).With("matcher", match).With("stack", stack.Trace().TrimRuntime())
			qm.logger.Warn(err.Error())
			errs = append(errs, err)
		}
		matcherReg = nil
	}

	// If the length of the mismatcherReg is 0 then we will get a nil and because this
	// was checked in the main we can ignore that as this is optional
	mismatcherReg := &regexp.Regexp{}
	_ = mismatcherReg // Bypass the ineffectual assignment check

	if len(strings.Trim(mismatch, " \n\r\t")) == 0 {
		mismatcherReg = nil
	} else {
		mismatcherReg, errGo = regexp.Compile(mismatch)
		if errGo != nil {
			err := kv.Wrap(errGo).With("mismatcher", mismatch).With("stack", stack.Trace().TrimRuntime())
			qm.logger.Warn(err.Error())
			errs = append(errs, err)
		}
	}

	qm.Lock()
	defer qm.Unlock()

	qm.matchRegExp = matcherReg
	qm.mismatchRegExp = mismatcherReg
	return errs
}

func (qm queueMatcherType) getPatterns() (matcher *regexp.Regexp, mismatcher *regexp.Regexp) {
	qm.Lock()
	defer qm.Unlock()

	qm.logger.Debug("requesting queue match patterns:", "match:", qm.match, "mismatch:", qm.mismatch)
	return qm.matchRegExp, qm.mismatchRegExp
}

func (qm queueMatcherType) init(ctx context.Context, namespace string, mapname string, logger *log.Logger) (err []kv.Error) {
	qm.logger = logger
	err = qm.updatePatterns(*queueMatch, *queueMismatch)

	listeners := server.K8sConfigUpdates()
	listeners.Add(qm.updater)

	go qm.listen(ctx, namespace, mapname)
	qm.logger.Debug("started queues matcher listener", "namespace:", namespace, "map:", mapname)

	fname := "./cmupdate.txt"
	go qm.listenFile(ctx, fname)
	qm.logger.Debug("started queues matcher file listener", "source:", fname)

	return err
}

func (qm queueMatcherType) listen(ctx context.Context, namespace string, mapname string) {
	for {
		select {
		case cmap := <-qm.updater:
			if cmap.NameSpace == namespace && cmap.Name == mapname {
				matchUpdate := ""
				mismatchUpdate := ""
				updated := false
				if match, isPresent := cmap.State[queueMatchConfigKey]; isPresent && match != qm.match {
					qm.logger.Debug("queues matcher listener got update", "match:", match, "namespace:", namespace, "map:", mapname)
					matchUpdate = match
					updated = true
				}
				if mismatch, isPresent := cmap.State[queueMismatchConfigKey]; isPresent && mismatch != qm.mismatch {
					qm.logger.Debug("queues matcher listener got update", "mismatch:", mismatch, "namespace:", namespace, "map:", mapname)
					mismatchUpdate = mismatch
					updated = true
				}
				if updated {
					if errs := qm.updatePatterns(matchUpdate, mismatchUpdate); len(errs) > 0 {
						for _, err := range errs {
							qm.logger.Info("queue matcher update failed:", err.Error())
						}
					}
				}
			}

		case <-ctx.Done():
			qm.logger.Info("stopping queues matcher listener.")
			return
		}
	}
}

func (qm queueMatcherType) readConfigUpdateFromFile(fname string, update *server.K8sConfigUpdate) (err kv.Error) {
	source, errGo := os.Open(filepath.Clean(fname))
	if errGo != nil {
		return kv.Wrap(errGo).With("src", fname)
	}
	defer source.Close()

	s := bufio.NewScanner(source)
	s.Split(bufio.ScanLines)
	if s.Scan() {
		update.NameSpace = strings.TrimSpace(s.Text())
	}
	if s.Scan() {
		update.Name = strings.TrimSpace(s.Text())
	}
	if s.Scan() {
		update.State[queueMatchConfigKey] = strings.TrimSpace(s.Text())
	}
	if s.Scan() {
		update.State[queueMismatchConfigKey] = strings.TrimSpace(s.Text())
	}
	qm.logger.Debug("read config update from file:", fname, " update:", *update)
	return nil
}

func (qm queueMatcherType) listenFile(ctx context.Context, fname string) {

	var update = server.K8sConfigUpdate{
		NameSpace: "",
		Name:      "",
		State:     map[string]string{},
	}
	for {
		select {
		case <-time.After(20 * time.Second):
			if err := qm.readConfigUpdateFromFile(fname, &update); err == nil {
				qm.updater <- update
			}

		case <-ctx.Done():
			qm.logger.Info("stopping queues matcher listener on file:", fname)
			return
		}
	}
}

func InitQueueMatcher(ctx context.Context, namespace string, mapname string, logger *log.Logger) (err []kv.Error) {
	return queueMatcher.init(ctx, namespace, mapname, logger)
}

func GetQueuePatterns() (matcher *regexp.Regexp, mismatcher *regexp.Regexp) {
	return queueMatcher.getPatterns()
}
