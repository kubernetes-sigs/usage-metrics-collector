package main

import (
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/usage-metrics-collector/cmd/metrics-prometheus-collector/cmd"
)

var (
	// Set by https://goreleaser.com/cookbooks/using-main.version?h=ld
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	_ = cmd.Execute(version, commit, date)
}
