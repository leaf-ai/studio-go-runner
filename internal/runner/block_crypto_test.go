// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"strings"
	"testing"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
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
