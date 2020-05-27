package runner

// This file contains the unit tests fot the message signing
// features of the runner

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ericchiang/k8s"
	core "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	"github.com/rs/xid"
)

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

// TestSignatureWatch exercises the signature file event watching feature.  This
// feature monitors a directory for signature files appearing and disappearing
// as an administrator manipulates the message signature public keys that will
// be used to authenticate that messages for the runner are genuine.
func TestSignatureWatch(t *testing.T) {
	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if err := IsAliveK8s(); err != nil {
		t.Fatal(err)
	}

	done, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			case <-done.Done():
				return
			}
		}
	}()

	// The directory location is the standard one for our containers inside Kubernetes
	// for mounting signatures from the signature 'secret' resource
	go InitSignatures(done, "/runner/certs/queues/signing", errorC)

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
	fmt.Println("change public key")
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
	fmt.Println("delete public key")
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
	fmt.Println("add public key")
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
