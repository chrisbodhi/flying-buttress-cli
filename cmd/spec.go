package cmd

import "github.com/spf13/cobra"

func newSpecCmd() *cobra.Command {
	spec := &cobra.Command{
		Use:   "spec",
		Short: "Manage spec packages",
		Long: `Commands for working with spec packages — downloading, scaffolding,
and generating implementations from specs.`,
	}
	spec.AddCommand(newAddCmd())
	spec.AddCommand(newGenerateCmd())
	spec.AddCommand(newHashCmd())
	spec.AddCommand(newInitCmd())
	spec.AddCommand(newListCmd())
	return spec
}
