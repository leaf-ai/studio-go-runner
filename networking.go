package runner

import (
	"net"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// Functions related to networking needs for the runner

func GetFreePort(hint string) (port int, err errors.Error) {
	addr, errGo := net.ResolveTCPAddr("tcp", hint)
	if errGo != nil {
		return 0, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	l, errGo := net.ListenTCP("tcp", addr)
	if errGo != nil {
		return 0, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	port = l.Addr().(*net.TCPAddr).Port

	// Dont defer as the port will be quickly reused
	l.Close()

	return port, nil
}
