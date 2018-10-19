package runner

// This file contains the implementation of code that checks to ensure
// that the local machine only has one entity accessing a named resource.
// This allows callers of this code to create and test for exclusive
// access to resources, or to check that only one instance of a
// process is running.

import (
	"fmt"
	"net"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// Exclusive is a data structure used to tracking and ensure only one
// instance of the go runner is active on a system at once
//
type Exclusive struct {
	Name     string
	ReleaseC chan struct{}
	listen   net.Listener
}

// NewExclusive is used to initialize a unix domain socket that ensure that only one
// runner process is active on a kubernetes pod or machine at the same time.  If there
// are other processes active then it will return an error.
//
func NewExclusive(name string, quitC chan struct{}) (excl *Exclusive, err errors.Error) {

	excl = &Exclusive{
		Name:     name,
		ReleaseC: quitC,
	}

	// Construct an abstract name socket that allows the name to be recycled between process
	// restarts without needing to unlink etc. For more information please see
	// https://gavv.github.io/blog/unix-socket-reuse/, and
	// http://man7.org/linux/man-pages/man7/unix.7.html
	sockName := "@/tmp/"
	sockName += name

	errGo := fmt.Errorf("")
	excl.listen, errGo = net.Listen("unix", sockName)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	go func() {
		go excl.listen.Accept()
		<-excl.ReleaseC
	}()
	return excl, nil
}
