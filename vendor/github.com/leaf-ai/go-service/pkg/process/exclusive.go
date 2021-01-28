// Copyright 2018-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package process // import "github.com/leaf-ai/go-service/pkg/process"

// This file contains the implementation of code that checks to ensure
// that the local machine only has one entity accessing a named resource.
// This allows callers of this code to create and test for exclusive
// access to resources, or to check that only one instance of a
// process is running.

import (
	"context"
	"net"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// Exclusive is a data structure used to tracking and ensure only one
// instance of the go runner is active on a system at once
//
type Exclusive struct {
	Name   string
	listen net.Listener
}

// NewExclusive is used to initialize a unix domain socket that ensure that only one
// runner process is active on a kubernetes pod or machine at the same time.  If there
// are other processes active then it will return an error.
//
// An example of a server that wishes to restrict itself to a single instance of a process
// would appear as follows:
//
//func main() {
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	// This is the one check that does not get tested when the server is under test
//	//
//	if _, err := process.NewExclusive(ctx, "my-server"); err != nil {
//		fmt.Fprintf(os.Sdterr, "An instance of this process is already running %s", err.Error())
//		os.Exit(-1)
//	}
//	…
//}
//
//
func NewExclusive(ctx context.Context, name string) (excl *Exclusive, err kv.Error) {

	excl = &Exclusive{
		Name: name,
	}

	// Construct an abstract name socket that allows the name to be recycled between process
	// restarts without needing to unlink etc. For more information please see
	// https://gavv.github.io/blog/unix-socket-reuse/, and
	// http://man7.org/linux/man-pages/man7/unix.7.html
	sockName := "@/tmp/"
	sockName += name

	listen, errGo := net.Listen("unix", sockName)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	excl.listen = listen

	go func() {
		go excl.listen.Accept()
		<-ctx.Done()
	}()
	return excl, nil
}
