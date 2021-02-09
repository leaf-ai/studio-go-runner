// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package defense

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"

	random "github.com/leaf-ai/studio-go-runner/pkg/rand"
)

// This file contains a number of tests related to handling key files for use in
// Encryption of the messages being used by the runner.

// TestRSA will test the encryption and decryption of short
// blocks of data, typically used for encryption of symetrics
// keys embeeded within messages etc
//
func TestRSA(t *testing.T) {
	passphrase := random.RandomString(10)
	privatePEM, publicPEM, err := GenerateKeyPair(passphrase)
	if err != nil {
		t.Fatal(err.With("stack", stack.Trace().TrimRuntime()))
	}

	// Extract the PEM-encoded data block
	pubBlock, _ := pem.Decode(publicPEM)
	if pubBlock == nil {
		t.Fatal(kv.NewError("public PEM not decoded").With("stack", stack.Trace().TrimRuntime()))
	}
	if got, want := pubBlock.Type, "RSA PUBLIC KEY"; got != want {
		t.Fatal(kv.NewError("unknown block type").With("got", got, "want", want).With("stack", stack.Trace().TrimRuntime()))
	}

	pub, errGo := x509.ParsePKCS1PublicKey(pubBlock.Bytes)
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	msg := []byte(random.RandomString(256))
	encrypted, errGo := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, msg, nil)
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	// Now we have the encrypted data, try decrypting it
	prvBlock, _ := pem.Decode(privatePEM)
	if prvBlock == nil {
		t.Fatal(kv.NewError("private PEM not decoded").With("stack", stack.Trace().TrimRuntime()))
	}
	if got, want := prvBlock.Type, "RSA PRIVATE KEY"; got != want {
		t.Fatal(kv.NewError("unknown block type").With("got", got, "want", want).With("stack", stack.Trace().TrimRuntime()))
	}

	decryptedBlock, errGo := x509.DecryptPEMBlock(prvBlock, []byte(passphrase))
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	prv, errGo := x509.ParsePKCS1PrivateKey(decryptedBlock)
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	out, errGo := rsa.DecryptOAEP(sha256.New(), rand.Reader, prv, encrypted, nil)
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	if 0 != bytes.Compare(msg, out) {
		t.Fatal(kv.NewError("roundtrip failed").With("stack", stack.Trace().TrimRuntime()))
	}
}
