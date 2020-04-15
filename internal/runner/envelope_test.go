// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"

	"github.com/go-test/deep"
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

func setupWrapper() (w *Wrapper, err kv.Error) {
	w = &Wrapper{
		passphrase: RandomString(64),
	}
	w.privatePEM, w.publicPEM, err = GenerateKeyPair(w.passphrase)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func TestEnvelopeConv(t *testing.T) {
	// Read and load a default unencrypted payload
	payload, errGo := ioutil.ReadFile(filepath.Join(*topDir, "assets/stock/plain_text.json"))
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	r, err := UnmarshalRequest(payload)
	if err != nil {
		t.Fatal(err)
	}

	wrapper, err := setupWrapper()
	if err != nil {
		t.Fatal(err)
	}

	// Push the request to an envelope and then back again
	e, err := wrapper.Envelope(r)
	if err != nil {
		t.Fatal(err)
	}

	rFinal, err := wrapper.Request(e)
	if err != nil {
		t.Fatal(err)
	}

	// Do a deep equal test as the first check
	if diff := deep.Equal(r, rFinal); diff != nil {
		t.Fatal(diff)
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

	wrapper, err := setupWrapper()
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt envelope with full message using PEMs that are self generated
	// for our test
	encrypted, err := wrapper.WrapRequest(r)
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt envelope check against original
	rUnwrapped, err := wrapper.UnwrapRequest(encrypted)
	if err != nil {
		t.Fatal(err)
	}

	// Do a deep equal test as the first check
	if diff := deep.Equal(r, rUnwrapped); diff != nil {
		t.Fatal(diff)
	}

	finalPayload, err := rUnwrapped.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	// Repack the original request so that they can be compared without
	// the pretty print getting in the way
	minifiedRequest, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(minifiedRequest, finalPayload) != 0 {
		t.Fatal(kv.NewError("in/out payloads mismatched").With("stack", stack.Trace().TrimRuntime()))
	}
}
