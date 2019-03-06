package runner

import (
	"net"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// Functions related to networking needs for the runner

// GetFreePort will find and return a port number that is found to be available
//
func GetFreePort(hint string) (port int, err kv.Error) {
	addr, errGo := net.ResolveTCPAddr("tcp", hint)
	if errGo != nil {
		return 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	l, errGo := net.ListenTCP("tcp", addr)
	if errGo != nil {
		return 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	port = l.Addr().(*net.TCPAddr).Port

	// Dont defer as the port will be quickly reused
	l.Close()

	return port, nil
}
