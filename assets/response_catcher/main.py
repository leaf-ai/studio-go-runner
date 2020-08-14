"""
A RabbitMQ python module to catch messages on a studio-go-runner
response queue
"""
import argparse


def main():
    """
    Main entry point to the response message catcher
    """

    # Initiate the parser
    parser = argparse.ArgumentParser()
    parser.add_argument("-V", "--version", help="show program version", action="store_true")
    parser.add_argument("--private-key", "-k", help="the file name of the private key file for decrypting responses")
    parser.add_argument("--output", "-o", help="the file name for output of the decrypted response messages (contents "
                        "of which are JSON formatted)")

    # Read arguments from the command line
    args = parser.parse_args()

    # Check for --version or -V
    if args.version:
        print("0.0")

    print("Hello World")


if __name__ == "__main__":
    main()
