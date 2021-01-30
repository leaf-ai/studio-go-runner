// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package defense

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains a number of abberviated functions for handling and using SSH
// style RSA key pairs.  It enforces that the max 4096 bits will be used and a
// passphrase also will be used.
//
// This code is used mainly in the tests.

// encryptPrivateKeyToPEM takes a private key and generates a PEM block for it
// that has been encrypted using the supplied password, pwd.
func encryptPrivateKeyToPEM(privateKey *rsa.PrivateKey, pwd string) (block *pem.Block, err kv.Error) {
	block = &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if len(pwd) == 0 {
		return nil, kv.NewError("passphrase must be specified").With("stack", stack.Trace().TrimRuntime())
	}

	encBlock, errGo := x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(pwd), x509.PEMCipherAES256)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	block = encBlock

	return block, nil
}

// generatePrivateKey generates a rsa.PrivateKey and SSH style .pub file content
func generatePrivateKey(pwd string) (privateKey *rsa.PrivateKey, privatePEM []byte, err kv.Error) {
	if len(pwd) == 0 {
		return nil, nil, kv.NewError("passphrase must be specified").With("stack", stack.Trace().TrimRuntime())
	}

	privateKey, errGo := rsa.GenerateKey(rand.Reader, 4096)
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = privateKey.Validate(); errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	block, err := encryptPrivateKeyToPEM(privateKey, pwd)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, pem.EncodeToMemory(block), nil
}

// extractPublicPEM transforms a rsa.PublicKey to PEM style .pub file content
func extractPublicPEM(privateKey *rsa.PrivateKey) (publicPEM []byte, err kv.Error) {
	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
		}), nil
}

// generateKeyPair produces a 4096 bit PEM formatted public and password protected RSA key pair
func GenerateKeyPair(pwd string) (privatePEM []byte, publicPEM []byte, err kv.Error) {
	privateKey, privatePEM, err := generatePrivateKey(pwd)
	if err != nil {
		return nil, nil, err
	}
	publicPEM, err = extractPublicPEM(privateKey)
	if err != nil {
		return nil, nil, err
	}
	return privatePEM, publicPEM, nil
}

func WriteKeyToFile(keyBytes []byte, outputFN string) (err kv.Error) {
	if len(keyBytes) == 0 {
		return kv.NewError("empty key").With("output-file", outputFN).With("stack", stack.Trace().TrimRuntime())
	}
	if _, err := os.Stat(outputFN); err == nil || os.IsExist(err) {
		return kv.NewError("file exists already").With("output-file", outputFN).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo := ioutil.WriteFile(outputFN, keyBytes, 0600); errGo != nil {
		return kv.Wrap(errGo).With("output-file", outputFN).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}
