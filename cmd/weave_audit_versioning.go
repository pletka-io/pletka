package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/versionaudit"
)

var weaveAuditVersioningOpts struct {
	format  string
	rules   []string
	timeout time.Duration
}

func newWeaveAuditVersioningCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "audit-versioning",
		Short: "List where the data breaks the versioning and inheritance rules",
		Long: `Reads parent links, releases, adoption receipts, ontology links and
categories and lists every place where they break a rule of
docs-oss/architecture/versioning-and-inheritance.md. Each finding names the
rule, the project, what it is about, and why.

Report-only: nothing is modified. Exits zero when the report was produced,
whether or not it has findings; findings are the expected result of running
this before a rule is enforced.

Limits of this version:
  - a project whose only parent is the legacy parent column is walked as
    following that parent's draft; L8 lists it
  - the closure walk follows every path, so its cost grows fast on data with
    many stacked diamonds; --timeout stops it
  - A8 compares a copied category with its source's draft
  - R3 lists every release that reaches a project under the shared version;
    where rows are already shared that list is an upper bound

Examples:
  pletka weave audit-versioning
  pletka weave audit-versioning --rule L5 --rule A2
  pletka weave audit-versioning --format json > audit.json`,
		RunE: runWeaveAuditVersioning,
	}
	c.Flags().StringVar(&weaveAuditVersioningOpts.format, "format", "text", "output format: text or json")
	c.Flags().StringArrayVar(&weaveAuditVersioningOpts.rules, "rule", nil,
		"limit the report to a rule id (repeatable); known: "+strings.Join(versionaudit.Rules, ", "))
	c.Flags().DurationVar(&weaveAuditVersioningOpts.timeout, "timeout", 5*time.Minute,
		"give up after this long; Ctrl-C also stops the queries")
	return c
}

// checkAuditVersioningFlags rejects a format or rule id the command does not
// know, before a database connection is opened.
func checkAuditVersioningFlags(format string, rules []string) error {
	if format != verifyFormatText && format != replayFormatJSON {
		return fmt.Errorf("invalid --format %q: want text or json", format)
	}
	if unknown := versionaudit.UnknownRules(rules); len(unknown) > 0 {
		return fmt.Errorf("unknown --rule %s: known rules are %s",
			strings.Join(unknown, ", "), strings.Join(versionaudit.Rules, ", "))
	}
	return nil
}

func runWeaveAuditVersioning(cmd *cobra.Command, args []string) error {
	opts := weaveAuditVersioningOpts
	if err := checkAuditVersioningFlags(opts.format, opts.rules); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	pool, err := cliruntime.OpenPingedPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	// One read-only snapshot: the checks agree with each other, and the
	// database itself refuses a write.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin read-only transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	findings, err := versionaudit.Run(ctx, sqlcgen.New(tx))
	if err != nil {
		return err
	}
	findings = versionaudit.Filter(findings, opts.rules)
	if opts.format == replayFormatJSON {
		return versionaudit.WriteJSON(os.Stdout, findings)
	}
	return versionaudit.WriteText(os.Stdout, findings)
}
