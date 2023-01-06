FROM golang:1.19 AS build

ARG VERSION

# Setup the environment
ENV CGO_ENABLED=0
ENV GOPATH=/go
ENV GO111MODULE=on
WORKDIR /workspace

ARG GOOS=linux
ARG GOARCH=amd64

ENV GOOS=$GOOS
ENV GOARCH=$GOARCH

# Install go dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Build and test code
COPY . .
RUN make build-docker

FROM alpine:3.14
EXPOSE 8080
EXPOSE 8090
RUN apk --no-cache add curl # install for debugging -- not required to run
COPY --from=build /workspace/bin/metrics-node-sampler /bin/metrics-node-sampler
COPY --from=build /workspace/bin/metrics-prometheus-collector /bin/metrics-prometheus-collector
COPY --from=build /workspace/bin/container-monitor /bin/container-monitor
