// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains a number of tests related to JSON document management including
// encoding, decoding, and encryption of payloads.

func TestEnvelopeDetectNeg(t *testing.T) {
	// Read and load a default unencrypted payload
	payload, errGo := ioutil.ReadFile(filepath.Join(*topDir, "assets/stock/plain_text.json"))
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	// Test IsEnvelope
	if isEnvelope, _ := IsEnvelope(payload); isEnvelope {
		t.Fatal(kv.NewError("mis-recognized envelope").With("stack", stack.Trace().TrimRuntime()))
	}

	// Read an encrypted payload
	// Test IsEnvelope
}

func TestEnvelopeDetectPos(t *testing.T) {
	// Read and load an encrypted payload
	payload, errGo := ioutil.ReadFile(filepath.Join(*topDir, "assets/stock/encrypted.json"))
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	// Test IsEnvelope
	if isEnvelope, err := IsEnvelope(payload); !isEnvelope {
		t.Fatal(kv.NewError("unrecognized envelope").With("stack", stack.Trace().TrimRuntime()))
	} else {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnvelopeCrypt(t *testing.T) {
	// Read and load a default unencrypted payload
	payload, errGo := ioutil.ReadFile(filepath.Join(*topDir, "assets/stock/plain_text.json"))
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	r, err := UnmarshalRequest(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt envelope with full message using PEMs that are self generated
	// for our test
	passphrase := RandomString(64)
	privatePEM, publicPEM, err := GenerateKeyPair(passphrase)
	wrapper := &Wrapper{
		privatePEM: privatePEM,
		publicPEM:  publicPEM,
		passphrase: passphrase,
	}
	encrypted, err := wrapper.WrapRequest(r)
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt envelope check against original
	rUnwrapped, err := wrapper.UnwrapRequest(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rUnwrapped.Marshal()
	if err != nil {
		t.Fatal(err)
	}
}
