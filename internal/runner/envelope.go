// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the implementation of an envelop message that will be used to
// add the chrome for holding StudioML requests and their respective signature attachment
// and encryption wrappers.

type OpenExperiment struct {
	Status    string `json:"status"`
	PythonVer string `json:"pthonver"`
}

// Message contains any clear text fields and either an an encrypted payload or clear text
// payloads as a Request.
type Message struct {
	Experiment         OpenExperiment `json:"experiment"`
	TimeAdded          float64        `json:"time_added"`
	ExperimentLifetime string         `json:"experiment_lifetime"`
	Resources          Resources      `json:"resources_needed"`
	Payload            string         `json:"payload"`
}

// Request marshals the requests made by studioML under which all of the other
// meta data can be found
type Envelope struct {
	Message   Message `json:"message"`
	Signature string  `json:"signature"`
}

// IsEnvelop is used to test if a JSON payload is indeed present
func IsEnvelope(msg []byte) (isEnvelope bool, err kv.Error) {
	fields := map[string]interface{}{}
	if errGo := json.Unmarshal(msg, &fields); errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	// Examine the fields and see that we have a message
	if _, isPresent := fields["message"]; !isPresent {
		return false, kv.NewError("'message' missing").With("stack", stack.Trace().TrimRuntime())
	}
	message, _ := fields["message"].(map[string]interface{})
	if _, isPresent := message["payload"]; !isPresent {
		return false, kv.NewError("'message.payload' missing").With("stack", stack.Trace().TrimRuntime())

	}
	return true, nil
}

// UnmarshalRequest takes an encoded StudioML envelope and extracts it
// into go data structures used by the go runner.
//
func UnmarshalEnvelope(data []byte) (e *Envelope, err kv.Error) {
	e = &Envelope{}
	if errGo := json.Unmarshal(data, e); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return e, nil
}

// Marshal takes the go data structure used to define a StudioML experiment envelope
// and serializes it as json to the byte array
//
func (e *Envelope) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

type Wrapper struct {
	publicPEM  []byte
	privatePEM []byte
	passphrase string
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

	fmt.Println(len(asymKeyB64), asymKeyB64)

	// append the encrypted semtric key, and the symetrically encrypted data into a BASE64 result
	return asymKeyB64 + "," + asymDataB64, nil
}

func (w *Wrapper) UnwrapRequest(encrypted string) (r *Request, err kv.Error) {
	// Check we have a private key and a passphrase
	if w == nil {
		return nil, kv.NewError("wrapper missing").With("stack", stack.Trace().TrimRuntime())
	}

	// Check to see if we have a private key, and a passphrase
	if len(w.privatePEM) == 0 {
		return nil, kv.NewError("private key missing").With("stack", stack.Trace().TrimRuntime())
	}
	if len(w.passphrase) == 0 {
		return nil, kv.NewError("passphrase missing").With("stack", stack.Trace().TrimRuntime())
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
	prvBlock, _ := pem.Decode(w.privatePEM)
	if prvBlock == nil {
		return nil, kv.NewError("private PEM not decoded").With("stack", stack.Trace().TrimRuntime())
	}
	if got, want := prvBlock.Type, "RSA PRIVATE KEY"; got != want {
		return nil, kv.NewError("unknown block type").With("got", got, "want", want).With("stack", stack.Trace().TrimRuntime())
	}

	decryptedBlock, errGo := x509.DecryptPEMBlock(prvBlock, []byte(w.passphrase))
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	prv, errGo := x509.ParsePKCS1PrivateKey(decryptedBlock)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	asymSliceKey, errGo := rsa.DecryptOAEP(sha256.New(), rand.Reader, prv, asymKeyDecoded, nil)
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
