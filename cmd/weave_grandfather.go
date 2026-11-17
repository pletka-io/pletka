package cmd

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

var weaveGrandfatherOpts struct {
	note string
	end  bool
}

func newWeaveGrandfatherCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "grandfather <project> <flag> <subject>",
		Short: "Name a draft that may keep breaking a versioning rule",
		Long: `Records a named exception: one project, one flag, one subject. The audit
keeps listing the finding and marks it as grandfathered.

An exception is set once. --end closes it, and one that has ended cannot be
set again. Set it from an audit run, and say which one in --note.

Flags and their subject:
  ontology-mixed   the project links two versions of one ontology itself and
                   uses terms of both; the subject is the ontology prefix

Examples:
  pletka weave grandfather ABC ontology-mixed crm --note "audit of 2026-11-17"
  pletka weave grandfather ABC ontology-mixed crm --end`,
		Args: cobra.ExactArgs(3),
		RunE: runWeaveGrandfather,
	}
	c.Flags().StringVar(&weaveGrandfatherOpts.note, "note", "", "where the exception comes from, e.g. the audit run")
	c.Flags().BoolVar(&weaveGrandfatherOpts.end, "end", false, "end the exception instead of setting it")
	return c
}

func runWeaveGrandfather(cmd *cobra.Command, args []string) error {
	opts := weaveGrandfatherOpts
	project, flag, subject := args[0], args[1], args[2]
	cmd.SilenceUsage = true

	ctx := cmd.Context()
	pool, err := cliruntime.OpenPingedPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := sqlcgen.New(pool)

	if opts.end {
		n, err := q.WeaveEndRuleException(ctx, sqlcgen.WeaveEndRuleExceptionParams{ProjectID: project, Flag: flag, Subject: subject})
		if err != nil {
			return fmt.Errorf("end exception: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("no open exception %s %s %s", project, flag, subject)
		}
		fmt.Printf("ended: %s %s %s\n", project, flag, subject)
		return nil
	}

	_, err = q.WeaveSetRuleException(ctx, sqlcgen.WeaveSetRuleExceptionParams{ProjectID: project, Flag: flag, Subject: subject, Note: opts.note})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("exception %s %s %s exists or has ended; an exception is set once", project, flag, subject)
		case "23503":
			return fmt.Errorf("no project %s", project)
		case "23514":
			return fmt.Errorf("unknown flag %q", flag)
		}
	}
	if err != nil {
		return fmt.Errorf("set exception: %w", err)
	}
	fmt.Printf("grandfathered: %s %s %s\n", project, flag, subject)
	return nil
}
