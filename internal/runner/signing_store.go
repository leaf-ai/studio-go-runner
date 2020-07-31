// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of a public key store that is used
// by clients of the system to sign their messages being sent across queue
// infrastructure
//
import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"time"

	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// PubkeyStore encapsulates a store of SSH public keys used for message signing, and encryption
type PubkeyStore struct {
	store *DynamicStore
	sync.Mutex
}

// extractRqstSigning will be used when files on the back store are loaded in to the
// collection of contents
func extractRqstSigning(data []byte) (key interface{}, err kv.Error) {
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

// extractRspnsPubkey will be used when files on the back store are loaded in to the
// collection of contents
func extractRspnsPubkey(data []byte) (key interface{}, err kv.Error) {
	if !bytes.HasPrefix(data, []byte("-----BEGIN RSA PUBLIC KEY----- ")) {
		return nil, kv.NewError("no '-----BEGIN RSA PUBLIC KEY-----' prefix").With("stack", stack.Trace().TrimRuntime())
	}

	pubBlock, _ := pem.Decode(data)
	if pubBlock.Type != "RSA PUBLIC KEY" {
		return key, kv.NewError("no '-----BEGIN RSA PUBLIC KEY-----' prefix").With("stack", stack.Trace().TrimRuntime())
	}
	pub, errGo := x509.ParsePKCS1PublicKey(pubBlock.Bytes)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return pub, nil
}

// GetRefresh will return a context that will be cancelled on
// the next refresh of signatures completing.  This us principally for testing
// at this time
//
func (s *PubkeyStore) GetRefresh() (doneCtx context.Context) {
	return s.store.getRefresh()
}

// Dir returns the absolute directory path from which signature files are being
// retrieved and used
func (s *PubkeyStore) Dir() (dir string) {
	return s.store.getDir()
}

// GetSSH retrieves a signature that has a queue name supplied by the caller
// as an exact match
//
func (s *PubkeyStore) GetSSH(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	item, err := s.store.get(q)

	if err != nil {
		return nil, "", err
	}

	return item.(ssh.PublicKey), ssh.FingerprintSHA256(item.(ssh.PublicKey)), nil
}

// SelectSSH retrieves an SSH style signature that has a queue name supplied by the caller
// using the longest prefix matched queue name for the supplied queue name
// that can be found.
//
func (s *PubkeyStore) SelectSSH(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	item, err := s.store.selection(q)
	if err != nil {
		return nil, "", err
	}
	return item.(ssh.PublicKey), ssh.FingerprintSHA256(item.(ssh.PublicKey)), nil
}

// Select retrieves an SSH style signature that has a queue name supplied by the caller
// using the longest prefix matched queue name for the supplied queue name
// that can be found.
//
func (s *PubkeyStore) Select(q string) (key *rsa.PublicKey, err kv.Error) {
	item, err := s.store.selection(q)
	if err != nil {
		return nil, err
	}
	cast := item.(rsa.PublicKey)
	return &cast, nil
}

// InitRqstSigWatcher is used to initialize a watch for signatures and to spawn the file system backed
// service function to perform the watching.
//
func InitRqstSigWatcher(ctx context.Context, configuredDir string, errorC chan<- kv.Error) (sigs *PubkeyStore, err kv.Error) {
	sigs = &PubkeyStore{}
	sigs.store, err = NewDynamicStore(ctx, configuredDir, extractRqstSigning, time.Duration(10*time.Second), errorC)
	return sigs, err
}

// InitRspnsSigWatcher is used to initialize a watch for signatures and to spawn the file system backed
// service function to perform the watching.
//
func InitRspnsSigWatcher(ctx context.Context, configuredDir string, errorC chan<- kv.Error) (sigs *PubkeyStore, err kv.Error) {
	sigs = &PubkeyStore{}
	sigs.store, err = NewDynamicStore(ctx, configuredDir, extractRspnsPubkey, time.Duration(10*time.Second), errorC)
	return sigs, err
}
