# Message Encryption

This section describes the message encryption feature of the runner.  Encryption of the message payloads are described in the docs/interface.md file.  Encryption is only supported within Kubernetes deployments.

# Key creation

```
echo "PassPhrase" > secret_phrase
ssh-keygen -t rsa -b 4096 -f studioml_message -C "Message Encryption Key" -N "PassPhrase"
ssh-keygen -t rsa -b 4096 -C "Example RSA" -f studioml_message.pub -e -m PEM > studioml_message.pub.pem
kubectl create secret generic ssh-key-secret --from-file=ssh-privatekey=studioml_message --from-file=ssh-publickey=studioml_message.pub
kubectl create secret generic ssh-passphrase --from-file=ssh-passphrase=secret_phrase
```

The passphrase is kept in a seperate secret to enable RBAC access to be used to isolate the two pieces of knowledge should your secrets management procedures call for this.

