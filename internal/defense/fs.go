package defense

import (
	"path/filepath"
	"strings"
)

// This file implements some simple checking file system functions

// WillEscape checks to see if the candidate name will escape the target directory
//
func WillEscape(candidate string, target string) (escapes bool) {

	effective, errGo := filepath.EvalSymlinks(filepath.Join(target, candidate))
	if errGo != nil {
		return true
	}

	relpath, errGo := filepath.Rel(target, effective)
	if errGo != nil {
		return true
	}

	return strings.HasPrefix(relpath, "..")
}
