# using static nonroot image
# user:group is nobody:nobody, uid:gid = 65534:65534
#
# Information concerning this base imge can be found at, https://github.com/GoogleContainerTools/distroless/blob/master/base/README.md
# https://console.cloud.google.com/gcr/images/distroless/GLOBAL/static@sha256:a7752b29b18bb106938caefd8dcce8a94199022cbd06ea42268b968f35e837a8/details?tab=info
# The original tag was docker pull gcr.io/distroless/static:latest-amd64
#
FROM gcr.io/distroless/static@sha256:a7752b29b18bb106938caefd8dcce8a94199022cbd06ea42268b968f35e837a8
# Copy the cpu mode static executable
COPY ./bin/serving-bridge-linux-amd64-cpu /go/bin/serving-bridge-linux-amd64-cpu

ENTRYPOINT ["/go/bin/serving-bridge-linux-amd64-cpu"]
