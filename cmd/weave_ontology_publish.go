package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
)

func ontologyPublicationFlags(cmd *cobra.Command, opts *ontologyrelease.Options) {
	cmd.Flags().StringToStringVar(&opts.Pins, "pin", nil, "concrete dependency ontologyID=versionID (repeatable)")
	cmd.Flags().StringArrayVar(&opts.ExternalImports, "external-import", nil, "exact owl:imports IRI retained as an external reference (repeatable)")
	cmd.Flags().StringArrayVar(&opts.ExternalReferences, "external-reference", nil, "exact term URI/qname retained as an external reference (repeatable)")
}

func newWeaveOntologyPublishCommand() *cobra.Command {
	var opts ontologyrelease.Options
	cmd := &cobra.Command{
		Use:   "ontology-publish <ontologyID> <versionID>",
		Short: "Publish current ontology bytes with concrete dependency pins",
		Long:  "Permanently seals the current body, resolves base/import/default dependencies, validates term references and retains the complete publication closure in one transaction. Dependencies must already be published; publish them first. This establishes a guarantee from now onward, not evidence of historical content. There is no unpublish or force-overwrite operation.",
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
			artifact, err := ontologyrelease.PublishCurrent(cmd.Context(), pool, ref, opts)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				OntologyID ontologybody.OntologyID `json:"ontology_id"`
				VersionID  ontologybody.VersionID  `json:"version_id"`
				Digest     string                  `json:"artifact_digest"`
			}{ref.OntologyID(), ref.VersionID(), artifact.Digest})
		},
	}
	ontologyPublicationFlags(cmd, &opts)
	return cmd
}
