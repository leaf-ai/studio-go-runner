// Copyright 2018-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package s3 // import "github.com/leaf-ai/studio-go-runner/internal/s3"

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"github.com/minio/minio-go/v7"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leaf-ai/go-service/pkg/archive"
	"github.com/leaf-ai/go-service/pkg/mime"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"github.com/leaf-ai/studio-go-runner/internal/request"

	bzip2w "github.com/dsnet/compress/bzip2"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	s3CA   = flag.String("s3-ca", "", "Used to specify a PEM file for the CA used in securing the S3/Minio connection")
	s3Cert = flag.String("s3-cert", "", "Used to specify a cert file for securing the S3/Minio connection, do not use with the s3-pem option")
	s3Key  = flag.String("s3-key", "", "Used to specify a key file for securing the S3/Minio connection, do not use with the s3-pem option")
)

var (
	numRetries = 6
	retryWait  = 3 * time.Second
)

// StorageImpl is a type that describes the implementation of an S3 storage entity
type StorageImpl int

type s3Storage struct {
	endpoint  string
	bucket    string
	key       string
	useSSL    bool
	creds     *request.AWSCredential
	transport *http.Transport
	client    *s3.Client
}

func (s *s3Storage) setRegion(env map[string]string) (err kv.Error) {
	// When using official S3 then the region will be encoded into the endpoint and in order to
	// prevent cross region authentication problems we will need to extract it and use the minio
	// New function and specify the region explicitly to reduce lookups, minio does
	// the processing to get a well known DNS name in these cases.
	//
	// For additional information about regions and naming for S3 endpoints please review the following,
	// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
	//
	// Use the default region that minio and AWS uses to start with
	region := "us-west-1"
	if envRegion := env["AWS_DEFAULT_REGION"]; len(envRegion) != 0 {
		region = envRegion
	}

	if len(s.creds.Region) > 0 {
		// Region is set directly in AWS credentials
		region = s.creds.Region
	}

	if s.endpoint != "s3.amazonaws.com" {
		if (strings.HasPrefix(s.endpoint, "s3-") || strings.HasPrefix(s.endpoint, "s3.")) &&
			strings.HasSuffix(s.endpoint, ".amazonaws.com") {
			region = s.endpoint[3:]
			region = strings.TrimSuffix(region, ".amazonaws.com")
			// Revert to a single well known address for DNS lookups to improve interoperability
			// when running in k8s etc
			s.endpoint = "s3.amazonaws.com"
			s.useSSL = true
		}
	}

	if len(region) == 0 {
		msg := "the AWS region is missing from the studioML request, and could not be deduced from the endpoint"
		return kv.NewError(msg).With("endpoint", s.endpoint).With("stack", stack.Trace().TrimRuntime())
	}

	s.creds.Region = region
	return nil
}

func (s *s3Storage) refreshClients() (err kv.Error) {
	if err = s.creds.Refresh(); err != nil {
		return err
	}
	// Using the BucketLookupPath strategy to avoid using DNS lookups for the buckets first
	// Do we have explicit static AWS credentials?

	var errGo error
	var cfg aws.Config
	if len(s.creds.AccessKey) > 0 && len(s.creds.SecretKey) > 0 {
		cfg, errGo = awsconfig.LoadDefaultConfig(
			context.TODO(),
			awsconfig.WithRegion(s.creds.Region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.creds.AccessKey, s.creds.SecretKey, "")),
		)
		if errGo != nil {
			return kv.Wrap(errGo).With("creds mode", "static", "endpoint", s.endpoint).With("stack", stack.Trace().TrimRuntime())
		}
	} else {
		cfg, errGo = awsconfig.LoadDefaultConfig(
			context.TODO(),
			awsconfig.WithRegion(s.creds.Region),
		)
		if errGo != nil {
			return kv.Wrap(errGo).With("creds mode", "default chain", "endpoint", s.endpoint).With("stack", stack.Trace().TrimRuntime())
		}
		return nil
	}
	s.client = s3.NewFromConfig(cfg)
	return nil
}

// NewS3storage is used to initialize a client that will communicate with S3 compatible storage.
func NewS3storage(ctx context.Context, creds request.AWSCredential, env map[string]string, endpoint string,
	bucket string, key string, validate bool, useSSL bool) (s *s3Storage, err kv.Error) {

	s = &s3Storage{
		endpoint: endpoint,
		bucket:   bucket,
		key:      key,
		useSSL:   useSSL,
		creds:    creds.Clone(),
	}

	if err = s.setRegion(env); err != nil {
		return nil, err.With("stack", stack.Trace().TrimRuntime())
	}

	// Set our initial AWS credentials,
	// in case they are represented by Vault reference
	tries := numRetries
	for tries > 0 {
		err = s.creds.Refresh()
		if err == nil {
			break
		}
		time.Sleep(retryWait)
		tries -= 1
	}
	if err != nil {
		return nil, kv.NewError("failed to get initial credentials").With("endpoint", endpoint).With("creds", creds)
	}

	// The use of SSL is mandated at this point to ensure that data protections
	// are effective when used by callers
	//
	pemData := []byte{}
	cert := tls.Certificate{}
	errGo := fmt.Errorf("")
	_ = errGo // Bypass the ineffectual assignment check

	if len(*s3Cert) != 0 || len(*s3Key) != 0 {
		if len(*s3Cert) == 0 || len(*s3Key) == 0 {
			return nil, kv.NewError("the s3-cert and s3-key files when used must both be specified")
		}
		if cert, errGo = tls.LoadX509KeyPair(*s3Cert, *s3Key); errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		s.useSSL = true
	}

	if len(*s3CA) != 0 {
		stat, errGo := os.Stat(*s3CA)
		if errGo != nil {
			return nil, kv.Wrap(errGo, "unable to read a PEM, or Certificate file from disk for S3 security")
		}
		if stat.Size() > 128*1024 {
			return nil, kv.NewError("the PEM, or Certificate file is suspiciously large, too large to be a PEM file")
		}
		if pemData, errGo = ioutil.ReadFile(*s3CA); errGo != nil {
			return nil, kv.Wrap(errGo, "PEM, or Certificate file read failed").With("stack", stack.Trace().TrimRuntime())

		}
		if len(pemData) == 0 {
			return nil, kv.NewError("PEM, or Certificate file was empty, PEM data is needed when the file name is specified")
		}
		s.useSSL = true
	}

	if s.useSSL {
		caCerts := &x509.CertPool{}

		if len(*s3CA) != 0 {
			if !caCerts.AppendCertsFromPEM(pemData) {
				return nil, kv.NewError("PEM Data could not be added to the system default certificate pool").With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			// First load the default CA's
			caCerts, errGo = x509.SystemCertPool()
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}

		s.transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCerts,
			},
		}
	}

	if err = s.refreshClients(); err != nil {
		return nil, err.With("stack", stack.Trace().TrimRuntime())
	}
	return s, nil
}

func (s *s3Storage) Close() {
}

func isAccessDenied(errGo error) bool {
	if errGo == nil {
		return false
	}
	msg := strings.Join(strings.Fields(strings.ToLower(errGo.Error())), " ")
	if strings.Contains(msg, "key") && strings.Contains(msg, "not exist") {
		return false
	}
	return true
}

func (s *s3Storage) waitAndRefreshClient() error {
	time.Sleep(retryWait)
	errGo := s.refreshClients()
	if errGo != nil {
		fmt.Printf(">>>>>REFRESH CLIENTS ERROR: %s [%v]\n", errGo.Error(), stack.Trace().TrimRuntime())
	}
	return errGo
}

func (s *s3Storage) retryGetObject(ctx context.Context, objectName string, opts minio.GetObjectOptions) (obj *minio.Object, size int64, err kv.Error) {

	defer func() {
		if err != nil {
			err = err.With("bucket", s.bucket).With("object", objectName)
		}
	}()

	var errGo error
	tries := numRetries
	for tries > 0 {
		obj, errGo = s.client.GetObject(ctx, s.bucket, objectName, opts)
		if errGo == nil {
			stat, errGoStat := obj.Stat()
			if errGoStat == nil {
				return obj, stat.Size, nil
			}
			errGo = errGoStat
		}
		fmt.Printf(">>>>>>>> retryGetObject ERROR %s/%s [%s]\n", s.bucket, objectName, errGo.Error())

		if isAccessDenied(errGo) {
			// Possible AWS credentials rotation, reset client and retry:
			s.waitAndRefreshClient()
			tries -= 1
		} else {
			return nil, 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	return nil, 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
}

type SrcProvider interface {
	getSource() (io.ReadCloser, int64, string, kv.Error)
}

func (s *s3Storage) retryPutObject(ctx context.Context, sp SrcProvider, dest string) (err kv.Error) {
	src, srcSize, srcName, err := sp.getSource()

	defer func() {
		if src != nil {
			src.Close()
		}
		if err != nil {
			err = err.With("src", srcName, "bucket", s.bucket, "key", dest).With("stack", stack.Trace().TrimRuntime())
		}
	}()

	if ctx.Err() != nil {
		return kv.NewError("upload context cancelled").With("stack", stack.Trace().TrimRuntime())
	}

	var errGo error
	tries := numRetries
	for tries > 0 {
		_, errGo = s.client.PutObject(ctx, s.bucket, dest, src, srcSize, minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})

		if errGo == nil {
			return nil
		}

		if isAccessDenied(errGo) {
			// Possible AWS credentials rotation, reset client and retry:
			src.Close()
			src, srcSize, srcName, err = sp.getSource()
			if err != nil {
				return err
			}

			s.waitAndRefreshClient()
			tries -= 1
		} else {
			return kv.Wrap(errGo)
		}
	}
	return kv.Wrap(errGo)
}

// Hash returns platform specific MD5 of the contents of the file that can be used by caching and other functions
// to track storage changes etc
//
// The hash on AWS S3 is not a plain MD5 but uses multiple hashes from file
// segments to increase the speed of hashing and also to reflect the multipart download
// processing that was used for the file, for a full explanation please see
// https://stackoverflow.com/questions/12186993/what-is-the-algorithm-to-compute-the-amazon-s3-etag-for-a-file-larger-than-5gb
func (s *s3Storage) Hash(ctx context.Context, name string) (hash string, err kv.Error) {
	key := name
	if len(key) == 0 {
		key = s.key
	}

	defer func() {
		if err != nil {
			err = err.With("bucket", s.bucket).With("name", name)
		}
	}()

	var errGo error
	tries := numRetries
	for tries > 0 {
		info, errGo := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
		if errGo == nil {
			return info.ETag, nil
		}
		if !isAccessDenied(errGo) {
			return "", kv.Wrap(errGo)
		}
		s.waitAndRefreshClient()
		tries -= 1
	}
	return "", kv.Wrap(errGo)
}

func (s *s3Storage) ListObjects(ctx context.Context, keyPrefix string) (names []string, warnings []kv.Error, err kv.Error) {
	// Create a done context to control 'ListObjects' go routine.
	doneCtx, cancel := context.WithCancel(ctx)

	// Indicate to our routine to exit cleanly upon return.
	defer cancel()

	names, err = s.retryListObjects(doneCtx, keyPrefix)
	return names, nil, err
}

func (s *s3Storage) retryListObjectsOnce(ctx context.Context, keyPrefix string) (names []string, err kv.Error, retry bool) {
	names = []string{}

	opts := minio.ListObjectsOptions{
		Prefix:    keyPrefix,
		Recursive: true,
		UseV1:     true,
	}
	objectCh := s.client.ListObjects(ctx, s.bucket, opts)
	for object := range objectCh {
		if object.Err != nil {
			if isAccessDenied(object.Err) {
				return names, kv.Wrap(object.Err), true
			}
			return names, kv.Wrap(object.Err).With("stack", stack.Trace().TrimRuntime()),
				false
		}
		names = append(names, object.Key)
	}
	return names, nil, false
}

func (s *s3Storage) retryListObjects(ctx context.Context, keyPrefix string) (names []string, err kv.Error) {

	defer func() {
		if err != nil {
			err = err.With("bucket", s.bucket).With("prefix", keyPrefix)
		}
	}()

	tries := numRetries
	for tries > 0 {
		names, err, retry := s.retryListObjectsOnce(ctx, keyPrefix)
		if !retry {
			return names, err
		}
		// Possible AWS credentials rotation, reset client and retry:
		s.waitAndRefreshClient()
		tries -= 1
	}
	return names, err
}

// Gather is used to retrieve files prefixed with a specific key.
func (s *s3Storage) Gather(ctx context.Context, keyPrefix string, outputDir string, maxBytes int64, tap io.Writer, failFast bool) (size int64, warnings []kv.Error, err kv.Error) {
	// Retrieve a list of the known keys that match the key prefix

	names := []string{}
	_ = names // Bypass the ineffectual assignment check

	names, warnings, err = s.ListObjects(ctx, keyPrefix)
	if err != nil {
		return size, warnings, err
	}

	// Place names into the gathered pool in sroted order to allow testing to
	// predictably download items when using the maxBytes parameter
	sort.Strings(names)

	// Download the keys within the prefix, making sure not to blow the budget
	for _, key := range names {
		s, w, e := s.Fetch(ctx, key, false, outputDir, maxBytes, tap)
		if len(w) != 0 {
			warnings = append(warnings, w...)
		}
		if e != nil {
			if failFast {
				return size, warnings, e
			}
			err = e
		}
		size += s
		maxBytes -= s
	}
	return size, warnings, err
}

func (s *s3Storage) getObject(ctx context.Context, key string, maxBytes int64, errCtx kv.List) (obj *minio.Object, err kv.Error) {
	obj, size, err := s.retryGetObject(ctx, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err.With("stack", stack.Trace().TrimRuntime())
	}

	// Check before downloading the file if it would on its own without decompression
	// blow the disk space budget assigned to it.  Doing this saves downloading the file
	// if there is an honest issue.
	if size > maxBytes {
		return nil, errCtx.NewError("blob size exceeded").With("size", humanize.Bytes(uint64(size)), "budget", humanize.Bytes(uint64(maxBytes))).With("stack", stack.Trace().TrimRuntime())
	}
	return obj, nil
}

func (s *s3Storage) fetchSideCopy(ctx context.Context, key string, maxBytes int64, tap io.Writer) (size int64, warns []kv.Error, err kv.Error) {
	errCtx := kv.With("name", key).With("bucket", s.bucket).With("key", key).With("endpoint", s.endpoint)

	obj, err := s.getObject(ctx, key, maxBytes, errCtx)
	if err != nil {
		return 0, warns, err
	}
	defer obj.Close()

	size, errGo := io.CopyN(tap, obj, maxBytes)
	if errGo != nil {
		if !errors.Is(errGo, io.EOF) {
			return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		errGo = nil
	}

	return size, warns, nil
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
func (s *s3Storage) Fetch(ctx context.Context, name string, unpack bool, output string, maxBytes int64, tap io.Writer) (size int64, warns []kv.Error, err kv.Error) {
	key := name
	if len(key) == 0 {
		key = s.key
	}

	if output == "" {
		// Special case when we just need to download file as it is.
		return s.fetchSideCopy(ctx, key, maxBytes, tap)
	}

	errCtx := kv.With("output", output).With("name", name).
		With("bucket", s.bucket).With("key", key).With("endpoint", s.endpoint)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return 0, warns, errCtx.NewError("a directory was not used, or did not exist").With("stack", stack.Trace().TrimRuntime())
	}

	obj, err := s.getObject(ctx, key, maxBytes, errCtx)
	if err != nil {
		return 0, warns, err
	}
	defer obj.Close()

	fileType, w := mime.MimeFromExt(name)
	if w != nil {
		warns = append(warns, w)
	}

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {
		var inReader io.ReadCloser

		switch fileType {
		case "application/x-gzip", "application/zip":
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader, errGo = gzip.NewReader(io.TeeReader(obj, tap))
			} else {
				inReader, errGo = gzip.NewReader(obj)
			}
		case "application/bzip2", "application/octet-stream":
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader = ioutil.NopCloser(bzip2.NewReader(io.TeeReader(obj, tap)))
			} else {
				inReader = ioutil.NopCloser(bzip2.NewReader(obj))
			}
		default:
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader = ioutil.NopCloser(io.TeeReader(obj, tap))
			} else {
				inReader = ioutil.NopCloser(obj)
			}
		}
		if errGo != nil {
			return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer inReader.Close()

		// Last in the stack is a tar file handling reader
		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errors.Is(errGo, io.EOF) {
				break
			} else if errGo != nil {
				return 0, warns, errCtx.Wrap(errGo).With("fileType", fileType).With("stack", stack.Trace().TrimRuntime())
			}

			outFN, errGo := filepath.Abs(filepath.Join(output, header.Name))
			if errGo != nil {
				return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			if len(header.Linkname) != 0 {
				escapes, err := defense.WillEscape(header.Linkname, outFN)
				if !escapes {
					escapes, err = defense.WillEscape(header.Name, outFN)
				}
				if escapes {
					if err != nil {
						return 0, warns, errCtx.Wrap(err).With("link", header.Linkname, "filename", header.Name, "output", output)
					} else {
						return 0, warns, errCtx.NewError("archive escaped").With("filename", header.Name, "output", output)
					}
				}

				if errGo = os.Symlink(header.Linkname, outFN); errGo != nil {
					return 0, warns, errCtx.Wrap(errGo, "symbolic link create failed").With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if info.IsDir() {
					if errGo = os.MkdirAll(outFN, os.FileMode(header.Mode)); errGo != nil {
						return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", outFN)
					}
				}
			case tar.TypeReg, tar.TypeRegA:

				_ = os.MkdirAll(filepath.Dir(outFN), os.ModePerm)

				if escapes, err := defense.WillEscape(header.Name, output); escapes {
					if err != nil {
						return 0, warns, errCtx.Wrap(err).With("filename", header.Name, "output", output, "dir", filepath.Dir(outFN))
					} else {
						return 0, warns, errCtx.NewError("archive escaped").With("filename", header.Name, "output", output)
					}
				}

				file, errGo := os.OpenFile(outFN, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
				if errGo != nil {
					return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", outFN)
				}

				size, errGo = io.CopyN(file, tarReader, maxBytes)
				file.Close()
				if errGo != nil {
					if !errors.Is(errGo, io.EOF) {
						return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", outFN)
					}
					errGo = nil
				}
			default:
				errGo = fmt.Errorf("unknown tar archive type '%c'", header.Typeflag)
				return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", outFN)
			}
		}
	} else {
		errGo := os.MkdirAll(output, 0700)
		if errGo != nil {
			return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("output", output)
		}
		path := filepath.Join(output, filepath.Base(key))
		f, errGo := os.Create(path)
		if errGo != nil {
			return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		if tap != nil {
			// Create a stack of readers that first tee off any data read to a tap
			// the tap being able to send data to things like caches etc
			//
			// Second in the stack of readers after the TAP is a decompression reader
			size, errGo = io.CopyN(outf, io.TeeReader(obj, tap), maxBytes)
			if errGo != nil {
				if !errors.Is(errGo, io.EOF) {
					return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}
				errGo = nil
			}
		} else {
			size, errGo = io.CopyN(outf, obj, maxBytes)
			if errGo != nil {
				if !errors.Is(errGo, io.EOF) {
					return 0, warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}
				errGo = nil
			}
		}
		outf.Flush()
	}
	return size, warns, nil
}

type FileSrcProvider struct {
	Name string
}

func (fp *FileSrcProvider) getSource() (io.ReadCloser, int64, string, kv.Error) {
	file, errGo := os.Open(filepath.Clean(fp.Name))
	if errGo != nil {
		return nil, 0, fp.Name, kv.Wrap(errGo).With("file", fp.Name)
	}
	fileStat, errGo := file.Stat()
	if errGo != nil {
		return file, 0, fp.Name, kv.Wrap(errGo).With("file", fp.Name)
	}
	return file, fileStat.Size(), fp.Name, nil
}

// uploadFile can be used to transmit a file to the S3 server using a fully qualified file
// name and key
func (s *s3Storage) uploadFile(ctx context.Context, src string, dest string) (err kv.Error) {
	if ctx.Err() != nil {
		return kv.NewError("upload context cancelled").With("stack", stack.Trace().TrimRuntime()).With("src", src, "bucket", s.bucket, "key", dest)
	}

	fileSrc := &FileSrcProvider{
		Name: filepath.Clean(src),
	}

	uploadCtx, cancel := context.WithDeadline(ctx, time.Now().Add(10*time.Minute))
	defer cancel()

	err = s.retryPutObject(uploadCtx, fileSrc, dest)
	return err
}

// Return directories as compressed artifacts to the AWS storage for an
// experiment
func (s *s3Storage) Deposit(ctx context.Context, src string, dest string) (warns []kv.Error, err kv.Error) {

	if !archive.IsTar(dest) {
		return warns, kv.NewError("uploads must be tar, or tar compressed files").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	}

	key := dest
	if len(key) == 0 {
		key = s.key
	}

	files, err := archive.NewTarWriter(src)
	if err != nil {
		return warns, err
	}

	if !files.HasFiles() {
		warns = append(warns, kv.NewError("no files found").With("src", src).With("stack", stack.Trace().TrimRuntime()))
		return warns, nil
	}

	// First, write to temporary .tar file
	tf, errGo := os.CreateTemp("", "deposit")
	if errGo != nil {
		return warns, kv.Wrap(errGo).With("src", src, "dest", dest)
	}
	tfName := tf.Name()

	err = tarFileWriter(tf, files, dest)
	if err != nil {
		return warns, err
	}

	defer func() {
		os.Remove(tfName)
	}()

	// "tf" is closed by now
	uploadCtx := context.Background()
	err = s.uploadFile(uploadCtx, tfName, dest)

	return warns, err
}

func tarFileWriter(pw *os.File, files *archive.TarWriter, dest string) (err kv.Error) {
	err = nil

	defer func() {
		if err != nil {
			err = err.With("key", dest)
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = kv.NewError(fmt.Sprint(r)).With("stack", stack.Trace().TrimRuntime())
			}
		}
		pw.Close()
	}()

	typ, _ := mime.MimeFromExt(dest)

	switch typ {
	case "application/tar", "application/octet-stream":
		tw := tar.NewWriter(pw)
		if errGo := files.Write(tw); errGo != nil {
			err = kv.Wrap(errGo)
		}
		tw.Close()
	case "application/bzip2":
		outZ, _ := bzip2w.NewWriter(pw, &bzip2w.WriterConfig{Level: 6})
		tw := tar.NewWriter(outZ)
		if errGo := files.Write(tw); errGo != nil {
			err = kv.Wrap(errGo)
		}
		tw.Close()
		outZ.Close()
	case "application/x-gzip":
		outZ := gzip.NewWriter(pw)
		tw := tar.NewWriter(outZ)
		if errGo := files.Write(tw); errGo != nil {
			err = kv.Wrap(errGo)
		}
		tw.Close()
		outZ.Close()
	case "application/zip":
		return kv.NewError("only tar archives are supported").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	default:
		return kv.NewError("unrecognized upload compression").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	}
	return err
}
