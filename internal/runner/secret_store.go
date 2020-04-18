// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"

	"github.com/awnumar/memguard"
)

// This file contains the implementation of a credentials structure that has been
// encrypted while in memory.

var (
	serverSecret = &memguard.Enclave{}
)

func init() {
	// Safely terminate in case of an interrupt signal
	memguard.CatchInterrupt()

	// Generate a key sealed inside an encrypted container
	serverSecret = memguard.NewEnclaveRandom(32)
}

func StopSecret() {
	// Purge the session when we return
	defer memguard.Purge()
}

type Wrapper struct {
	publicPEM  []byte
	privateKey *rsa.PrivateKey
}

// KubertesWrapper is used to obtain, if available, the Kubernetes stored encryption
// parameters for the server
func KubernetesWrapper() (w *Wrapper, err kv.Error) {
	/**
	request := &SSHKeyRequest{
		keyNS:            "default",
		keySecret:        "studioml-runner-key-secret",
		privateName:      "ssh-privatekey",
		publicName:       "ssh-publickey",
		passphraseNS:     "default",
		passphraseSecret: "studioml-runner-passphrase-secret",
		passphrase:       "sh-passphrase",
	}
	**/

	publicPEM, privatePEM, passphrase, err := SSHKeys("certs/message/encryption", "certs/message/passphrase")
	if err != nil {
		return nil, err
	}

	return NewWrapper(publicPEM, privatePEM, passphrase)
}

func SSHKeys(cryptoDir string, passphraseDir string) (publicPEM []byte, privatePEM []byte, passphrase []byte, err kv.Error) {

	if err = IsAliveK8s(); err != nil {
		return nil, nil, nil, nil
	}

	// First make sure all the appropriate mounts exist
	info, errGo := os.Stat(cryptoDir)
	if errGo == nil {
		if !info.IsDir() {
			return nil, nil, nil, kv.NewError("not a directory").With("dir", cryptoDir).With("stack", stack.Trace().TrimRuntime())
		}
	} else {
		return nil, nil, nil, kv.Wrap(err).With("dir", cryptoDir).With("stack", stack.Trace().TrimRuntime())
	}
	if info, errGo := os.Stat(passphraseDir); errGo == nil {
		if !info.IsDir() {
			return nil, nil, nil, kv.NewError("not a directory").With("dir", passphraseDir).With("stack", stack.Trace().TrimRuntime())
		}
	} else {
		return nil, nil, nil, kv.Wrap(err).With("dir", passphraseDir).With("stack", stack.Trace().TrimRuntime())
	}

	// We have ether directories at least needed to create our secrets, read in the PEMs and passphrase

	if publicPEM, errGo = ioutil.ReadFile(filepath.Join(cryptoDir, "ssh-publickey")); errGo != nil {
		return nil, nil, nil, kv.Wrap(errGo).With("dir", passphraseDir).With("stack", stack.Trace().TrimRuntime())
	}
	if privatePEM, errGo = ioutil.ReadFile(filepath.Join(cryptoDir, "ssh-privatekey")); errGo != nil {
		return nil, nil, nil, kv.Wrap(errGo).With("dir", passphraseDir).With("stack", stack.Trace().TrimRuntime())
	}
	if passphrase, errGo = ioutil.ReadFile(filepath.Join(passphraseDir, "ssh-passphrase")); errGo != nil {
		return nil, nil, nil, kv.Wrap(errGo).With("dir", passphraseDir).With("stack", stack.Trace().TrimRuntime())
	}
	return publicPEM, privatePEM, passphrase, nil
}

func NewWrapper(publicPEM []byte, privatePEM []byte, passphrase []byte) (w *Wrapper, err kv.Error) {
	w = &Wrapper{
		publicPEM: publicPEM,
	}
	// Decrypt the RSA encrypted asymmetric key
	prvBlock, _ := pem.Decode(privatePEM)
	if prvBlock == nil {
		return nil, kv.NewError("private PEM not decoded").With("stack", stack.Trace().TrimRuntime())
	}
	if got, want := prvBlock.Type, "RSA PRIVATE KEY"; got != want {
		return nil, kv.NewError("unknown block type").With("got", got, "want", want).With("stack", stack.Trace().TrimRuntime())
	}

	// TODO Place the enclave handling here
	decryptedBlock, errGo := x509.DecryptPEMBlock(prvBlock, passphrase)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("phrase", passphrase).With("stack", stack.Trace().TrimRuntime())
	}

	// TODO Place the enclave handling here
	w.privateKey, errGo = x509.ParsePKCS1PrivateKey(decryptedBlock)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	w.privateKey.Precompute()

	return w, nil
}

func (w *Wrapper) getPrivateKey() (privateKey *rsa.PrivateKey, err kv.Error) {
	return w.privateKey, nil
}

func (w *Wrapper) WrapRequest(r *Request) (encrypted string, err kv.Error) {

	if w == nil {
		return "", kv.NewError("wrapper missing").With("stack", stack.Trace().TrimRuntime())
	}

	// Check to see if we have a public key
	if len(w.publicPEM) == 0 {
		return "", kv.NewError("public key missing").With("stack", stack.Trace().TrimRuntime())
	}

	// Serialize the request
	buffer, err := r.Marshal()
	if err != nil {
		return "", err
	}
	pubBlock, _ := pem.Decode(w.publicPEM)
	pub, errGo := x509.ParsePKCS1PublicKey(pubBlock.Bytes)
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// encrypt the data and retrieve a symmetric key
	asymKey, asymData, err := EncryptBlock(buffer)
	if err != nil {
		return "", err
	}
	asymDataB64 := base64.StdEncoding.EncodeToString(asymData)

	// encrypt the symmetric key using the public RSA PEM
	asymEncKey, errGo := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, asymKey[:], nil)
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	asymKeyB64 := base64.StdEncoding.EncodeToString(asymEncKey)

	// append the encrypted semtric key, and the symetrically encrypted data into a BASE64 result
	return asymKeyB64 + "," + asymDataB64, nil
}

func (w *Wrapper) UnwrapRequest(encrypted string) (r *Request, err kv.Error) {
	// Check we have a private key and a passphrase
	if w == nil {
		return nil, kv.NewError("wrapper missing").With("stack", stack.Trace().TrimRuntime())
	}

	// break off the fixed length symetric but RSA encrypted key using the comma delimiter
	items := strings.Split(encrypted, ",")
	if len(items) > 2 {
		return nil, kv.NewError("too many values in encrypted data").With("stack", stack.Trace().TrimRuntime())
	}
	if len(items) < 2 {
		return nil, kv.NewError("missing values in encrypted data").With("stack", stack.Trace().TrimRuntime())
	}

	asymKeyDecoded, errGo := base64.StdEncoding.DecodeString(items[0])
	if errGo != nil {
		return nil, kv.Wrap(errGo, "asymmetric key bad").With("stack", stack.Trace().TrimRuntime())
	}
	asymBodyDecoded, errGo := base64.StdEncoding.DecodeString(items[1])
	if errGo != nil {
		return nil, kv.Wrap(errGo, "asymmetric encrypted data bad").With("stack", stack.Trace().TrimRuntime())
	}

	// Decrypt the RSA encrypted asymmetric key
	prvKey, err := w.getPrivateKey()
	if err != nil {
		return nil, err
	}
	asymSliceKey, errGo := rsa.DecryptOAEP(sha256.New(), rand.Reader, prvKey, asymKeyDecoded, nil)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	asymKey := [32]byte{}
	copy(asymKey[:], asymSliceKey[:32])

	// Decrypt the data using the decrypted asymmetric key
	decryptedBody, err := DecryptBlock(asymKey, asymBodyDecoded)
	if err != nil {
		return nil, err
	}
	r, errGo = UnmarshalRequest(decryptedBody)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return r, nil
}

func (w *Wrapper) Envelope(r *Request) (e *Envelope, err kv.Error) {
	e = &Envelope{
		Message: Message{
			Experiment: OpenExperiment{
				Status:    r.Experiment.Status,
				PythonVer: r.Experiment.PythonVer,
			},
			TimeAdded:          r.Experiment.TimeAdded,
			ExperimentLifetime: r.Config.Lifetime,
			Resource:           r.Experiment.Resource,
		},
	}

	e.Message.Payload, err = w.WrapRequest(r)
	return e, err
}

func (w *Wrapper) Request(e *Envelope) (r *Request, err kv.Error) {
	return w.UnwrapRequest(e.Message.Payload)
}
