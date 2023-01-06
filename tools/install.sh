#/bin/bash

set -e

cd tools

go install \
    github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
    github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 \
    google.golang.org/protobuf/cmd/protoc-gen-go \
    google.golang.org/grpc/cmd/protoc-gen-go-grpc \
    sigs.k8s.io/controller-tools/cmd/controller-gen \
    github.com/golangci/golangci-lint/cmd/golangci-lint \
    sigs.k8s.io/kustomize/kustomize/v3
