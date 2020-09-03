"""
Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

A RabbitMQ python module to catch messages on a studio-go-runner
response queue.

A simple invocation might appear as follows

python3 main.py --private-key=example-test-key --password=PassPhrase -q=test /
    -r="amqp://guest:guest@localhost:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5" \
    --output=responses.txt

"""
import argparse
import os
import sys
import pika
import time
import base64

from Crypto.PublicKey import RSA
from Crypto.Cipher import PKCS1_OAEP
from Crypto.Hash import SHA256

import nacl.secret
import nacl.encoding

from google.protobuf.json_format import MessageToJson
import google.protobuf.text_format as text_format
import reports_pb2 as reports


def initialize(cipher, rmq_url, rmq_queue, output=sys.stdout):

    # Connect to the rabbitMQ server
    connection = pika.BlockingConnection(pika.URLParameters(rmq_url))
    channel = connection.channel()

    # Create a queue that we expect studioml will place encrypted response messages on
    channel.queue_declare(queue=rmq_queue)

    # start the receiver
    last_empty = time.time()
    while True:
        try:
            method_frame, header_frame, body = channel.basic_get(rmq_queue, auto_ack=True)
        except pika.exceptions.ChannelClosedByBroker as ex:
            print(f"RMQ error, {ex}")
            return 0
        except ValueError as ex:
            print(f"RMQ file error, {ex}")
            return 0
        if method_frame:
            last_empty = time.time()
            # Split the message into two pieces the first is the Encrypted symetric key, encrypted using
            # a private key from a public/private key pair.  The second being the encrypted gRPC message
            # encrypted using the first symetric key
            parts = body.decode("utf-8").split(",")
            symKeyBytes = base64.b64decode(parts[0])
            # msgBytes has a nonce in the first 24 bytes and then the message payload follows that
            msgBytes = base64.b64decode(parts[1])

            try:
                msgKey = cipher.decrypt(symKeyBytes)
            except ValueError as ex:
                print(f"crypto sym key error, {ex}")
                return 0
            except TypeError as ex:
                print(f"crypto sym key error, {ex}")
                return -1

            box = nacl.secret.SecretBox(msgKey, nacl.encoding.RawEncoder)
            unencrypted = box.decrypt(msgBytes[nacl.secret.SecretBox.NONCE_SIZE:], msgBytes[0:nacl.secret.SecretBox.NONCE_SIZE],
                                      nacl.encoding.RawEncoder)

            report = reports.Report()
            text_format.Parse(unencrypted, report)
            print(' '.join(MessageToJson(report).splitlines()), file=output)
        else:
            seconds_elapsed = time.time() - last_empty

            hours, rest = divmod(seconds_elapsed, 3600)
            minutes, seconds = divmod(rest, 60)
            if hours > 0 or minutes > 2:
                last_empty = time.time()
                print('Queue empty')
            time.sleep(0.3)


def main():
    """
    Main entry point to the response message catcher
    """

    # Initiate the parser
    parser = argparse.ArgumentParser()
    required = parser.add_argument_group('required arguments')
    optional = parser.add_argument_group('optional arguments')

    parser.add_argument("-V", "--version", help="show program version", action="store_true")
    required.add_argument("--private-key", "-k", help="the file name of the private key file for decrypting responses",
                          required=True)
    optional.add_argument("--password", "-p", help="the password that should be used with the private key file")
    required.add_argument("--rmq-url", "-r", help="the fully qualified RabbitMQ host with username and password", required=True)
    required.add_argument("--rmq-queue", "-q", help="the RabbitMQ queue name to be used to message receiving", required=True)
    optional.add_argument("--output", "-o", help="the file name for output of the decrypted response messages (contents "
                          "of which are JSON formatted)")

    # Read arguments from the command line
    args = parser.parse_args()

    # Check for --version or -V
    if args.version:
        print("0.0")
        return 0

    if not args.private_key:
        print(f"private key file option used for decryption not specified\n", file=sys.stderr)
        print(parser.print_help())
        return -1

    if not os.path.exists(args.private_key):
        print(f"file {args.private_key} not found", file=sys.stderr)
        return -1

    if not args.rmq_url:
        print(f"--rmq-url not specified", file=sys.stderr)
        return -1

    if not args.rmq_queue:
        print(f"--rmq-queue not specified", file=sys.stderr)
        return -1

    output = sys.stdout
    if args.output:
        if args.output != "-":
            try:
                output = open(args.output, 'w')
            except Exception as ex:
                print(f"output file error, {ex}", file=sys.stderr)
                return -1

    prvt_key = RSA.importKey(open(args.private_key, 'rb').read(), args.password)
    cipher = PKCS1_OAEP.new(prvt_key, SHA256)

    # Blocking function
    return initialize(cipher, args.rmq_url, args.rmq_queue, output)


if __name__ == "__main__":
    sys.exit(main())
