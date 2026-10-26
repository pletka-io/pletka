package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func newReleaseDependencyAuditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "audit-dependencies",
		Short: "Report releases without a captured project dependency inventory",
		Long:  "Reports unverified project dependency inventories as JSON, including retired releases. These releases cannot be sources for new cuts until verified from historical evidence or replaced by new cuts. This command does not modify data or certify ontology artifacts.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pool, err := cliruntime.OpenPGXPool(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()
			rows, err := release.AuditProjectDependencies(cmd.Context(), pool)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(rows); err != nil {
				return fmt.Errorf("write project dependency audit: %w", err)
			}
			return nil
		},
	}
}
