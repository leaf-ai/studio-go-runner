// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"path/filepath"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// GenerateTestKeys will generate a prtivate.pem and public.prm file in the nominated directory
// containing PEM formatted RSA keys with a passphrase if supplied.
//
// Never use keys generated this way in production.
//
func GenerateTestKeys(dir string, bitSize int, passphrase string) (err kv.Error) {
	reader := rand.Reader

	key, errGo := rsa.GenerateKey(reader, bitSize)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if err = exportPrivatePEM(filepath.Join(dir, "private.pem"), passphrase, key); err != nil {
		return err
	}

	if err = exportPublicPEM(filepath.Join(dir, "public.pem"), key.PublicKey); err != nil {
		return err
	}

	return nil
}

func exportPrivatePEM(fn string, passphrase string, key *rsa.PrivateKey) (err kv.Error) {
	out, errGo := os.Create(fn)
	if errGo != nil {
		return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
	}
	defer out.Close()

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if passphrase != "" {
		block, errGo = x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(passphrase), x509.PEMCipherAES256)
		if errGo != nil {
			return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
		}
	}

	if errGo = pem.Encode(out, block); errGo != nil {
		return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func exportPublicPEM(fn string, pubkey rsa.PublicKey) (err kv.Error) {
	asn1Bytes, errGo := asn1.Marshal(pubkey)
	if errGo != nil {
		return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
	}

	output, errGo := os.Create(fn)
	if errGo != nil {
		return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
	}
	defer output.Close()

	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: asn1Bytes,
	}

	if errGo = pem.Encode(output, block); errGo != nil {
		return kv.Wrap(errGo).With("filename", fn, "stack", stack.Trace().TrimRuntime())
	}
	return nil
}
