// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	"github.com/rs/xid"
)

// TestCrypt is used to validate the AES large block/file style of symetric encryption
func TestCrypt(t *testing.T) {
	data := RandomString(16 * 1024)
	key, encrypted, err := EncryptBlock([]byte(data))
	if err != nil {
		t.Fatal(err.With("stack", stack.Trace().TrimRuntime()))
	}

	decrypted, err := DecryptBlock(key, encrypted)
	if err != nil {
		t.Fatal(err.With("stack", stack.Trace().TrimRuntime()))
	}
	if strings.Compare(data, string(decrypted)) != 0 {
		t.Fatal(kv.NewError("encryption decryption cycle failed").With("stack", stack.Trace().TrimRuntime()))
	}

	// Test the negative case for the key
	key[0] = 'x'
	decrypted, err = DecryptBlock(key, encrypted)
	if err == nil {
		t.Fatal(kv.NewError("bad key was accepted").With("stack", stack.Trace().TrimRuntime()))
	}
	if strings.Compare(data, string(decrypted)) == 0 {
		t.Fatal(kv.NewError("bad key was accepted").With("stack", stack.Trace().TrimRuntime()))
	}
}

// TestCryptoPython is used to test symmetric encryption in python using nacl SecretBox and Go
// crypto libraries
func TestCryptoPython(t *testing.T) {
	// Create a new TMPDIR because the python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	// Create a random passphrase
	passphrase := xid.New().String()
	// Get a pair of RSA private keys to use just for this test
	if err := GenerateTestKeys(tmpDir, 4096, passphrase); err != nil {
		t.Fatal(err)
	}

	publicPEM, err := ioutil.ReadFile(filepath.Join(tmpDir, "public.pem"))
	if err != nil {
		t.Fatal(err)
	}
	privatePEM, err := ioutil.ReadFile(filepath.Join(tmpDir, "private.pem"))
	if err != nil {
		t.Fatal(err)
	}

	// Grab know files from the crypto test library and place them into
	// our temporary test directory
	testFiles := map[string]os.FileMode{
		filepath.Join("..", "..", "assets", "crypto", "encryptor.py"): 0600,
		filepath.Join("..", "..", "assets", "crypto", "encryptor.sh"): 0700,
	}
	output, err := runPythonTest(testFiles, tmpDir, 3)
	if err != nil {
		t.Fatal(err)
	}

	clear := output[len(output)-2]
	encrypted := output[len(output)-1]

	w, err := NewWrapper(publicPEM, privatePEM, []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := w.unwrapRaw(encrypted)
	if err != nil {
		for _, aLine := range output[len(output)-2:] {
			fmt.Println(aLine)
		}
		t.Fatal(err)
	}

	// Create our own encryption wrapper and break things apart
	// UnwrapRequest
	if diff := deep.Equal(clear, string(decrypted)); diff != nil {
		t.Fatal(diff)
	}
}

// runPythonTest will use the set of test files to start a python process that will return
// the console output any/or an error.  The caller can specify the max size of the buffer
// used to hold the last (keepLines) of lines from the console.  If tmpDir is specified
// that directory will be used to run the process and will not be removed after the
// function completes, in the case it is blank then the function will generate a directory
// run the python in it then remove it.
//
func runPythonTest(testFiles map[string]os.FileMode, tmpDir string, keepLines uint) (output []string, err kv.Error) {

	output = []string{}

	// I fthe optional temporary directory is not supplied then we create,
	// use it and then remove it, this allows callers to load data into the
	// directory they supply if they wish
	if len(tmpDir) == 0 {
		// Create a new TMPDIR because the python pip tends to leave dirt behind
		// when doing pip builds etc
		t, errGo := ioutil.TempDir("", "")
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer func() {
			os.RemoveAll(t)
		}()
		tmpDir = t
	}

	expectedScript := ""

	for fn, mode := range testFiles {
		assetFN, errGo := filepath.Abs(fn)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		destFN, errGo := filepath.Abs(filepath.Join(tmpDir, filepath.Base(assetFN)))
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		if _, err := CopyFile(assetFN, destFN); err != nil {
			return nil, err
		}

		if errGo = os.Chmod(destFN, mode); errGo != nil {
			return nil, kv.Wrap(errGo).With("destFN", destFN, "stack", stack.Trace().TrimRuntime())
		}

		if mode == 0700 {
			expectedScript = destFN
		}
	}

	// Save the output from the run using the last say 10 lines as a default otherwise
	// use the callers specified number of lines if they specified any
	if keepLines == 0 {
		keepLines = 10
	}
	output = make([]string, keepLines, keepLines)

	// Now setup is done execute the experiment
	dataC := make(chan *string, 1)
	go func() {
		for {
			select {
			case line := <-dataC:
				if line == nil {
					return
				}
				// Push to the back of the stack of lines, then pop from the front
				output = append(output, *line)
				output = output[1:]
			}
		}
	}()

	return output, CmdRun(context.TODO(), expectedScript, dataC)
}
