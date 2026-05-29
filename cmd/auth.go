package cmd

import "github.com/spf13/cobra"

func newAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "No-op placeholder",
		RunE:  func(_ *cobra.Command, _ []string) error { return nil },
	}
}
