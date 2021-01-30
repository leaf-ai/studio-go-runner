// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package defense

import (
	"crypto/rand"
	"io"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"

	"golang.org/x/crypto/nacl/secretbox"
)

// This file contains code to enable the encryption and decryption of larger
// blocks of data than RSA encryption is suited for.  These functions
// allow symetric key encryption on larger blocks of data with the expectation
// that the symmetric key will ne encrypted using an asymmetric RSA and placed
// at the front of a data block or file.
//
// This set of functions use the nacl compatible secretbox implementation to
// encrypt the contents of the payload with the secret key being obtained
// from an RSA encrypted header at the start of the bytes block.
//
// This package is interoperable with NaCl: https://nacl.cr.yp.to/secretbox.html.
//
// This package works using a 32 byte secret key, a 24 byte nonce, and then the payload.

// EncryptBlock will generate a nonce, and a secret and will encrypted the supplied
// data using the generated key bundling a 24 byte nonce with it returning the key,
// and the encoded data with the nonce prepended to it.

func EncryptBlock(data []byte) (key [32]byte, enc []byte, err kv.Error) {
	key = [32]byte{}
	if _, errGo := io.ReadFull(rand.Reader, key[:]); errGo != nil {
		return key, nil, kv.Wrap(errGo, "secret could not be generated").With("stack", stack.Trace().TrimRuntime())
	}

	nonce := [24]byte{}
	if _, errGo := io.ReadFull(rand.Reader, nonce[:]); errGo != nil {
		return key, nil, kv.Wrap(errGo, "nonce could not be generated").With("stack", stack.Trace().TrimRuntime())
	}

	encrypted := secretbox.Seal(nonce[:], data, &nonce, &key)

	return key, encrypted, nil
}

func DecryptBlock(key [32]byte, in []byte) (clear []byte, err kv.Error) {

	decryptNonce := [24]byte{}
	copy(decryptNonce[:], in[:24])

	decrypted, ok := secretbox.Open(nil, in[24:], &decryptNonce, &key)
	if !ok {
		return nil, kv.NewError("decryption failure").With("stack", stack.Trace().TrimRuntime())
	}

	return decrypted, nil
}
