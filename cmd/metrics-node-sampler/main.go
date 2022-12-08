package main

import (
	"sigs.k8s.io/usage-metrics-collector/cmd/metrics-node-sampler/cmd"
)

var (
	// Set by https://goreleaser.com/cookbooks/using-main.version?h=ld
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.Execute(version, commit, date)
}
