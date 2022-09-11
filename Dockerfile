FROM golang as builder

WORKDIR /go/src/github.com/carlory/firefly

COPY . /go/src/github.com/carlory/firefly 
RUN cd /go/src/github.com/carlory/firefly && \
    go build -o /bin/firefly-controller-manager cmd/firefly-controller-manager/controller-manager.go


# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base:nonroot

WORKDIR /go/src/github.com/carlory/firefly
COPY --from=builder /go/src/github.com/carlory/firefly /go/src/github.com/carlory/firefly
COPY --from=builder /bin/firefly-controller-manager  /bin/firefly-controller-manager
USER 65532:65532

# RUN cd /go/src/github.com/carlory/firefly && \
#     go build -o firefly-controller-manager cmd/firefly-controller-manager/controller-manager.go