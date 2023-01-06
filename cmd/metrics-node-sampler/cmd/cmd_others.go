//go:build !linux

package cmd

import (
	"github.com/spf13/cobra"
)

func initContainerMonitor(rootCmd *cobra.Command) {
	// Not implemented for Mac
	return
}
