# Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.
#
import os
import sys
import paramiko
import base64
import traceback


class Signer:
    """
    Implementation for experiment payload builder
    using private key ed25519 SSH signing.
    """
    def __init__(self, key_fn: str):
        """
        param: keypath - file path to .pem file with public key
        """

        key_path = os.path.abspath(key_fn)
        self.key = None
        try:
            self.key = paramiko.Ed25519Key.from_private_key_file(filename=key_path)
        except Exception as ex:
            print('FAILED to import private key file: {} {}'.format(key_path, traceback.format_exc(ex)))
            os.exit(-1)

    def _sign_str(self, payload: str):
        if self.key is None:
            print('signing key is missing')
            os.exit(-1)
        sig = self.key.sign_ssh_data(bytes(payload, 'utf-8'))
        if isinstance(sig, paramiko.Message):
            sig = sig.asbytes()
        return base64.b64encode(sig).decode()

    def sign(self, payload: str):
        return self._sign_str(payload)


def main():
    if len(sys.argv) < 4:
        print('USAGE {} private-key-file-path file-to-sign signature-file-path'.format(sys.argv[0]))
        sys.exit(-1)

    with open(sys.argv[3], 'w+') as f:
        signer = Signer(sys.argv[1])
        with open(sys.argv[2], 'r') as file:
            data = file.read()
        try:
            result = signer.sign(data)
        except Exception as ex:
            f.write('FAILED to sign data {}'.format(ex))
            sys.exit(-1)

        f.write(result)


if __name__ == '__main__':
    main()
