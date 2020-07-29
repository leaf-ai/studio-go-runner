// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of a public key store that is used
// by clients of the system to sign their messages being sent across queue
// infrastructure
//
import (
	"bytes"
	"context"
	"fmt"
	"time"

	"io/ioutil"
	"sync"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"golang.org/x/crypto/ssh"
)

type Signatures struct {
	store *DynamicStore
	sync.Mutex
}

func (s *Signatures) Reset() {
	s.store.Reset()
}

func extractPubKey(data []byte) (key interface{}, err kv.Error) {
	if !bytes.HasPrefix(data, []byte("ssh-ed25519 ")) {
		return key, kv.NewError("no ssh-ed25519 prefix").With("stack", stack.Trace().TrimRuntime())
	}

	pub, _, _, _, errGo := ssh.ParseAuthorizedKey(data)
	if errGo != nil {
		return key, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if pub.Type() != ssh.KeyAlgoED25519 {
		return key, kv.NewError("not ssh-ed25519").With("stack", stack.Trace().TrimRuntime())
	}
	return pub, nil
}

func (s *Signatures) update(fn string) (err kv.Error) {
	return s.store.update(fn)
}

// getFingerprint can be used to have the fingerprint of a file containing a pem formatted rsa public key.
// A base64 string of the binary finger print will be returned.
//
func getFingerprint(fn string) (fingerprint string, err kv.Error) {
	data, errGo := ioutil.ReadFile(fn)
	if errGo != nil {
		return "", kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	key, err := extractPubKey(data)
	if err != nil {
		return "", err.With("filename", fn)
	}

	return ssh.FingerprintSHA256(key.(ssh.PublicKey)), nil
}

// GetRefresh will return a context that will be cancelled on
// the next refresh of signatures completing.  This us principally for testing
// at this time
//
func (s *Signatures) GetRefresh() (doneCtx context.Context) {
	return s.store.getRefresh()
}

// Dir returns the absolute directory path from which signature files are being
// retrieved and used
func (s *Signatures) Dir() (dir string) {
	return s.store.getDir()
}

// Get retrieves a signature that has a queue name supplied by the caller
// as an exact match
//
func (s *Signatures) Get(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	item, err := s.store.get(q)

	if err != nil {
		return nil, "", err
	}

	return item.(ssh.PublicKey), ssh.FingerprintSHA256(item.(ssh.PublicKey)), nil
}

// Select retrieves a signature that has a queue name supplied by the caller
// using the longest prefix matched queue name for the supplied queue name
// that can be found.
//
func (s *Signatures) Select(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	item, err := s.store.selection(q)
	if err != nil {
		return nil, "", err
	}
	return item.(ssh.PublicKey), ssh.FingerprintSHA256(item.(ssh.PublicKey)), nil
}

func reportErr(err kv.Error, errorC chan<- kv.Error) {
	if err == nil {
		return
	}

	// Remove the entry for this function from the stack
	stk := stack.Trace().TrimRuntime()[1:]

	defer func() {
		_ = recover()
		if err != nil {
			fmt.Println(err.With("stack", stk).Error())
		}
	}()

	// Try to send the error and backoff to simply printing it if
	// we could not send it to the reporting module
	select {
	case errorC <- err.With("stack", stk):
	case <-time.After(time.Second):
		fmt.Println(err.With("stack", stk).Error())
	}
}

// InitSignatures is used to initialize a watch for signatures and to spawn the file system backed
// service function to perform the watching.
//
func InitSignatures(ctx context.Context, configuredDir string, errorC chan<- kv.Error) (sigs *Signatures, err kv.Error) {
	sigs = &Signatures{}
	sigs.store, err = NewDynamicStore(ctx, configuredDir, extractPubKey, errorC)
	return sigs, err
}
