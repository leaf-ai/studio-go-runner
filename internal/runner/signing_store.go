// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of a public key store that is used
// by clients of the system to sign their messages being sent across queue
// infrastructure
//
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"io/ioutil"
	"sync"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"golang.org/x/crypto/ssh"
)

type Signatures struct {
	sigs map[string]ssh.PublicKey // The known public keys retrieved from the secrets mount directory
	dir  string                   // Secrets mount directory
	sync.Mutex
}

type RefreshContext struct {
	ctx    context.Context
	cancel context.CancelFunc
	sync.Mutex
}

var (
	// signatures contains a map with the index being the prefix of queue names and their public keys
	signatures = Signatures{
		sigs: map[string]ssh.PublicKey{},
	}

	refreshContext = RefreshContext{}
)

func init() {
	ctx, cancel := context.WithCancel(context.Background())
	refreshContext = RefreshContext{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (refresh *RefreshContext) Reset() {
	refresh.Lock()
	defer refresh.Unlock()

	refresh.cancel()
	refresh.ctx, refresh.cancel = context.WithCancel(context.Background())
}

func extractPubKey(data []byte) (key ssh.PublicKey, err kv.Error) {
	if !bytes.HasPrefix(data, []byte("ssh-ed25519 ")) {
		return key, kv.NewError("no ssh-ed25519 prefix").With("stack", stack.Trace().TrimRuntime())
	}

	pub, _, _, _, errGo := ssh.ParseAuthorizedKey(data)
	if errGo != nil {
		return key, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if pub.Type() != ssh.KeyAlgoED25519 {
		return key, kv.NewError("not ssh-ed25519").With("stack", stack.Trace().TrimRuntime())
	}
	return pub, nil
}

func (s *Signatures) update(fn string) (err kv.Error) {
	data, errGo := ioutil.ReadFile(fn)
	if errGo != nil {
		if os.IsNotExist(errGo) {
			s.Lock()
			delete(s.sigs, filepath.Base(fn))
			s.Unlock()
			return nil
		}
		return kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	pub, err := extractPubKey(data)
	if err != nil {
		return err.With("filename", fn)
	}

	s.Lock()
	s.sigs[filepath.Base(fn)] = pub
	s.Unlock()

	return nil
}

// getFingerprint can be used to have the fingerprint of a file containing a pem formatted rsa public key.
// A base64 string of the binary finger print will be returned.
//
func getFingerprint(fn string) (fingerprint string, err kv.Error) {
	data, errGo := ioutil.ReadFile(fn)
	if errGo != nil {
		return "", kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	key, err := extractPubKey(data)
	if err != nil {
		return "", err.With("filename", fn)
	}

	return ssh.FingerprintSHA256(key), nil
}

// GetSignatures returns the signing public key struct for accessing
// methods related to signature selection etc.
//
func GetSignatures() (s *Signatures) {
	return &signatures
}

// GetSignaturesRefresh will return a context that will be cancelled on
// the next refresh of signatures completing.  This us principally for testing
// at this time
//
func GetSignaturesRefresh() (doneCtx context.Context) {
	refreshContext.Lock()
	defer refreshContext.Unlock()

	return refreshContext.ctx
}

// Dir returns the absolute directory path from which signature files are being
// retrieved and used
func (s *Signatures) Dir() (dir string) {
	signatures.Lock()
	defer signatures.Unlock()

	return signatures.dir
}

// Get retrieves a signature that has a queue name supplied by the caller
// as an exact match
//
func (s *Signatures) Get(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	s.Lock()
	key, isPresent := s.sigs[q]
	s.Unlock()

	if !isPresent {
		return nil, "", kv.NewError("not found").With("queue", q).With("stack", stack.Trace().TrimRuntime())
	}
	return key, ssh.FingerprintSHA256(key), nil
}

// Get retrieves a signature that has a queue name supplied by the caller
// using the longest prefix matched queue name for the supplied queue name
// that can be found.
//
func (s *Signatures) Select(q string) (key ssh.PublicKey, fingerprint string, err kv.Error) {
	// The lock is kept until we are done to ensure once a prefix is matched to its longest length
	// that we still have the public key for it
	s.Lock()
	defer s.Unlock()
	prefixes := make([]string, 0, len(s.sigs))
	for k := range s.sigs {
		prefixes = append(prefixes, k)
	}
	sort.Strings(prefixes)

	// Start with no valid match as a prefix
	bestMatch := ""
	wouldBeAt := 0

	// Roll through the sorted prefixes while there is a still a valid signature name prefix of the q (queue)
	// names, stop when the q supplied no longer satisfies the prefix and the one prior would be
	// the shortest signature prefix of the q name.
	for {
		if prefixes[wouldBeAt] == q {
			bestMatch = prefixes[wouldBeAt]
			break
		}
		if strings.HasPrefix(q, prefixes[wouldBeAt]) {
			if len(bestMatch) == 0 || len(bestMatch) < len(prefixes[wouldBeAt]) {
				bestMatch = prefixes[wouldBeAt]
			}
		}
		if wouldBeAt += 1; wouldBeAt >= len(prefixes) {
			break
		}
	}

	if len(bestMatch) == 0 {
		return nil, "", kv.NewError("not found").With("queue", q).With("stack", stack.Trace().TrimRuntime())
	}
	key = s.sigs[bestMatch]
	return key, ssh.FingerprintSHA256(key), nil
}

func reportErr(err kv.Error, errorC chan<- kv.Error) {
	if err == nil {
		return
	}

	// Remove the entry for this function from the stack
	stk := stack.Trace().TrimRuntime()[1:]

	defer func() {
		_ = recover()
		if err != nil {
			fmt.Println(err.With("stack", stk).Error())
		}
	}()

	// Try to send the error and backoff to simply printing it if
	// we could not send it to the reporting module
	select {
	case errorC <- err.With("stack", stk):
	case <-time.After(time.Second):
		fmt.Println(err.With("stack", stk).Error())
	}
}

// InitSignatures is used to initialize a watch for signatures
func InitSignatures(ctx context.Context, configuredDir string, errorC chan<- kv.Error) {

	dir, errGo := filepath.Abs(configuredDir)
	if errGo != nil {
		reportErr(kv.Wrap(errGo).With("dir", dir), errorC)
	}

	// Wait until the directory exists and accessed at least once
	updatedEntries, errGo := ioutil.ReadDir(dir)
	// Record the last modified time for the file representing a signature key
	entries := make(map[string]time.Time, len(updatedEntries))

	// Set the last time an error was reported to more then 15 minutes ago so
	// that the first error is displayed immediately
	lastErrNotify := time.Now().Add(-1 * time.Hour)

	// Wait until we get at least one good read from the
	// directory being monitored for signatures
	for {
		if errGo == nil {
			break
		}

		// Only display this particular error
		if time.Since(lastErrNotify) > time.Duration(15*time.Minute) {
			if errGo != nil {
				reportErr(kv.Wrap(errGo).With("dir", dir), errorC)
			}
			lastErrNotify = time.Now()
		}

		select {
		case <-time.After(10 * time.Second):
			_, errGo = ioutil.ReadDir(dir)
		case <-ctx.Done():
			return
		}
	}

	// Once we know we have a working signatures storage directory save its location
	// so that test software can inject certificates of their own when running
	// with a production server under test
	signatures.Lock()
	signatures.dir = dir
	signatures.Unlock()

	// Event loop for the watcher until the server shuts down
	for {
		select {

		case <-time.After(10 * time.Second):

			// It is possible that the signatures store is changed during runtime
			// so refresh the location
			signatures.Lock()
			dir = signatures.dir
			signatures.Unlock()

			// A lookaside collection for checking the presence of directory entries
			// that are no longer found on the disk
			deletionCheck := make(map[string]time.Time, len(entries))

			if updatedEntries, errGo = ioutil.ReadDir(dir); errGo != nil {
				reportErr(kv.Wrap(errGo).With("dir", dir), errorC)
				continue
			}

			for _, entry := range updatedEntries {

				if entry.IsDir() {
					continue
				}

				if entry.Name()[0] == '.' {
					continue
				}

				// Symbolic link checking
				if entry.Mode()&os.ModeSymlink != 0 {
					target, errGo := filepath.EvalSymlinks(filepath.Join(dir, entry.Name()))
					if errGo != nil {
						reportErr(kv.Wrap(errGo).With("dir", dir, "target", entry.Name()), errorC)
						continue
					}
					if entry, errGo = os.Stat(target); errGo != nil {
						reportErr(kv.Wrap(errGo).With("dir", dir, "target", entry.Name()), errorC)
						continue
					}
				}

				curEntry, isPresent := entries[entry.Name()]
				if !isPresent || curEntry.Round(time.Second) != entry.ModTime().Round(time.Second) {
					entries[entry.Name()] = entry.ModTime().Round(time.Second)
					if err := signatures.update(filepath.Join(dir, entry.Name())); err != nil {
						// info is a special file that is used to prevent the secret from not
						// being created by Kubernetes when there are no secrets to be mounted
						if entry.Name() != "info" {
							reportErr(err, errorC)
						}
					}
				}

				deletionCheck[entry.Name()] = curEntry
			}
			for name := range entries {
				if _, isPresent := deletionCheck[name]; !isPresent {
					// Have the update method check for the presence of the file,
					// it will cleanup if the file is not found
					signatures.update(filepath.Join(dir, name))
					// Now remove the missing from our small lookaside collection
					delete(entries, name)
				}
			}

			// Signal any waiters that the refresh has been processed and replace the context
			// used for this with a new one that can be waited on by observers
			refreshContext.Reset()

		case <-ctx.Done():
			return
		}
	}
}
