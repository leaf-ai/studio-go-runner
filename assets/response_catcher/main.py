"""
A RabbitMQ python module to catch messages on a studio-go-runner
response queue.

A simple invocation might appear as follows

python3 main.py --private-key=example-test-key --password=PassPhrase -q=test /
    -r="amqp://guest:guest@localhost:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5"

"""
import argparse
import os
import sys
import pika
import time

from Crypto.PublicKey import RSA
from Crypto.Cipher import PKCS1_OAEP


def initialize(cipher, rmq_url, rmq_queue, output=sys.stdout):
    """
    encrypted = ""

    plaintext = unseal_box.decrypt(encrypted)
    print(plaintext.decode('utf-8'))
    """

    print("Hello World", file=output)

    # Connect to the rabbitMQ server
    connection = pika.BlockingConnection(pika.URLParameters(rmq_url))
    channel = connection.channel()

    # Create a queue that we expect studioml will place encrypted response messages on
    channel.queue_declare(queue=rmq_queue)

    # start the receiver
    last_empty = time()
    while True:
        method_frame, header_frame, body = channel.basic_get(rmq_queue, auto_ack=True)
        if method_frame:
            last_empty = time()
            print(method_frame, header_frame, body)
            channel.basic_ack(method_frame.delivery_tag)
        else:
            seconds_elapsed = time() - last_empty

            hours, rest = divmod(seconds_elapsed, 3600)
            minutes, seconds = divmod(rest, 60)
            if hours > 0 or minutes > 5:
                last_empty = time()
                print('Queue empty')
            time.sleep(1)


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

    output = sys.stdout
    if args.output:
        if args.output != "-":
            try:
                output = open(args.output, 'w')
            except Exception as ex:
                print(f"output file error, {ex}", file=sys.stderr)
                return -1

    prvt_key = RSA.importKey(open(args.private_key, 'rb').read(), args.password)
    cipher = PKCS1_OAEP.new(prvt_key)

    # Blocking function
    initialize(cipher, args.rmq_url, args.rmq_queue, output)


if __name__ == "__main__":
    sys.exit(main())
