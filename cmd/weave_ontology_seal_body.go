package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func newWeaveOntologySealBodyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ontology-seal-body <ontologyID> <versionID>",
		Short: "Permanently seal the current local body of an ontology version",
		Long:  "Captures the current metadata, terms, relations, companion files, catalog descriptor and namespace context, then prevents body edits and deletion in storage. The IDs are ontology and version storage identities, not project IDs or version labels. This operation establishes a guarantee from now onward; it does not certify historical content, resolve dependency/default pins, or migrate project materialization and restore. New imports remain unsealed until explicitly sealed. There is no unseal operation.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := ontologybody.NewReference(ontologybody.OntologyID(args[0]), ontologybody.VersionID(args[1]))
			if err != nil {
				return err
			}
			pool, err := cliruntime.OpenPGXPool(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()
			tx, err := pool.BeginTx(cmd.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				return err
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
				defer cancel()
				_ = tx.Rollback(cleanup)
			}()
			if err := ontologybody.Seal(cmd.Context(), sqlcgen.New(tx), ref); err != nil {
				return fmt.Errorf("seal ontology body: %w", err)
			}
			if err := tx.Commit(cmd.Context()); err != nil {
				return fmt.Errorf("commit ontology body seal: %w", err)
			}
			var body ontologybody.Body
			if err := ontologybody.NewFactory(pool).Read(cmd.Context(), ref, func(r ontologybody.Reader) error {
				var readErr error
				body, readErr = r.Body(cmd.Context())
				return readErr
			}); err != nil {
				return err
			}
			receipt := struct {
				OntologyID    ontologybody.OntologyID `json:"ontology_id"`
				VersionID     ontologybody.VersionID  `json:"version_id"`
				FormatVersion int16                   `json:"body_format_version"`
				Digest        string                  `json:"body_digest"`
				SealedAt      time.Time               `json:"sealed_at"`
			}{ref.OntologyID(), ref.VersionID(), body.FormatVersion, body.Digest, body.SealedAt}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(receipt)
		},
	}
}
