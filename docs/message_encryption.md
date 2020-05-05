# Message Encryption

This section describes the message encryption, and signing features of the runner.  Message payloads are described in the docs/interface.md file.  Encryption, and signing is only supported within Kubernetes deployments.  The reason for this is that standalone runners cannot be secured and have shared secrets without the isolation provided by Kubernetes.

Encrypted payloads use a hybrid cryptosystem, [please click for a detailed description](https://en.wikipedia.org/wiki/Hybrid_cryptosystem).

Message signing uses Ed25519 signing as defined by RFC8032, more information can be found at[https://ed25519.cr.yp.to/](https://ed25519.cr.yp.to/).

Ed25519 certificate SHA1 fingerprints, not intended to be cryptographicaly secure, will be used by clients to assert identity, confirmed by successful verification.a  Verification still relies on a full public key.

<!--ts-->

Table of Contents
=================

* [Message Encryption](#message-encryption)
* [Table of Contents](#table-of-contents)
* [Introduction](#introduction)
* [Encryption](#encryption)
  * [Key creation by the cluster owner](#key-creation-by-the-cluster-owner)
* [Mount secrets into runner deployment](#mount-secrets-into-runner-deployment)
  * [Message format](#message-format)
* [Signing](#signing)
* [Python StudioML configuration](#python-studio-configuration)
<!--te-->

# Introduction

This document describes encryption of Request messages sent by StudioML clients to the runner.

Encryption of messages has two tiers, the first tier is a Public-key scheme that has the runner employ a private key and a public key that is given to experimenters using the python or other client software.

The concerns to users of the system is to obtain from the computer cluster owner the public key, and only the public key.  The public key can then be made accessible to the client for securing the messages exchanged with the runner compute instances.

The compute cluster owner will be resposible for generating the public-private key pair and manging the integrity of the private key.  They will also be responsible for distribution of the public key to any experiments, or users of the system.

The client encrypts a per message secret that is encrypted using the public key, and prepended to a payload that contains the request message encrypted using the secret.

# Encryption

## Key creation by the cluster owner

The owner of the compute cluster is responsible for the generation of key pair for use with the message encryption.  The following commands show the creation of the key pairs.

```
echo -n "PassPhrase" > secret_phrase
ssh-keygen -t rsa -b 4096 -f studioml_message -C "Message Encryption Key" -N "PassPhrase"
ssh-keygen -f studioml_message.pub -e -m PEM > studioml_message.pub.pem
cp studioml_message studioml_message.pem
ssh-keygen -f studioml_message.pem -e -m PEM -p -P "PassPhrase" -N "PassPhrase"
```

The private key file and the passphrase should be considered as valuable secrets for your organization that MUST be protected and cared for appropriately.

Once the keypair has been created they can be loaded into the Kubernetes runner cluster using the following commands:

```
kubectl create secret generic studioml-runner-key-secret --from-file=ssh-privatekey=studioml_message.pem --from-file=ssh-publickey=studioml_message.pub.pem
kubectl create secret generic studioml-runner-passphrase-secret --from-file=ssh-passphrase=secret_phrase
```

The passphrase is kept in a seperate secret to enable RBAC access to be used to isolate the two pieces of knowledge should your secrets management procedures call for this.

The public PEM key MUST be the only file delivered to client side users of StudioML in PEM Key file format, for example:

```
-----BEGIN RSA PUBLIC KEY-----
MIICCgKCAgEAtZurOEVuT9bhjiUWX7U8EFxL8oMGWSLXf4M6QBsJ5TljtSqyIxvI
kXiQDLIpJXY8KRmiR9RghGopvB5NfAMLZtfwozuju2NtnSn0UPI+6O4ED6TfDP5F
eta/6tUKAuvxVwF5Yvr7en1qnbv4L86vqeukrn/gIPTb7LlsFjt6uHlxA6xTAun/
HfRKlBiWR5rIi/fwuUMmTGpAcCa8s5Gqfla28FfsknGOipy4Vw4Mt7f93ke1dHN+
dY/J2TpCm/GNJuFaHc4EgHE8uw+jU6uBgpZAJSIzK5dxYniEjZS93CWxs2HN8dmV
wEqleT02agWW4cfa13X3Lz1YoQkCjYtSqB8Y2KjT1q7sSll0HExWV58kFPk9FmIy
JniMLcLFzAxGDM5UgtmsdSYmqN49vlqOejxfYxy6GrKXrkRGCDuQKyb2m/WQLXGU
8cGqwuVpN/JNWjiG4+NaxWRzfE2Yk4gbhcYqXRocNMlidG0Sx/xrFTFln86lmGJ1
RCse6jv3beENf5lfrz4ddAzAssjTivmlZgJCTK2oROT3WPI/G6CaBQadt13XkQLW
hAZDbnsZMhOVH3/UiQJ6DwgV0yK5FND4jkbHM3GWGNLRIrnL9F0I8c1p9X2oCx6T
plgCug3iz5cE9+G2455Y1vaVMBEKSm1REhsdTYzPBV/yXPpPR4lUCmkCAwEAAQ==
-----END RSA PUBLIC KEY-----
```

A single key pair is used to encrypt all requests on the cluster at this time.  A future feature is envisioned to allow multiple key pairs.

When the runner is run the secrets are mounted into the container that Kubernetes is managing.  This is done using the deployment yaml.  When performing deployments the yaml should be reviewed for runner pod, and their runner container to ensure that the secrets are available and that they are mounted.  If these secrets are not loaded into the cluster the runner pod should remain in a pending state.

# Mount secrets into runner deployment

Secrets used by the runner will be mounted into the runner pod using the Kubernetes deployment pod resource definition.  An example of this is provided within the sample AWS CPU runner that can be found in the [../examples/aws/cpu/deployment.yaml](../examples/aws/cpu/deployment.yaml) file.

Two mounts will be created firstly for the keyfiles, secondly for the passphrase.  These two are split to allow for RBAC to be employed in the cluster should you want it.  The motivation is that you might want to divide ownership between two parties for the private key and the and avoid revealing one of these to the other.

If you wish to use encrypted traffic exclusively be sure to remove the ```CLEAR_TEXT_MESSAGES: "true"``` entry from your ConfigMap entries in the yaml.

In any event the yaml need to mount these secrets appears as follows:

```
apiVersion: apps/v1
kind: Deployment
metadata:
 name: studioml-go-runner-deployment
 labels:
   app: studioml-go-runner
spec:
 ...
 template:
   ...
   spec:
      ...
      containers:
      - name: studioml-go-runner
        ...
        volumeMounts:
        - name: message-encryption
          mountPath: "/runner/certs/message/encryption"
          readOnly: true
        - name: encryption-passphrase
          mountPath: "/runner/certs/message/passphrase"
          readOnly: true
        ...
      volumes:
        ...
        - name: message-encryption
          secret:
            optional: false
            secretName: studioml-runner-key-secret
            items:
            - key: ssh-privatekey
              path: ssh-privatekey
            - key: ssh-publickey
              path: ssh-publickey
        - name: encryption-passphrase
          secret:
            optional: false
            secretName: studioml-runner-passphrase-secret
            items:
            - key: ssh-passphrase
              path: ssh-passphrase
```

## Message format

The encrypted\_data block contains two comma seperated Base64 strings.  The first string contains a symmetric key that is encrypted using RSA-OAEP with a key length of 4096 bits, and the sha256 hashing algorithm. The second field contains the JSON string for the Request message that is first encrypted using a NaCL SecretBox encryption and then encoded as Base64.

The encryption works in two steps, first the secretbox based symmetric shared key is generated for every message by the source generating the message.  The data within the messages is encrypted with the symmetric key.  The symmetric key is then encrypted and placed at the front of the message using an asymmetric key.  This has the following effects:

The sender can decrypt the payload if they retain their original symmetric key.
The sender can not decrypt the symmetric key, once it is placed encrypted into the payload
The legitimate runner if able to access the RSA PEM private key can decrypt the asymmetric key, and only then can subsequently decrypt the Request in the payload.
Evesdropping software cannot decrypt the asymmetricly encrypted secretbox key and so cannot decrypt the rest of the payload.

# Signing

Message signing is a way of protecting the runner receiving messages from processing spoofed requests.  To prevent this the runner can be configured to read public key information from Kubernetes secrets and then to use this to validate messages that are being received.  The configuration information for the runner signing keys is detailed in the [message_encryption.md](message_encryption.md) file.

Message signing uses Ed25519 signing as defined by RFC8032, more information can be found at[https://ed25519.cr.yp.to/](https://ed25519.cr.yp.to/).

Ed25519 certificate SHA1 fingerprints, not intended to be cryptographicaly secure, will be used by clients to assert identity, confirmed by successful verification.a  Verification still relies on a full public key.

```
openssl ecparam -genkey -name prime256v1 -noout -out studioml_signing.pem
openssl ec -in studioml_signing.pem -pubout -out studioml_signing.pub.pem
```

The finger print will need to be captured and this will appear something like the following:

```
openssl ec -in studioml_signing.pem -pubout -outform DER 2>/dev/null | openssl sha256 -binary | base64
4O6DVWmMVngBe9o6IQITV6nx+atnc/z9eUmbw+bc1m4=
```

# Python StudioML configuration

In order to use experiment payload encryption with Python-based StudioML client,
StudioML section of experiment configuration must specify
a path to public key file in PEM format. If such a path is not specified,
experiment payload will be submitted unencrypted, in plain text form.

StudioML configuration would include the following (example):

```
{
   ...
   "studio_ml_config": {
         ...
         "public_key_path": "/home/user/keys/my-key.pem",
         ...
   }
   ...
}
```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
