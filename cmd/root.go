// Package cmd contains the Flying Buttress CLI commands.
package cmd

import (
	"context"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

// newRootCmd builds the root cobra command tree. Exposed (unexported) so tests
// can drive the command graph without going through fang/os.Args.
func newRootCmd() *cobra.Command {
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
	root.AddCommand(newListCmd())
	return root
}

// Execute is the entry point called from main.
func Execute(ctx context.Context) error {
	return fang.Execute(ctx, newRootCmd())
}
