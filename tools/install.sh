#/bin/bash

set -e

# Tools are recorded as imports in tools.go, this command extracts the import directive, lists the dependencies and installs them
grep '^\s*_' tools/tools.go | awk '{print $2}' | xargs -t go install -mod=readonly -modfile=tools/go.mod
