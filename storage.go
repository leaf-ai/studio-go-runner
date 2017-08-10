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
	"io/ioutil"
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

		var inReader io.ReadCloser
		if strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".tar.gzip") {
			if inReader, err = gzip.NewReader(obj); err != nil {
				return err
			}
		} else {
			inReader = ioutil.NopCloser(bufio.NewReader(obj))
		}
		defer inReader.Close()

		tarReader := tar.NewReader(inReader)

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

func writeToTar(tw *tar.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if stat, err := file.Stat(); err == nil {
		// now lets create the header as needed for this file within the tarball
		header := new(tar.Header)
		header.Name = path
		header.Size = stat.Size()
		header.Mode = int64(stat.Mode())
		header.ModTime = stat.ModTime()
		// write the header to the tarball archive
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		// copy the file data to the tarball
		if _, err := io.Copy(tw, file); err != nil {
			return err
		}
	}
	return nil
}

// Return directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *Storage) Return(src string, dest string, timeout time.Duration) (err error) {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj := s.client.Bucket(s.bucket).Object(dest).NewWriter(ctx)
	if err != nil {
		return err
	}
	defer obj.Close()

	var outw io.Writer

	if strings.HasSuffix(dest, ".tgz") ||
		strings.HasSuffix(dest, ".tar.gz") ||
		strings.HasSuffix(dest, ".tar.gzip") {
		outZ := gzip.NewWriter(obj)
		defer outZ.Close()
		outw = outZ
	} else {
		outw = bufio.NewWriter(obj)
	}

	tw := tar.NewWriter(outw)
	defer tw.Close()

	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		link := ""
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			if link, err = os.Readlink(file); err != nil {
				return err
			}
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

		// write the header
		if err = tw.WriteHeader(header); err != nil {
			return err
		}

		// return on directories since there will be no content to tar, only headers
		if !fi.Mode().IsRegular() {
			return nil
		}

		// open files for taring
		f, err := os.Open(file)
		defer f.Close()
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		return nil
	})
}
