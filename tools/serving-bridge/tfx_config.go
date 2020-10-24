// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of TFX Model serving configuration
// handling functions

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/protobuf/encoding/prototext"

	"github.com/karlmutch/k8s"
	core "github.com/karlmutch/k8s/apis/core/v1"
	meta "github.com/karlmutch/k8s/apis/meta/v1"

	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/leaf-ai/studio-go-runner/pkg/server"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// ReadTFXCfg is used to read the TFX serving configuration file and parse it into a format
// that can be used internally for dealing with model descriptions
//
func ReadTFXCfg(ctx context.Context, cfg Config, logger *log.Logger) (tfxCfg *serving_config.ModelServerConfig, err kv.Error) {

	_, span := global.Tracer(tracerName).Start(ctx, "read-tfx-cfg")
	defer span.End()

	data := []byte{}
	fn := ""

	if len(cfg.tfxConfigFn) != 0 {
		if !strings.HasPrefix(cfg.tfxConfigFn, "s3://") {
			fn = cfg.tfxConfigFn
			fp, errGo := os.Open(fn)
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
			}
			defer fp.Close()

			data, errGo = ioutil.ReadAll(fp)
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			data, err = ReadS3Cfg(ctx, cfg)
		}
	} else {
		if len(cfg.tfxConfigCM) == 0 {
			return nil, kv.NewError("one of TFX configuration settings must be specified").With("stack", stack.Trace().TrimRuntime())
		}
		data, err = ReadTFXCfgConfigMap(ctx, cfg)
	}

	tfxCfg = &serving_config.ModelServerConfig{}

	// Unmarshal the text into the struct
	if errGo := prototext.Unmarshal(data, tfxCfg); errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return tfxCfg, nil
}

// ReadS3Cfg is used to retrieve a TFX server config from an S3 location
func ReadS3Cfg(ctx context.Context, cfg Config) (data []byte, err kv.Error) {
	_, span := global.Tracer(tracerName).Start(ctx, "read-tfx-cfg")
	defer span.End()

	client, errGo := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure: false,
	})
	if errGo != nil {
		err := kv.Wrap(errGo).With("endpoint", cfg.endpoint).With("stack", stack.Trace().TrimRuntime())
		span.SetStatus(codes.Unavailable, err.Error())
		return nil, err
	}

	if logger.IsTrace() {
		client.TraceOn(nil)
	}

	key := cfg.tfxConfigCM
	obj, errGo := client.GetObject(ctx, cfg.bucket, key, minio.GetObjectOptions{})
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("bucket", cfg.bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
	}

	buffer := &bytes.Buffer{}
	if _, errGo = io.Copy(buffer, obj); errGo != nil {
		return nil, kv.Wrap(errGo).With("bucket", cfg.bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
	}
	return buffer.Bytes(), nil
}

// ReadTFXCfgConfigMap is used to retrieve the TFX serving configuration from a
// Kubernetes configMap
//
func ReadTFXCfgConfigMap(ctx context.Context, cfg Config) (data []byte, err kv.Error) {
	// Use the Kubernetes configuration map storage
	if err := server.IsAliveK8s(); err != nil {
		return nil, err
	}

	k8sClient, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	configMap := &core.ConfigMap{}
	if errGo = k8sClient.Get(ctx, k8sClient.Namespace, cfg.tfxConfigCM, configMap); errGo != nil {
		return nil, kv.Wrap(errGo).With("namespace", k8sClient.Namespace, "ConfigMap", cfg.tfxConfigCM).With("stack", stack.Trace().TrimRuntime())
	}

	cfgData, isPresent := configMap.Data[cfg.tfxConfigCM]
	if !isPresent {
		return nil, kv.NewError("configuration absent").With("namespace", k8sClient.Namespace, "ConfigMap", cfg.tfxConfigCM).With("stack", stack.Trace().TrimRuntime())
	}
	return []byte(cfgData), nil
}

// WriteTFXCfg is used to output the models configured for serving by TFX to an
// ASCII format protobuf file.
//
func WriteTFXCfg(ctx context.Context, cfg Config, tfxCfg *serving_config.ModelServerConfig, logger *log.Logger) (err kv.Error) {

	_, span := global.Tracer(tracerName).Start(ctx, "write-tfx-cfg")
	defer span.End()

	opts := prototext.MarshalOptions{
		Multiline: true,
	}

	// Marshall the protobuf data structure into prototext format output
	data, errGo := opts.Marshal(tfxCfg)
	if errGo != nil {
		return kv.Wrap(errGo).With("filename", cfg.tfxConfigFn).With("stack", stack.Trace().TrimRuntime())
	}

	if len(cfg.tfxConfigFn) != 0 {
		if !strings.HasPrefix(cfg.tfxConfigFn, "s3://") {
			if errGo = ioutil.WriteFile(cfg.tfxConfigFn, data, 0600); errGo != nil {
				return kv.Wrap(errGo).With("filename", cfg.tfxConfigFn).With("stack", stack.Trace().TrimRuntime())
			}
			return nil
		} else {
			return WriteTFXCfgS3(ctx, cfg, data)
		}
	}

	if len(cfg.tfxConfigCM) == 0 {
		return kv.NewError("one of TFX configuration settings must be specified").With("stack", stack.Trace().TrimRuntime())
	}
	return WriteTFXCfgCXonfigMap(ctx, cfg, data)
}

// WriteTFXCfgCXonfigMap is used to serialize the TFX Serving configuration
// and persist it into a Kubernetes ConfigMap
func WriteTFXCfgCXonfigMap(ctx context.Context, cfg Config, data []byte) (err kv.Error) {
	// Use the Kubernetes configuration map storage

	if err := server.IsAliveK8s(); err != nil {
		return err
	}

	k8sClient, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	configMap := &core.ConfigMap{
		Metadata: &meta.ObjectMeta{
			Name:      k8s.String(cfg.tfxConfigCM),
			Namespace: k8s.String(k8sClient.Namespace),
		},
		Data: map[string]string{cfg.tfxConfigCM: string(data)},
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	// Upsert a k8s config map that we can use for testing purposes
	if errGo = k8sClient.Update(ctx, configMap); errGo != nil {
		// If an HTTP error was returned by the API server, it will be of type
		// *k8s.APIError. This can be used to inspect the status code.
		if apiErr, ok := errGo.(*k8s.APIError); ok {
			// Resource already exists. Carry on.
			if apiErr.Code == http.StatusNotFound {
				errGo = k8sClient.Create(ctx, configMap)
			}
		}
	}
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// WriteTFXCfgS3 is used to persist the TFX serving configuration to
// an S3 blob
func WriteTFXCfgS3(ctx context.Context, cfg Config, data []byte) (err kv.Error) {
	_, span := global.Tracer(tracerName).Start(ctx, "write-tfx-cfg-s3")
	defer span.End()

	client, errGo := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure: false,
	})
	if errGo != nil {
		err = kv.Wrap(errGo).With("endpoint", cfg.endpoint).With("stack", stack.Trace().TrimRuntime())
		span.SetStatus(codes.Unavailable, err.Error())
		return err
	}

	if logger.IsTrace() {
		client.TraceOn(nil)
	}

	key := cfg.tfxConfigCM
	_, errGo = client.PutObject(context.Background(), cfg.bucket, key, bytes.NewReader([]byte(data)), int64(len(data)),
		minio.PutObjectOptions{})
	if errGo != nil {
		return kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}
