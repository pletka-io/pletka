package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

const verifyFormatText = "text"

func newProjectVerifyGitCommand() *cobra.Command {
	var opts gitmaterializer.VerifyOptions
	var format string
	cmd := &cobra.Command{
		Use:   "verify-git <dir>",
		Short: "Check a project tree without a database: structure, references, and every field path as the path builder would offer it",
		Long: `verify-git reads an authored project tree and reports what load-git would
reject and what the path builder could not rebuild. It needs no database:
ontologies come from the tree's vendor/ontologies (pletka.sum verified) or
from --ontologies, a directory with the same layout. Parents come from
--parent (repeatable) or vendor/projects. Findings are printed as
file:line: code: message; any finding makes the command exit non-zero.`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != verifyFormatText && format != replayFormatJSON {
				return fmt.Errorf("invalid --format %q: want text or json", format)
			}
			rep, err := gitmaterializer.Verify(context.Background(), args[0], opts)
			if err != nil {
				return err
			}
			if err := printVerifyReport(cmd, format, rep); err != nil {
				return err
			}
			if len(rep.Findings) > 0 {
				return fmt.Errorf("%d finding(s)", len(rep.Findings))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&opts.ParentDirs, "parent", nil, "tree of a declared parent project (repeatable)")
	cmd.Flags().StringVar(&opts.OntologiesDir, "ontologies", "", "directory in vendor/ontologies layout to take ontologies from instead of the tree's vendor/")
	cmd.Flags().BoolVar(&opts.SkipOntology, "structure-only", false, "skip the ontology and path checks")
	cmd.Flags().StringVar(&format, "format", verifyFormatText, "output format: text or json")
	return cmd
}

func printVerifyReport(cmd *cobra.Command, format string, rep *gitmaterializer.VerifyReport) error {
	out := cmd.OutOrStdout()
	if format == replayFormatJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	for _, f := range rep.Findings {
		if _, err := fmt.Fprintln(out, f.String()); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "\nFields: %d  Steps: %d  Findings: %d\n", rep.Fields, rep.Steps, len(rep.Findings))
	return err
}
