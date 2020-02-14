Prometheus Metrics

The go runner supports a range of prometheus metrics.  This document contains a list of the runner specific metrics.  Go also provides a set of metrics related to the Go runtime, these are detailed at https://github.com/prometheus/client_golang/blob/master/prometheus/go_collector.go.

If you wish to add new metrics to the runner specific items the best practices for naming prometheus metrics can be found at, https://prometheus.io/docs/practices/naming/.

runner_queue_refresh_success    Number of successful queue inventory checks (host, project)
runner_queue_refresh_fail       Number of failed queue inventory checks (host, project)
runner_queue_checked            Number of times a queue is queried for work (host, queue_type, queue_name)
runner_queue_ignored            Number of times a queue is intentionally not queried, or skipped work (host, queue_type, queue_name)
runner_project_running            Number of experiments being actively worked on per queue (host, project, experiment, queue_type, queue_name)
runner_project_completed          Number of experiments that have been run per queue (host, project, experiment, queue_type, queue_name)

runner_cache_hits               Number of cache hits (host,hash)
runner_cache_misses             Number of cache misses (host,hash)



Copyright &copy 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
