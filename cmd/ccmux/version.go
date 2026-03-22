package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Args:    cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
		},
	}
}

func printVersion() {
	v := buildVersion()
	fmt.Printf("ccmux %s\n", v)
	if GitCommit != "unknown" {
		fmt.Printf("  commit:     %s\n", GitCommit)
	}
	if BuildTime != "unknown" {
		fmt.Printf("  built:      %s\n", BuildTime)
	}
}
