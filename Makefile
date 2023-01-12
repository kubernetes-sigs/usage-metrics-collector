SHELL := /bin/bash

export BINARIES         ?= $(shell go list ./cmd/... | grep -v integration | grep -v cmds | grep -v metrics-node-sampler/cmd | grep -v metrics-prometheus-collector/cmd)
export GO111MODULE      ?= on
export GOPATH           ?= $(shell go env $GOPATH)
export GOOS             ?= $(shell go env GOOS)
export GOARCH           ?= $(shell go env GOARCH)
export GIT_BRANCH       ?= $(shell git rev-parse --abbrev-ref HEAD)
export GIT_COMMIT       ?= $(shell git rev-parse --short HEAD)
export DATE             ?= $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')
export VERSION          ?= $(shell git tag)
export VERSION_FLAGS    = -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.date="$(DATE)"
export LD_FLAGS         ?= -ldflags="$(VERSION_FLAGS) -w -s"

## --------------------------------------
## Directories
## --------------------------------------

COMMON_SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

# the root directory of this repo
ifeq ($(origin ROOT_DIR),undefined)
ROOT_DIR := $(abspath $(COMMON_SELF_DIR))
endif

# a directory that contains project built binaries
ifeq ($(origin BIN_DIR), undefined)
BIN_DIR := $(ROOT_DIR)/bin
endif

ifeq ($(origin TOOLS_DIR), undefined)
TOOLS_DIR := $(ROOT_DIR)/.tools
endif

$(TOOLS_DIR):
	mkdir -p $(TOOLS_DIR) && cd $(TOOLS_DIR) && go mod init tempmod

# We are using Kubebuilder Test Assets for integration testing
K8S_VERSION=1.22.0
KUBEBUILDER := $(abspath $(TOOLS_DIR)/kubebuilder)
export KUBEBUILDER_ASSETS := $(KUBEBUILDER)/bin

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= crd

## --------------------------------------
## Make targets
## --------------------------------------
default: all

all: fmt vet test build

verify: fmt vet verify-licenses

help:			## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

clean:			## Remove all build and test related artifacts
	rm -rf ./.out bin

test: $(KUBEBUILDER_ASSETS)          ## Run local tests
	go test -count=1 -v ./...

vet:			## Run go vet on this project
	go vet ./...

fmt: ## Format the code
	go fmt ./...

build-dir:
	@mkdir bin 2>/dev/null || true

## Some binaries are Linux/amd64-only. To build, run
##     GOOS=linux GOARCH=amd64 make build
build:	proto build-dir	 ## Build this project go binaries
	@$(foreach pkg,$(BINARIES),\
		go build -o bin $(LD_FLAGS) $(pkg);)

## Some binaries are Linux/amd64-only. To build, run
##     GOOS=linux GOARCH=amd64 make build
build-docker:	build-dir	 ## Build this project go binaries
	@$(foreach pkg,$(BINARIES),\
		go build -o bin $(LD_FLAGS) $(pkg);)

generate: controller-gen
	$(CONTROLLER_GEN) object paths="./..."

manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=config/crds

.PHONY: proto
proto:
	protoc -I ./proto \
		--go_out . --go_opt paths=source_relative \
		--go-grpc_out . --go-grpc_opt paths=source_relative \
		--grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
		./proto/pkg/sampler/api/api.proto
	protoc -I ./proto \
		--go_out . --go_opt paths=source_relative \
		./proto/pkg/collector/api/api.proto


$(KUBEBUILDER_ASSETS):
	curl -sSLo envtest-bins.tar.gz "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-$(K8S_VERSION)-$(GOOS)-amd64.tar.gz"
	mkdir -p $(KUBEBUILDER)
	tar -C $(KUBEBUILDER) --strip-components=1 -zvxf envtest-bins.tar.gz
	rm envtest-bins.tar.gz

## --------------------------------------
.PHONY: docker
docker:	## Build the docker image
	docker build . --progress=plain -t usage-metrics-collector:v$(VERSION)

.PHONY: tools
tools: ## Install dev tools.
	tools/install.sh

.PHONY: lint
lint: tools ## Lint the code.
	golangci-lint run

.PHONY: reviewable
reviewable: all

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

run-local:
	docker build . -t usage-metrics-collector:v0.0.0
	kind load docker-image usage-metrics-collector:v0.0.0
	kustomize build config | kubectl apply -f -

## --------------------------------------
## License
## --------------------------------------

HAS_ADDLICENSE:=$(shell which addlicense)
.PHONY: verify-licenses
verify-licenses: addlicense
	find . -type f -name "*.go" ! -path "*/vendor/*" | xargs $(GOPATH)/bin/addlicense -check || (echo 'Run "make update"' && exit 1)

.PHONY: update-licenses
update-licenses: addlicense
	find . -type f -name "*.go" ! -path "*/vendor/*" | xargs $(GOPATH)/bin/addlicense -c "The Kubernetes Authors."

.PHONY: addlicense
addlicense:
ifndef HAS_ADDLICENSE
	go install github.com/google/addlicense@v1.0.0
endif
