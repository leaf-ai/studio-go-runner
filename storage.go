package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage
import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Storage struct {
	project string
	bucket  string
	client  *storage.Client
}

func (*Storage) getCred() (opts option.ClientOption, err error) {
	val, isPresent := os.LookupEnv("GOOGLE_FIREBASE_CREDENTIALS")
	if !isPresent {
		return nil, fmt.Errorf(`the environment variable GOOGLE_FIREBASE_CREDENTIALS was not set,
		fix this by saving your firebase credentials.  To do this use the Firebase Admin SDK 
		panel inside the Project Settings menu and then navigate to Setting -> Service Accounts 
		section.  This panel gives the option of generating private keys for your account.  
		Creating a key will overwrite any existing key. Save the generated key into a safe 
		location and define an environment variable GOOGLE_FIREBASE_CREDENTIALS to point at this file`)
	}
	return option.WithServiceAccountFile(val), nil
}

func NewStorage(projectID string, bucket string, validate bool, timeout time.Duration) (s *Storage, err error) {

	s = &Storage{
		project: projectID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cred, err := s.getCred()
	if err != nil {
		return nil, err
	}

	if s.client, err = storage.NewClient(ctx, cred); err != nil {
		return nil, err
	}

	if validate {
		// Validate the bucket during the NewBucket to give an early warning of issues
		buckets := s.client.Buckets(ctx, projectID)
		for {
			attrs, err := buckets.Next()
			if err == iterator.Done {
				return nil, fmt.Errorf("project %s bucket %s not found", projectID, bucket)
			}
			if err != nil {
				return nil, err
			}
			if attrs.Name == bucket {
				break
			}
		}
	}

	s.bucket = bucket
	return s, nil
}

func (s *Storage) Close() {
	s.client.Close()
}

func (s *Storage) Retrieve(timeout time.Duration) (links []string, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	objs := s.client.Bucket(s.bucket).Objects(ctx, nil)
	for {
		attrs, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		links = append(links, attrs.MediaLink)
	}
	return links, nil
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
func (s *Storage) Fetch(name string, unpack bool, output string, timeout time.Duration) (err error) {

	// Make sure output is an existing directory
	info, err := os.Stat(output)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, err := s.client.Bucket(s.bucket).Object(name).NewReader(ctx)
	if err != nil {
		return err
	}
	defer obj.Close()

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {

		var outw io.Reader
		if strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".tar.gzip") {
			if outw, err = gzip.NewReader(obj); err != nil {
				return err
			}
		} else {
			outw = bufio.NewReader(obj)
		}

		tarReader := tar.NewReader(outw)

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}

			path := filepath.Join(output, header.Name)
			info := header.FileInfo()
			if info.IsDir() {
				if err = os.MkdirAll(path, info.Mode()); err != nil {
					return err
				}
				continue
			}

			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}

			_, err = io.Copy(file, tarReader)
			file.Close()
			if err != nil {
				return err
			}
		}
	} else {
		f, err := os.Create(filepath.Join(output, name))
		if err != nil {
			return err
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		_, err = io.Copy(outf, obj)
		if err != nil {
			return err
		}
		outf.Flush()
	}
	return nil
}

// Return directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *Storage) Return(src string, dest string) (err error) {
	// Bundle the artifact directory into a compressed tar file

	return nil
}
