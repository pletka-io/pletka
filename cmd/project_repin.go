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
	rpFrom         string
	rpIncludeDraft bool
	rpDryRun       bool
	rpApply        bool
)

func newProjectRepinCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repin-children <parentID> <version>",
		Short: "Point a parent's children at one of its releases",
		Long: `Moves the children that inherit from a parent onto one of its releases.

A pinned child keeps reading the release it was pinned to, which is what
pinning means — so a parent's new release changes nothing its children see,
and anything added to the parent since the old release stays invisible to
them. Re-pinning is the operation that moves them, and it is bulk work the
UI can only do one child at a time.

Children that follow the parent's DRAFT are left alone unless
--include-draft is given: following draft is a deliberate choice, and
pinning such a child silently would take away the live view it asked for.

Dry-run by default. A parent or version that does not exist is an error, not
an empty report, so a mistyped argument cannot read as "nothing to do".`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE:         runProjectRepin,
	}
	cmd.Flags().StringVar(&rpFrom, "from", "", "only move children currently pinned at this version")
	cmd.Flags().BoolVar(&rpIncludeDraft, "include-draft", false, "also pin children that currently follow the parent's draft")
	cmd.Flags().BoolVar(&rpDryRun, "dry-run", true, "report the move without writing; --dry-run=false writes, exactly as --apply does")
	cmd.Flags().BoolVar(&rpApply, "apply", false, "write the new pins")
	return cmd
}

func runProjectRepin(cmd *cobra.Command, args []string) error {
	if rpApply && cmd.Flags().Changed("dry-run") && rpDryRun {
		return errors.New("--apply and --dry-run are mutually exclusive")
	}
	apply := rpApply || !rpDryRun

	ctx := context.Background()
	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	report, err := project.RepinChildren(ctx, pool, args[0], args[1], project.RepinOptions{
		From:         rpFrom,
		IncludeDraft: rpIncludeDraft,
		Apply:        apply,
	})
	if report != nil {
		if rerr := report.Render(os.Stdout); rerr != nil && err == nil {
			return fmt.Errorf("write report: %w", rerr)
		}
	}
	return err
}
