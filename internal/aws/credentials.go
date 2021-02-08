// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package aws_int

// This file contains the implementation of functions related to AWS.
//
// Especially functions related to the credentials file handling

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// AWSCred is used to encapsulate the credentials that are to be used to access an AWS resource
// suhc as an S3 bucket for example.
//
type AWSCred struct {
	Project string
	Region  string
	Creds   *credentials.Credentials
}

// AWSExtractCreds can be used to populate a set of credentials from a pair of config and
// credentials files typicall found in the ~/.aws directory by AWS clients
//
func AWSExtractCreds(filenames []string) (cred *AWSCred, err kv.Error) {

	cred = &AWSCred{
		Project: "aws_" + filepath.Base(filepath.Dir(filenames[0])),
	}

	credsDone := false

	// AWS Does not read the region automatically from the config so lets read it here
	for _, aFile := range filenames {
		wasConfig := func() bool {
			f, err := os.Open(filepath.Clean(aFile))
			if err != nil {
				return false
			}
			if len(cred.Region) == 0 {
				scan := bufio.NewScanner(f)
				for scan.Scan() {
					line := scan.Text()
					line = strings.Replace(line, " ", "", -1)
					if strings.HasPrefix(strings.ToLower(line), "region=") {
						tokens := strings.SplitN(line, "=", 2)
						cred.Region = tokens[1]
						return true
					}
				}
			}

			f.Close()
			return false
		}()

		if !credsDone && !wasConfig {
			cred.Creds = credentials.NewSharedCredentials(aFile, "default")
			credsDone = true
		}
	}

	if len(cred.Region) == 0 {
		return nil, kv.NewError("none of the supplied files defined a region").With("stack", stack.Trace().TrimRuntime()).With("files", filenames)
	}

	if !credsDone {
		return nil, kv.NewError("credentials never loaded").With("stack", stack.Trace().TrimRuntime()).With("files", filenames)
	}
	return cred, nil
}
