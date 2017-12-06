// +build testrunmain

package main

import "testing"

// TestRunMain can be used to run the server in production mode as opposed to
// funit or unit testing mode.  Traditionally gathering coverage data and running
// in production are done seperately.  This unit test allows the runner to do
// both at the same time.  To do this a test binary is generated using the command
//
// cd $(GOROOT)/src/github.com/SentientTechnologies/studio-go-runner
// go test -coverpkg="." -c -o bin/runner-cpu-run-coverage -tags 'testrunmain NO_CUDA' cmd/runner/*.go
//
// Then the resulting /bin/runner-cpu-run-coverage binary is run as through it were a traditional
// server binary for the go runner using the command below.  The difference being that the
// binary now has coverage instrumentation.  In order to collect the coverage run any production
// workload and use cases you need then CTRL-C the server.
//
// ./bin/runner-cpu-run-coverage -test.run "^TestRunMain$" -test.coverprofile=system.out
//
// As an additional feature coverage files have is that they can also be merged using
// commands similar to the following:
//
// $ go get github.com/wadey/gocovmerge
// $ gocovmerge unit.out system.out > all.out
// $ go tool cover -html all.out
//
// Using the coverage merge tool testing done using a fully deployed system with
// real projects, proxies, projects, and workloads along with integration testing can be merged
// together from different test steps in an integration and test pipeline.
//
func TestRunMain(t *testing.T) {
	<-TestStopC
}
