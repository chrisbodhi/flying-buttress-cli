package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"buttress/internal/config"
	"buttress/internal/store"
	"buttress/internal/tui"
)

func newListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed spec packages",
		Long: `Read buttress.lock and display all installed spec packages with their
pinned content hash and install date.

Examples:
  buttress list
  buttress list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output the lock file as JSON")
	return cmd
}

func runList(asJSON bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	st, err := store.New(cfg.Registry.CacheDir, "")
	if err != nil {
		return err
	}

	lock, err := st.ReadLock()
	if err != nil {
		return err
	}

	return renderList(lock, asJSON, os.Stdout, os.Stderr)
}

// renderList writes the lock map to stdout in either a human-readable table
// or as indented JSON. The empty-state hint is written to stderr so it does
// not pollute scripts that pipe stdout.
func renderList(lock map[string]store.LockEntry, asJSON bool, stdout, stderr io.Writer) error {
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(lock)
	}

	if len(lock) == 0 {
		fmt.Fprintln(stderr, tui.StyleDim.Render("No specs installed. Run 'buttress add @org/pkg' to get started."))
		return nil
	}

	keys := make([]string, 0, len(lock))
	for k := range lock {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		e := lock[k]
		fmt.Fprintf(stdout, "%s  %s  %s\n",
			tui.StyleTitle.Render(store.LockKey(e.Org, e.Pkg)),
			tui.ShortHash(e.Hash),
			tui.StyleDim.Render(e.InstalledAt.Local().Format(time.RFC3339)))
	}
	return nil
}
