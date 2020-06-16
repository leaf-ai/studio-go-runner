package runner

// This file contains the unit tests fot the message signing
// features of the runner

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ericchiang/k8s"
	core "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	"github.com/rs/xid"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

var (
	sigWatchDone = context.Background()
	initSigWatch sync.Once
)

func InitSigWatch() {
	StartSigWatch(sigWatchDone, "/runner/certs/queues/signing")
}

func StartSigWatch(ctx context.Context, sigDir string) {

	errorC := make(chan kv.Error)
	defer close(errorC)

	go func() {
		for {
			select {
			case err := <-errorC:
				if err == nil {
					return
				}
				fmt.Println(err.Error())
			case <-ctx.Done():
				return
			}
		}
	}()

	// The directory location is the standard one for our containers inside Kubernetes
	// for mounting signatures from the signature 'secret' resource
	go InitSignatures(ctx, sigDir, errorC)
}

// TestFingerprint does an expected value test for the SHA256 fingerprint
// generation facilities in Go for our purposes.
//
func TestSignatureFingerprint(t *testing.T) {
	pKey := []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFITo06Pk8sqCMoMHPaQiQ7BY3pjf7OE8BDcsnYozmIG kmutch@awsdev")

	expected := "SHA256:rM9uPGQWiB8BrF542H5tJdVQoWU2+jw00w1KnXjywTY"

	// Create a new TMPDIR so that we can cleanup
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	testFN := filepath.Join(tmpDir, "public_key.pub")
	if errGo := ioutil.WriteFile(testFN, pKey, 0600); errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("filename", testFN).With("stack", stack.Trace().TrimRuntime()))
	}
	fp, err := getFingerprint(testFN)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(expected, fp); diff != nil {
		t.Fatal(diff)
	}
}

func generateTestKey() (publicKey ssh.PublicKey, fp string, err kv.Error) {
	pubKey, _, errGo := ed25519.GenerateKey(rand.Reader)
	if errGo != nil {
		return nil, "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	sshKey, errGo := ssh.NewPublicKey(pubKey)
	if errGo != nil {
		return nil, "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return sshKey, ssh.FingerprintSHA256(sshKey), nil
}

// TestSignatureCascade will add signatures to the signature config map and will
// then run a series of queries against the runners internal record of signatures
// and queues and will validate the correct selection of partial queue names that
// were selected.  For this test we will use a temporary directory to populate
// signatures.
//
func TestSignatureCascade(t *testing.T) {

	// Create a directory to be used with signatures
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer os.RemoveAll(dir)

	watchDone, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()

	StartSigWatch(watchDone, dir)

	// Contains all of the matches to be attempted that are not exact matches
	attemptMatches := map[string]string{
		"r":                 "",
		"rmq_z":             "rmq_",
		"rmq_donn_":         "rmq_donn",
		"rmq_andrei_andrei": "rmq_andrei",
		"rmq_karlx":         "rmq_karl",
	}

	// Queue names against which we are going to add public keys
	queues := []string{"rmq_", "rmq_karl", "rmq_andrei", "rmq_k", "rmq_ka", "rmq_kar", "rmq_donn", "rmq_do"}

	type keyTracker struct {
		q   string
		fp  string
		key ssh.PublicKey
	}
	keys := map[string]keyTracker{}

	for _, q := range queues {
		// Now write a set of test files to be used for selecting signatures, and record
		// the data we have written to exercise the signatures implementation
		pubKey, fp, err := generateTestKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[q] = keyTracker{
			q:   q,
			fp:  fp,
			key: pubKey,
		}

		// Write the secrets to files
		fn := filepath.Join(dir, q)
		if errGo = ioutil.WriteFile(fn, ssh.MarshalAuthorizedKey(pubKey), 0600); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("file", fn).With("stack", stack.Trace().TrimRuntime()))
		}
		//signatures.Data[newKey] = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFITo06Pk8sqCMoMHPaQiQ7BY3pjf7OE8BDcsnYozmIG kmutch@awsdev")
		//expectedFingerprint := "SHA256:rM9uPGQWiB8BrF542H5tJdVQoWU2+jw00w1KnXjywTY"
		// ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDA/bv8Usu/5rqUk6mJnYMD0gXgXn/8gQpcnVR4dt4tm

		//SHA256:VV6NzLszADZ+PHkzK0k3TntaksOmiv4o9rJ3s0mrJ6U

	}

	// Get access to the signature store
	sigs := GetSignatures()

	// Wait for the signature store to refresh itself with our new files
	<-GetSignaturesRefresh().Done()

	// Go through the queue names looking for matches
	for _, aCase := range keys {
		key, fp, err := sigs.Get(aCase.q)
		if err != nil {
			t.Fatal(err)
		}
		if diff := deep.Equal(aCase.fp, fp); diff != nil {
			t.Fatal(diff)
		}
		if diff := deep.Equal(aCase.key, key); diff != nil {
			t.Fatal(diff)
		}
		key, fp, err = sigs.Select(aCase.q)
		if err != nil {
			t.Fatal(err)
		}
		if diff := deep.Equal(aCase.fp, fp); diff != nil {
			t.Fatal(diff)
		}
		if diff := deep.Equal(aCase.key, key); diff != nil {
			t.Fatal(diff)
		}
	}

	// Go through the queue names looking for prefixes
	for prefix, qExpect := range attemptMatches {
		key, _, err := sigs.Get(prefix)
		if err == nil {
			t.Fatal(kv.NewError("expected error, error not returned").With("prefix", prefix, "queueExpected", qExpect).With("stack", stack.Trace().TrimRuntime()))
		}
		if key != nil {
			t.Fatal(kv.NewError("key found, expected error").With("prefix", prefix, "queueExpected", qExpect).With("stack", stack.Trace().TrimRuntime()))
		}

		key, fp, err := sigs.Select(prefix)
		if key == nil && err != nil && len(qExpect) == 0 {
			continue
		}
		if len(qExpect) == 0 && key == nil {
			if err == nil {
				t.Fatal(kv.NewError("expected error, error not returned").With("prefix", prefix, "queueExpected", qExpect).With("stack", stack.Trace().TrimRuntime()))
			}
			continue
		}

		expectedKey := keys[qExpect].key
		if diff := deep.Equal(key, expectedKey); diff != nil {
			fmt.Println("Test case", "prefix", prefix, "queueExpected", qExpect)
			t.Fatal(diff)
		}
		if diff := deep.Equal(fp, keys[qExpect].fp); diff != nil {
			t.Fatal(diff)
		}
		if diff := deep.Equal(qExpect, keys[qExpect].q); diff != nil {
			t.Fatal(diff)
		}
	}
}

// TestSignatureWatch exercises the signature file event watching feature.  This
// feature monitors a directory for signature files appearing and disappearing
// as an administrator manipulates the message signature public keys that will
// be used to authenticate that messages for the runner are genuine.
//
func TestSignatureWatch(t *testing.T) {
	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if err := IsAliveK8s(); err != nil {
		t.Fatal(err)
	}

	// Start a signature watcher that will output any errors or failures
	// in the background
	initSigWatch.Do(InitSigWatch)

	// The downward API within K8s is configured within the build YAML
	// to pass the pods namespace into the pods environment table.
	namespace, isPresent := os.LookupEnv("K8S_NAMESPACE")
	if !isPresent {
		t.Fatal(kv.NewError("K8S_NAMESPACE missing").With("stack", stack.Trace().TrimRuntime()))
	}

	// Get access to the signature store
	sigs := GetSignatures()

	// Start a ticker that will be used throughout this test
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	// Use the kubernetes client to modify the config map and then
	// check the signature store
	// K8s API receiver to be used to manipulate the config maps we are testing
	client, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		t.Fatal(errGo)
	}

	signatures := &core.Secret{}
	if errGo = client.Get(context.Background(), namespace, "studioml-signing", signatures); errGo != nil {
		t.Fatal(errGo)
	}

	// Add a key
	newKey := xid.New().String()
	signatures.Data[newKey] = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFITo06Pk8sqCMoMHPaQiQ7BY3pjf7OE8BDcsnYozmIG kmutch@awsdev")
	expectedFingerprint := "SHA256:rM9uPGQWiB8BrF542H5tJdVQoWU2+jw00w1KnXjywTY"

	if errGo := client.Update(context.Background(), signatures); errGo != nil {
		t.Fatal(errGo)
	}
	// Wait for the key to appear in the signatures collection
	func() {
		for {
			select {
			case <-tick.C:
				_, fp, err := sigs.Get(newKey)
				if err != nil {
					continue
				}
				if diff := deep.Equal(expectedFingerprint, fp); diff != nil {
					t.Fatal(diff)
				}
				return
			}
		}
	}()

	// Change a key
	signatures.Data[newKey] = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKohNVg9rRRrUlOSdksrXczWzuR9jN+NE2ZpX2Myw+k9 kmutch@awsdev")
	expectedFingerprint = "SHA256:0Q8tSkwT/m8p4eAsUIFDUfonQZyleEla5nFQCvWE5lk"

	if errGo := client.Update(context.Background(), signatures); errGo != nil {
		t.Fatal(errGo)
	}
	// Wait for the key to change its value in the signatures collection
	func() {
		for {
			select {
			case <-tick.C:
				_, fp, err := sigs.Get(newKey)
				if err != nil {
					t.Fatal(err)
				}
				if diff := deep.Equal(expectedFingerprint, fp); diff == nil {
					return
				}
			}
		}
	}()

	// Delete a Key
	delete(signatures.Data, newKey)
	if errGo := client.Update(context.Background(), signatures); errGo != nil {
		t.Fatal(errGo)
	}
	// Wait for the key to disappear from the signatures collection
	func() {
		for {
			select {
			case <-tick.C:
				_, _, err := sigs.Get(newKey)
				if err != nil {
					return
				}
			}
		}
	}()

	// Add a key
	signatures.Data[newKey] = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFITo06Pk8sqCMoMHPaQiQ7BY3pjf7OE8BDcsnYozmIG kmutch@awsdev")
	expectedFingerprint = "SHA256:rM9uPGQWiB8BrF542H5tJdVQoWU2+jw00w1KnXjywTY"

	if errGo := client.Update(context.Background(), signatures); errGo != nil {
		t.Fatal(errGo)
	}
	// Wait for the key to appear a second time in the signatures collection
	func() {
		for {
			select {
			case <-tick.C:
				_, fp, err := sigs.Get(newKey)
				if err != nil {
					continue
				}
				if diff := deep.Equal(expectedFingerprint, fp); diff != nil {
					t.Fatal(diff)
				}
				return
			}
		}
	}()

	// Purge the data we used from the signatures
	delete(signatures.Data, newKey)
	if errGo := client.Update(context.Background(), signatures); errGo != nil {
		t.Fatal(errGo)
	}
}
