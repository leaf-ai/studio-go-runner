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

	"github.com/davecgh/go-spew/spew"
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
		// os.RemoveAll(tmpDir)
		fmt.Println(tmpDir)
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

	expectedScript := ""

	for fn, mode := range testFiles {
		assetFN, errGo := filepath.Abs(fn)
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}

		destFN, errGo := filepath.Abs(filepath.Join(tmpDir, filepath.Base(assetFN)))
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}

		if _, err := CopyFile(assetFN, destFN); err != nil {
			t.Fatal(err)
		}

		if errGo = os.Chmod(destFN, mode); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("destFN", destFN, "stack", stack.Trace().TrimRuntime()))
		}

		if mode == 0700 {
			expectedScript = destFN
		}
	}

	// Save the output from the run using the last say 10 lines
	lineStack := make([]string, 10, 10)

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
				lineStack = append(lineStack, *line)
				lineStack = lineStack[1:]
				fmt.Println(*line)
			}
		}
	}()

	if err := CmdRun(context.TODO(), expectedScript, dataC); err != nil {
		fmt.Println(spew.Sdump(lineStack))
		t.Fatal(err)
	}
	clear := lineStack[len(lineStack)-2]
	encrypted := lineStack[len(lineStack)-1]

	w, err := NewWrapper(publicPEM, privatePEM, []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := w.unwrapRaw(encrypted)
	if err != nil {
		t.Fatal(err)
	}

	// Create our own encryption wrapper and break things apart
	// UnwrapRequest
	if diff := deep.Equal(clear, string(decrypted)); diff != nil {
		t.Fatal(diff)
	}
}
