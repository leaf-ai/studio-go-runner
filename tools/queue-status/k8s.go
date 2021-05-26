// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/davecgh/go-spew/spew"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func defaultKubeConfig() (config string) {
	if home := homedir.HomeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}
	return config
}

func loadKnownJobs(ctx context.Context, cfg *Config, cluster string, namespace string, tag string, inCluster bool, queues *Queues) (err kv.Error) {

	// Allow both in cluster as well as external access to kubernetes, try the incluster version first
	// and if that does not work move to external cluster access
	config, errGo := rest.InClusterConfig()
	if errGo != nil {
		// use the current context in kubeconfig
		config, errGo = clientcmd.BuildConfigFromFlags("", cfg.kubeconfig)
		if err != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	clientset, errGo := kubernetes.NewForConfig(config)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	opts := metav1.ListOptions{
		//FieldSelector: "metadata.name=kubernetes",
		//LabelSelector: "app=<APPNAME>",
	}
	for {
		pods, errGo := clientset.CoreV1().Pods(namespace).List(ctx, opts)

		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		for _, aPod := range pods.Items {
			fmt.Print(spew.Sdump(aPod.Name))
		}
		if len(pods.Continue) == 0 {
			break
		}
		opts.Continue = pods.Continue
	}

	return nil
}
