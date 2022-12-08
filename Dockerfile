
FROM golang:1.19 AS builder

ARG VERSION

# Setup the environment
ENV CGO_ENABLED=0
ENV GOPATH=/go
ENV GO111MODULE=on
ENV GOBIN=/usr/local/bin/
WORKDIR /workspace

# Install non-go tools
RUN apt update
RUN apt install -y protobuf-compiler

# Install go tools
COPY tools/ ./tools
RUN ./tools/install.sh

# Install go dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Build and test code
COPY . .
ENV GOOS=linux
ENV GOARCH=amd64
RUN make build

FROM alpine:3.14
EXPOSE 8080
EXPOSE 8090
RUN apk --no-cache add curl # install for debugging -- not required to run
COPY --from=builder /workspace/bin/metrics-node-sampler /bin/metrics-node-sampler
COPY --from=builder /workspace/bin/metrics-prometheus-collector /bin/metrics-prometheus-collector
COPY --from=builder /workspace/bin/container-monitor /bin/container-monitor
