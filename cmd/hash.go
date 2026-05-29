package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"buttress/internal/contenthash"
)

func newHashCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "hash [dir]",
		Short: "Compute the content hash of a spec directory",
		Long: `Compute the content hash that buttress uses to verify a spec package.

Run this in your spec repository before creating a git tag and updating
VERSIONS.txt. The output is the value that goes on the hash line in VERSIONS.txt
and becomes the git tag name (with ':' replaced by '-').

Examples:
  buttress spec hash .
  buttress spec hash --verbose .
  buttress spec hash ./my-spec`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return runHash(dir, verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list each file included in the hash")
	return cmd
}

func runHash(dir string, verbose bool) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	hash, files, err := contenthash.HashDirVerbose(dir)
	if err != nil {
		return fmt.Errorf("hashing %s: %w", dir, err)
	}

	if verbose {
		for _, f := range files {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Println(hash)
	return nil
}
