module github.com/leaf-ai/studio-go-runner

go 1.15

require (
	cloud.google.com/go/storage v1.6.0
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/Masterminds/vcs v1.13.1 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/armon/go-radix v0.0.0-20180808171621-7fddfc383310 // indirect
	github.com/awnumar/memguard v0.22.1
	github.com/aws/aws-sdk-go v1.35.20
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/dgryski/go-farm v0.0.0-20180109070241-2de33835d102
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dsnet/compress v0.0.1
	github.com/dustin/go-humanize v1.0.0
	github.com/ekalinin/github-markdown-toc.go v0.0.0-20201214100212-a3e410f71786 // indirect
	github.com/eknkc/basex v1.0.0 // indirect
	github.com/evanphx/json-patch v4.1.0+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-enry/go-license-detector/v4 v4.0.0
	github.com/go-stack/stack v1.8.0
	github.com/go-test/deep v1.0.5
	github.com/go-yaml/yaml v2.1.0+incompatible // indirect
	github.com/golang/dep v0.5.4 // indirect
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.4 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/jjeffery/kv v0.8.1
	github.com/jmank88/nuts v0.4.0 // indirect
	github.com/karlmutch/base62 v0.0.0-20150408093626-b80cdc656a7a
	github.com/karlmutch/ccache v2.0.3-0.20180726214243-573f5233780c+incompatible
	github.com/karlmutch/circbuf v0.0.0-20150827004946-bbbad097214e
	github.com/karlmutch/duat v0.0.0-20200918224055-5162d53e3510
	github.com/karlmutch/envflag v0.0.0-20160830095501-ae3268980a29
	github.com/karlmutch/go-cache v2.0.0+incompatible
	github.com/karlmutch/go-nvml v0.0.0-20200203202551-277366df5c37
	github.com/karlmutch/go-shortid v0.0.0-20160104014424-6c56cef5189c
	github.com/karlmutch/hashstructure v0.0.0-20170609045927-2bca23e0e452
	github.com/karlmutch/k8s v1.2.1-0.20200715200931-d87bc94d5dd7
	github.com/karlmutch/logxi v0.0.0-20210126195415-2f02ee1dbe8d
	github.com/karlmutch/petname v0.0.0-20190202005206-caff460d43c2 // indirect
	github.com/karlmutch/semver v1.4.0 // indirect
	github.com/karlmutch/vtclean v0.0.0-20170504063817-d14193dfc626
	github.com/karlseguin/expect v1.0.7 // indirect
	github.com/leaf-ai/go-service v0.0.0-20210127003452-9ede098a0bd9
	github.com/lthibault/jitterbug v2.0.0+incompatible
	github.com/makasim/amqpextra v0.14.3
	github.com/mgutz/logxi v0.0.0-20170321173016-3753102df44e
	github.com/mholt/archiver v2.1.0+incompatible
	github.com/michaelklishin/rabbit-hole v1.4.0
	github.com/minio/minio-go/v7 v7.0.7
	github.com/mitchellh/copystructure v1.0.0
	github.com/nbutton23/zxcvbn-go v0.0.0-20180912185939-ae427f1e4c1d // indirect
	github.com/nightlyone/lockfile v1.0.0 // indirect
	github.com/nwaples/rardecode v0.0.0-20171029023500-e06696f847ae // indirect
	github.com/onsi/ginkgo v1.14.1 // indirect
	github.com/onsi/gomega v1.10.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/otiai10/copy v1.2.0
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.15.0
	github.com/rs/xid v1.2.1
	github.com/sdboyer/constext v0.0.0-20170321163424-836a14457353 // indirect
	github.com/shirou/gopsutil v3.20.12+incompatible
	github.com/streadway/amqp v1.0.0
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf // indirect
	github.com/valyala/fastjson v1.2.0
	github.com/ventu-io/go-shortid v0.0.0-20171029131806-771a37caa5cf // indirect
	go.opentelemetry.io/otel v0.16.0
	go.opentelemetry.io/otel/sdk v0.16.0 // indirect
	go.uber.org/atomic v1.7.0
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9 // indirect
	golang.org/x/sys v0.0.0-20210124154548-22da62e12c0c // indirect
	google.golang.org/api v0.29.0
	google.golang.org/grpc v1.31.0 // indirect
	google.golang.org/protobuf v1.25.0
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	honnef.co/go/tools v0.0.1-2020.1.6 // indirect
)
