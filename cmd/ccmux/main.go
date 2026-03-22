package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Set by goreleaser via -ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func buildVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 8 {
				return "dev-" + s.Value[:8]
			}
		}
	}
	return "dev"
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ccmux",
		Short:         "Telegram bridge for Claude Code",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		newGatewayCommand(),
		newHookCommand(),
		newVersionCommand(),
	)

	return cmd
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
