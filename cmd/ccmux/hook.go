package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ccmux/ccmux/internal/hook"
)

func newHookCommand() *cobra.Command {
	var install bool

	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Manage Claude Code hooks",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if install {
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("resolving executable path: %w", err)
				}
				return hook.Install(exe)
			}
			return hook.Run()
		},
	}

	cmd.Flags().BoolVar(&install, "install", false, "Install the hook into ~/.claude/settings.json")

	return cmd
}
