package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

var (
	rrProject         string
	rrDryRun          bool
	rrApply           bool
	rrAcceptAmbiguous bool
)

func newReleaseReconstructCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconstruct",
		Short: "Fill the examples/vocabulary/placement/credit archives for releases cut before they existed",
		Long: `Seven archive tables (examples, example values, vocabularies, vocabulary
entries, collection placements, project attributions, project actors) were
added to the release snapshot after most releases had already been cut. Those
releases carry no rows in them. This command reconstructs what it safely can.

It does NOT snapshot today's draft. On a fleet where projects have been edited
since their last release that would publish in-progress work under an existing
version number. It reconstructs from row timestamps instead, and classifies
every live row against the release's created_at:

  exact      created before the release and untouched since — its current
             content IS the release's content, so it is archived
  excluded   created after the release — confidently not in it
  ambiguous  created before the release but edited since — it belonged to the
             release, but its pre-release content is gone. Reported with its
             id and both timestamps, never guessed, never archived.

weave_project_attributions and weave_project_actors have no updated_at column,
so an edit to an existing row is invisible to this method; those two tables
print an explicit caveat. Rows deleted since a release are likewise
unrecoverable and invisible.

Dry run by default. --apply writes, and refuses with a non-zero exit if any
ambiguous row was found unless --accept-ambiguous is also given — an operator
must not be able to fill the archives while silently skipping rows they were
never told about. The inserts are idempotent, so a writing run is safe to
re-run. --dry-run=false writes exactly as --apply does.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runReleaseReconstruct,
	}
	cmd.Flags().StringVar(&rrProject, "project", "", "limit to one project id (default: every project with releases)")
	cmd.Flags().BoolVar(&rrDryRun, "dry-run", true, "classify and report without writing anything; --dry-run=false writes, exactly as --apply does")
	cmd.Flags().BoolVar(&rrApply, "apply", false, "write the exact rows into the archives")
	cmd.Flags().BoolVar(&rrAcceptAmbiguous, "accept-ambiguous", false, "when writing: proceed despite ambiguous rows, archiving only the unambiguous ones")
	return cmd
}

func runReleaseReconstruct(cmd *cobra.Command, _ []string) error {
	if rrApply && cmd.Flags().Changed("dry-run") && rrDryRun {
		return errors.New("--apply and --dry-run are mutually exclusive")
	}
	// --dry-run is honoured, not merely declared: an operator who reads
	// `--dry-run=false` as "this will write" is not being unreasonable, and a
	// flag that silently means nothing is worse than no flag. Either spelling
	// writes; the default (dry-run true, apply false) does not.
	apply := rrApply || !rrDryRun
	if rrAcceptAmbiguous && !apply {
		return errors.New("--accept-ambiguous only means something on a writing run (--apply or --dry-run=false)")
	}

	ctx := context.Background()
	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	report, err := release.Reconstruct(ctx, pool, release.ReconstructOptions{
		ProjectID:       rrProject,
		Apply:           apply,
		AcceptAmbiguous: rrAcceptAmbiguous,
	})
	if report != nil {
		report.Render(os.Stdout, apply)
	}
	switch {
	case errors.Is(err, release.ErrAmbiguousRows):
		// Non-zero exit, with the ambiguous rows already printed above.
		return fmt.Errorf("%w — nothing was written", err)
	case errors.Is(err, release.ErrNoSuchProject):
		return fmt.Errorf("%w — check the spelling of --project; nothing was written", err)
	case errors.Is(err, release.ErrProjectHasNoReleases):
		return fmt.Errorf("%w — there is nothing to reconstruct for it; nothing was written", err)
	}
	return err
}
