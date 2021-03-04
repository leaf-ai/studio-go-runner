package defense

import (
	"path/filepath"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file implements some simple checking file system functions

// WillEscape checks to see if the candidate name will escape the target directory
//
func WillEscape(candidate string, target string) (escapes bool, err kv.Error) {

	effective, errGo := filepath.EvalSymlinks(filepath.Join(target, candidate))
	// If the directory exists the the eval will have directed us to where
	// the link is targeted at, if not then the directory might not exist
	// in which case we can simply use the supplied target path
	if errGo != nil {
		effective = filepath.Clean(candidate)
	}

	relpath, errGo := filepath.Rel(target, filepath.Join(target, effective))
	if errGo != nil {
		return true, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return strings.HasPrefix(relpath, ".."), nil
}
