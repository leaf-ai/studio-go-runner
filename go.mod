module github.com/leaf-ai/studio-go-runner

go 1.16

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/Rhymond/go-money v1.0.2
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/awnumar/memguard v0.22.2
	github.com/aws/aws-sdk-go v1.38.63
	github.com/cenkalti/backoff/v4 v4.1.1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.7.1
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/docker/docker v20.10.3+incompatible // indirect
	github.com/dsnet/compress v0.0.1
	github.com/dustin/go-humanize v1.0.0
	github.com/ekalinin/github-markdown-toc.go v0.0.0-20201214100212-a3e410f71786 // indirect
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-enry/go-license-detector/v4 v4.2.0
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/go-stack/stack v1.8.0
	github.com/go-test/deep v1.0.7
	github.com/go-yaml/yaml v2.1.0+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jjeffery/kv v0.8.1
	github.com/karlmutch/base62 v0.0.0-20150408093626-b80cdc656a7a
	github.com/karlmutch/ccache v2.0.3-0.20180726214243-573f5233780c+incompatible
	github.com/karlmutch/circbuf v0.0.0-20150827004946-bbbad097214e
	github.com/karlmutch/duat v0.0.0-20210225015104-bcfa908c74fb
	github.com/karlmutch/envflag v0.0.0-20210224011628-e09410f046e9
	github.com/karlmutch/go-cache v2.0.0+incompatible
	github.com/karlmutch/go-nvml v0.0.0-20200203202551-277366df5c37
	github.com/karlmutch/go-shortid v0.0.0-20170305092000-935de6796a71
	github.com/karlmutch/hashstructure v0.0.0-20170609045927-2bca23e0e452
	github.com/karlmutch/k8s v1.2.1-0.20210224003752-d750059a3836
	github.com/karlmutch/logxi v0.0.0-20210224194221-fde727bca873
	github.com/karlmutch/petname v0.0.0-20190202005206-caff460d43c2 // indirect
	github.com/karlmutch/vtclean v0.0.0-20170504063817-d14193dfc626
	github.com/karlseguin/expect v1.0.7 // indirect
	github.com/klauspost/cpuid/v2 v2.0.6 // indirect
	github.com/leaf-ai/go-service v0.0.0-20210524171939-49dd5f03976b
	github.com/lthibault/jitterbug v2.0.0+incompatible
	github.com/makasim/amqpextra v0.16.4
	github.com/mholt/archiver/v3 v3.5.0
	github.com/michaelklishin/rabbit-hole/v2 v2.10.0
	github.com/minio/minio v0.0.0-20210507214158-ab7d5ee3d98e // indirect
	github.com/minio/minio-go/v7 v7.0.11
	github.com/mitchellh/copystructure v1.2.0
	github.com/montanaflynn/stats v0.6.6 // indirect
	github.com/odg0318/aws-ec2-price v0.0.0-20200327030318-88202a04e06d
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/otiai10/copy v1.2.0
	github.com/prometheus/client_golang v1.10.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.23.0
	github.com/prometheus/prom2json v1.3.0
	github.com/rs/xid v1.3.0
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil v3.21.5+incompatible
	github.com/shirou/gopsutil/v3 v3.21.4 // indirect
	github.com/streadway/amqp v1.0.1-0.20200716223359-e6b33f460591
	github.com/tebeka/atexit v0.3.0
	github.com/tinylib/msgp v1.1.5 // indirect
	github.com/tklauser/go-sysconf v0.3.5 // indirect
	github.com/ulikunitz/xz v0.5.8 // indirect
	github.com/valyala/fastjson v1.6.3
	github.com/ventu-io/go-shortid v0.0.0-20201117134242-e59966efd125 // indirect
	go.opentelemetry.io/otel v0.16.0
	go.uber.org/atomic v1.8.0
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	golang.org/x/term v0.0.0-20210422114643-f5beecf764ed // indirect
	golang.org/x/tools v0.1.3 // indirect
	google.golang.org/genproto v0.0.0-20210224155714-063164c882e6 // indirect
	google.golang.org/grpc v1.36.0 // indirect
	google.golang.org/protobuf v1.26.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
)

replace (
	golang.org/x/text v0.3.0 => golang.org/x/text v0.3.3
	golang.org/x/text v0.3.1 => golang.org/x/text v0.3.3
	golang.org/x/text v0.3.2 => golang.org/x/text v0.3.3
)
