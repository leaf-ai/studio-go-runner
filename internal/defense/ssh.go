// Copyright 2020-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package defense

// This file contains functions to assist in wrangling SSH signatures
// for encoding and decoding etc to achieve parity with Python Paramiko
// clients etc

import (
	"encoding/binary"

	"golang.org/x/crypto/ssh"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func parseString(in []byte) (out, rest []byte, err kv.Error) {
	if len(in) < 4 {
		return out, rest, kv.NewError("bad length").With("stack", stack.Trace().TrimRuntime())
	}
	length := binary.BigEndian.Uint32(in)
	in = in[4:]
	if uint32(len(in)) < length {
		return out, rest, kv.NewError("truncated data").With("stack", stack.Trace().TrimRuntime())
	}
	out = in[:length]
	rest = in[length:]

	return out, rest, nil
}

// ParseSSHSignature is used to extract a signature from a byte buffer encoded
// formatted according to, https://tools.ietf.org/html/draft-ietf-curdle-ssh-ed25519-01.
// A pair of Length,Value items.  The first the Format string for the signature
// and the second the bytes of the key blob.
//
func ParseSSHSignature(in []byte) (out *ssh.Signature, err kv.Error) {
	format, in, err := parseString(in)
	if err != nil {
		return nil, err
	}

	out = &ssh.Signature{
		Format: string(format),
	}

	out.Blob, _, err = parseString(in)
	return out, err
}
