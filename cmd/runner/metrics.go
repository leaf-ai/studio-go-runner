// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jjeffery/kv"
)

var (
	refreshSuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_refresh_success",
			Help: "Number of successful queue inventory checks.",
		},
		[]string{"host", "project"},
	)
	refreshFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_refresh_fail",
			Help: "Number of failed queue inventory checks.",
		},
		[]string{"host", "project"},
	)

	queueChecked = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_checked",
			Help: "Number of times a queue is queried for work.",
		},
		[]string{"host", "queue_type", "queue_name"},
	)
	queueIgnored = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_ignored",
			Help: "Number of times a queue is intentionally not queried, or skipped work.",
		},
		[]string{"host", "queue_type", "queue_name"},
	)
	queueRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_project_running",
			Help: "Number of experiments being actively worked on per queue.",
		},
		[]string{"host", "queue_type", "queue_name", "project", "experiment"},
	)
	queueRan = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_project_completed",
			Help: "Number of experiments that have been run per queue.",
		},
		[]string{"host", "queue_type", "queue_name", "project", "experiment"},
	)
)

func init() {
	prometheus.MustRegister(refreshSuccesses)
	prometheus.MustRegister(refreshFailures)
	prometheus.MustRegister(queueChecked)
	prometheus.MustRegister(queueIgnored)
	prometheus.MustRegister(queueRunning)
	prometheus.MustRegister(queueRan)
}

func GetCounterValue(metric *prometheus.CounterVec, labels prometheus.Labels) (val float64, err kv.Error) {
	m := &dto.Metric{}
	if errGo := metric.With(labels).Write(m); errGo != nil {
		return 0, kv.Wrap(errGo)
	}
	return m.Counter.GetValue(), nil
}

func GetGaugeValue(metric *prometheus.GaugeVec, labels prometheus.Labels) (val float64, err kv.Error) {
	m := &dto.Metric{}
	if errGo := metric.With(labels).Write(m); errGo != nil {
		return 0, kv.Wrap(errGo)
	}
	return m.Counter.GetValue(), nil
}
