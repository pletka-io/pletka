package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

func newWeaveOntologyDraftCommand() *cobra.Command {
	command := &cobra.Command{Use: "ontology-draft", Short: "Edit isolated ontology drafts and publish immutable releases"}
	command.AddCommand(newOntologyDraftStartCommand(), newOntologyDraftShowCommand(), newOntologyDraftSaveCommand(), newOntologyDraftPublishCommand())
	return command
}
func runOntologyDraft(cmd *cobra.Command, fn func(context.Context, *ontology.DraftService) (any, error)) error {
	pool, err := cliruntime.OpenPGXPool(cmd.Context())
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := fn(cmd.Context(), ontology.NewDraftService(pool, nil))
	if err != nil {
		return err
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
}
func readOntologyDraftDocument(cmd *cobra.Command, path string) (ontology.DraftDocument, error) {
	var document ontology.DraftDocument
	reader := cmd.InOrStdin()
	if path == "" {
		return document, fmt.Errorf("--document is required (JSON file or - for stdin)")
	}
	if path != "-" {
		//nolint:gosec // The operator explicitly selects the local input file.
		file, err := os.Open(path)
		if err != nil {
			return document, err
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return document, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return document, fmt.Errorf("document must contain exactly one JSON object")
	}
	return document, nil
}
func newOntologyDraftStartCommand() *cobra.Command {
	var documentPath, base string
	command := &cobra.Command{Use: "start <ontologyID>", Short: "Create a draft from a document or published source", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if (documentPath == "") == (base == "") {
			return fmt.Errorf("choose exactly one of --document and --from-release")
		}
		var document ontology.DraftDocument
		if documentPath != "" {
			var err error
			document, err = readOntologyDraftDocument(cmd, documentPath)
			if err != nil {
				return err
			}
		}
		return runOntologyDraft(cmd, func(ctx context.Context, s *ontology.DraftService) (any, error) {
			if base != "" {
				return s.StartFromRelease(ctx, ontologybody.OntologyID(args[0]), ontologybody.VersionID(base))
			}
			return s.Start(ctx, ontologybody.OntologyID(args[0]), document)
		})
	}}
	command.Flags().StringVar(&documentPath, "document", "", "draft document JSON file, or - for stdin")
	command.Flags().StringVar(&base, "from-release", "", "published version ID owned by this ontology")
	return command
}
func newOntologyDraftShowCommand() *cobra.Command {
	return &cobra.Command{Use: "show <ontologyID>", Short: "Read the current draft identity, revision and document", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return runOntologyDraft(cmd, func(ctx context.Context, s *ontology.DraftService) (any, error) {
			return s.Current(ctx, ontologybody.OntologyID(args[0]))
		})
	}}
}
func newOntologyDraftSaveCommand() *cobra.Command {
	var documentPath string
	var revision int64
	command := &cobra.Command{Use: "save <ontologyID> <draftID>", Short: "Save a document only at the expected draft revision", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if revision < 1 {
			return fmt.Errorf("--revision must be positive")
		}
		ref, err := ontology.NewDraftReference(ontologybody.OntologyID(args[0]), ontology.DraftID(args[1]))
		if err != nil {
			return err
		}
		document, err := readOntologyDraftDocument(cmd, documentPath)
		if err != nil {
			return err
		}
		return runOntologyDraft(cmd, func(ctx context.Context, s *ontology.DraftService) (any, error) {
			var out ontology.DraftSnapshot
			err := s.Edit(ctx, ref, revision, func(editor ontology.DraftEditor) error {
				var err error
				out, err = editor.Save(ctx, document)
				return err
			})
			return out, err
		})
	}}
	command.Flags().StringVar(&documentPath, "document", "", "draft document JSON file, or - for stdin")
	command.Flags().Int64Var(&revision, "revision", 0, "expected revision from show")
	return command
}
func newOntologyDraftPublishCommand() *cobra.Command {
	var revision int64
	var request ontology.DraftPublishRequest
	command := &cobra.Command{Use: "publish <ontologyID> <draftID>", Short: "Publish an expected draft revision as a new immutable release", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if revision < 1 || request.Version == "" {
			return fmt.Errorf("--revision and --version are required")
		}
		ref, err := ontology.NewDraftReference(ontologybody.OntologyID(args[0]), ontology.DraftID(args[1]))
		if err != nil {
			return err
		}
		return runOntologyDraft(cmd, func(ctx context.Context, s *ontology.DraftService) (any, error) {
			return s.Publish(ctx, ref, revision, request)
		})
	}}
	command.Flags().Int64Var(&revision, "revision", 0, "expected revision from show")
	command.Flags().StringVar(&request.Version, "version", "", "new ontology release label")
	command.Flags().BoolVar(&request.SetActive, "set-active", false, "select the new release in the publication transaction")
	return command
}
