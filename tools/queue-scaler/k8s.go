// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

const (
	appLabel     = "app=queue-scaler.cognizant-ai.dev"
	queueLabel   = "queue.cognizant-ai.dev"
	jobNameLabel = "job-name"
)

func defaultKubeConfig() (config string) {
	if home := homedir.HomeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}
	return config
}

func loadKnownJobs(ctx context.Context, cfg *Config, cluster string, namespace string, inCluster bool, queues *Queues) (err kv.Error) {

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

	if config == nil {
		return kv.NewError("The kubernetes configuration could not be found").With("stack", stack.Trace().TrimRuntime())
	}

	clientset, errGo := kubernetes.NewForConfig(config)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	opts := metav1.ListOptions{
		LabelSelector: appLabel,
	}
	for {
		pods, errGo := clientset.CoreV1().Pods(namespace).List(ctx, opts)

		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		for _, aPod := range pods.Items {
			if aPod.Status.Phase != corev1.PodPending && aPod.Status.Phase != corev1.PodRunning {
				continue
			}
			if qName, isPresent := aPod.Labels[queueLabel]; isPresent && len(qName) != 0 {
				queue, isPresent := (*queues)[qName]
				if !isPresent {
					logger.Warn("queue processing inside cluster not found in queue catalog", "queue", qName, "stack", stack.Trace().TrimRuntime())
					continue
				}
				jobName, isPresent := aPod.Labels[jobNameLabel]
				if !isPresent {
					logger.Warn("queue processing inside cluster missing job-name label", "queue", qName, "stack", stack.Trace().TrimRuntime())
					continue
				}
				pods, isPresent := queue.Jobs[jobName]
				if !isPresent {
					pods = map[string]struct{}{}
				}
				pods[aPod.Name] = struct{}{}
				queue.Jobs[jobName] = pods
			}
		}
		if len(pods.Continue) == 0 {
			break
		}
		opts.Continue = pods.Continue
	}

	for qName, qDetails := range *queues {
		// Count the jobs that are running
		qDetails.Running = 0
		for _, pods := range qDetails.Jobs {
			qDetails.Running += len(pods)
		}
		(*queues)[qName] = qDetails
	}

	return nil
}
