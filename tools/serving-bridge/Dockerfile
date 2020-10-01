# using static nonroot image
# user:group is nobody:nobody, uid:gid = 65534:65534
#
# Information concerning this base imge can be found at, https://github.com/GoogleContainerTools/distroless/blob/master/base/README.md
#
FROM gcr.io/distroless/static@sha256:08cce4d65b19780e10b555578a0f9c9af51310308ec31e648577122d40e6922a

# Copy the cpu mode static executable
COPY ./bin/serving-bridge-linux-amd64-cpu /go/bin/serving-bridge-linux-amd64-cpu

ENTRYPOINT ["/go/bin/serving-bridge-linux-amd64-cpu"]
