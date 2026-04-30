// Package cmd contains the Flying Buttress CLI commands.
package cmd

import (
	"context"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

// Execute is the entry point called from main.
func Execute(ctx context.Context) error {
	root := &cobra.Command{
		Use:   "buttress",
		Short: "Flying Buttress — spec registry CLI",
		Long: `buttress is the command-line interface to the Flying Buttress spec registry.

Distribute docs, tests, specs, and type definitions — let your LLM generate
the implementation.`,
		SilenceUsage: true,
	}

	root.AddCommand(newAddCmd())
	root.AddCommand(newGenerateCmd())
	root.AddCommand(newHashCmd())

	return fang.Execute(ctx, root)
}
