package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

var (
	aoChildrenOf   string
	aoPinnedTo     string
	aoIncludeDraft bool
	aoNote         string
	aoDryRun       bool
	aoApply        bool
)

func newProjectAddOntologyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-ontology <ontologyVersionID|prefix> --children-of <parentID>",
		Short: "Give a parent's children their own link to an ontology",
		Long: `Gives each selected child of a parent its own link to an ontology.

A child pinned to a parent's release resolves the ontologies that release
archived, and nothing the parent has added since. Re-pinning onto a newer
release is usually the answer — but not when that release also carries work
that is not ready, which would hand the children all of it to get one
ontology. Giving the child its own link is the alternative: it owns the
ontology whatever release it reads, and no release is rewritten to claim it
held something it did not.

Children that follow the parent's DRAFT are skipped unless --include-draft,
since they already resolve whatever the draft carries. Children that already
have the ontology are skipped too, so the command is safe to re-run.

A child that owns an ontology and later inherits the same one resolves it
once, own winning on overlap — a redundant row, not a conflict.

Dry-run by default.`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runProjectAddOntology,
	}
	cmd.Flags().StringVar(&aoChildrenOf, "children-of", "", "parent project whose children are given the ontology (required)")
	cmd.Flags().StringVar(&aoPinnedTo, "pinned-to", "", "only children pinned at this version of the parent")
	cmd.Flags().BoolVar(&aoIncludeDraft, "include-draft", false, "also give it to children that follow the parent's draft")
	cmd.Flags().StringVar(&aoNote, "note", "", "usage_notes written on each new link, so the row says why it exists")
	cmd.Flags().BoolVar(&aoDryRun, "dry-run", true, "report the change without writing; --dry-run=false writes, exactly as --apply does")
	cmd.Flags().BoolVar(&aoApply, "apply", false, "write the links")
	_ = cmd.MarkFlagRequired("children-of")
	return cmd
}

func runProjectAddOntology(cmd *cobra.Command, args []string) error {
	if aoApply && cmd.Flags().Changed("dry-run") && aoDryRun {
		return errors.New("--apply and --dry-run are mutually exclusive")
	}
	apply := aoApply || !aoDryRun

	ctx := context.Background()
	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	report, err := project.AddOntologyToChildren(ctx, pool, aoChildrenOf, args[0], project.AddOntologyOptions{
		PinnedTo:     aoPinnedTo,
		IncludeDraft: aoIncludeDraft,
		Note:         aoNote,
		Apply:        apply,
	})
	if report != nil {
		if rerr := report.Render(os.Stdout); rerr != nil && err == nil {
			return fmt.Errorf("write report: %w", rerr)
		}
	}
	return err
}
