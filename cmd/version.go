package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dankomiocevic/ghoti/internal/buildinfo"
)

// NewVersionCommand returns the command to get ghoti version.
func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Return Ghoti version",
		Long:  "Return Ghoti version",
		RunE:  version,
		Args:  cobra.NoArgs,
	}

	return cmd
}

// print out the built version.
func version(cmd *cobra.Command, _ []string) error {
	cmd.Printf("Ghoti version `%s` build from `%s` on `%s` ", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	return nil
}
