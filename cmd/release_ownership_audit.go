package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func newReleaseOwnershipAuditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "audit-ownership",
		Short: "Report releases whose archive ownership needs verification",
		Long:  "Reports unverified historical releases as JSON. Collision indicators are leads for investigation, not proof of corruption. This command does not modify archives or infer historical content from current drafts.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pool, err := cliruntime.OpenPGXPool(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()
			rows, err := release.AuditArchiveOwnership(cmd.Context(), pool)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(rows); err != nil {
				return fmt.Errorf("write archive ownership audit: %w", err)
			}
			return nil
		},
	}
}
