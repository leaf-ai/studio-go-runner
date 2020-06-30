// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
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

	// Grab known files from the crypto test library and place them into
	// our temporary test directory
	testFiles := map[string]os.FileMode{
		filepath.Join("..", "..", "assets", "crypto", "encryptor.py"): 0600,
		filepath.Join("..", "..", "assets", "crypto", "encryptor.sh"): 0700,
	}
	output, err := PythonRun(testFiles, tmpDir, 20)
	if err != nil {
		t.Fatal(err)
	}

	payload, errGo := ioutil.ReadFile(filepath.Join(tmpDir, "payload"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	lines := strings.Split(string(payload), "\n")
	clear := lines[0]
	encrypted := lines[1]

	w, err := NewWrapper(publicPEM, privatePEM, []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := w.unwrapRaw(encrypted)
	if err != nil {
		for _, aLine := range output {
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
