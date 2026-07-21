package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

var weaveOntologyImportOpts struct {
	ontologyID string
	file       string
	version    string
	setActive  bool
	commit     bool
	jsonOut    bool
}

func newWeaveOntologyImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ontology-import",
		Short: "Probe or import an ontology version from an RDF file",
		Long: `CLI twin of the admin ontology upload (POST /admin/ontologies/{id}/versions/import).
Runs the exact same service path (ProbeImportVersion + ImportVersionWithOptions) against
the connected database, so namespace resolution is checked against live bindings.

Default is a dry-run probe: it parses the file, resolves qnames against the target
database's namespace bindings, and reports class/property counts, any missing namespace
bindings, warnings, and whether the version can be imported. Nothing is written unless
--commit is passed.

Examples:
  # Dry-run probe against the connected DB
  pletka weave ontology-import --ontology 0000000000QK4Q8BDE2S5KFTK4 --file CRMsurv_v1.0.2_nsfix.rdf

  # Actually import (replaces the version if the ID already exists)
  pletka weave ontology-import --ontology <id> --file fixed.rdf --commit --set-active`,
		RunE: runWeaveOntologyImport,
	}
	cmd.Flags().StringVar(&weaveOntologyImportOpts.ontologyID, "ontology", "", "target ontology id (required)")
	cmd.Flags().StringVar(&weaveOntologyImportOpts.file, "file", "", "RDF/XML file to probe or import (required)")
	cmd.Flags().StringVar(&weaveOntologyImportOpts.version, "version", "", "version string (default: derived from the file)")
	cmd.Flags().BoolVar(&weaveOntologyImportOpts.setActive, "set-active", false, "mark the imported version active (only with --commit)")
	cmd.Flags().BoolVar(&weaveOntologyImportOpts.commit, "commit", false, "perform the import; without it the command only probes (dry-run)")
	cmd.Flags().BoolVar(&weaveOntologyImportOpts.jsonOut, "json", false, "emit the probe result as JSON")
	return cmd
}

func runWeaveOntologyImport(cmd *cobra.Command, args []string) error {
	o := weaveOntologyImportOpts
	if o.ontologyID == "" || o.file == "" {
		cmd.SilenceUsage = true
		return fmt.Errorf("--ontology and --file are required")
	}
	content, err := os.ReadFile(o.file)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	ctx := context.Background()
	pool, err := cliruntime.OpenPingedPGXPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	weaveStore := weave.NewPostgresStore(pool)
	svc := ontology.NewService(ontology.NewPostgresStore(pool), weaveStore.Projects(), slog.Default())

	input := ontology.ImportUploadInput{
		OntologyID:    o.ontologyID,
		Filename:      filepath.Base(o.file),
		Content:       content,
		VersionString: o.version,
		SetActive:     o.setActive,
	}

	probe, importInput, err := svc.ProbeImportVersion(ctx, input)
	if err != nil {
		cmd.SilenceUsage = true
		return fmt.Errorf("probe: %w", err)
	}

	if o.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(probe); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	} else {
		printOntologyImportProbe(probe)
	}

	if !o.commit {
		fmt.Println("\n(dry-run — pass --commit to import)")
		return nil
	}
	if !probe.CanImport {
		cmd.SilenceUsage = true
		return fmt.Errorf("cannot import: probe reported can_import=false (see missing namespaces / warnings above)")
	}

	version, err := svc.ImportVersionWithOptions(ctx, importInput, ontology.ImportVersionOptions{})
	if err != nil {
		cmd.SilenceUsage = true
		return fmt.Errorf("import: %w", err)
	}
	fmt.Printf("\n✓ Imported version %q (id %s) into ontology %s\n", version.VersionString, version.ID, probe.OntologyPrefix)
	return nil
}

func printOntologyImportProbe(p *ontology.ImportProbeResult) {
	fmt.Println("=== Ontology Import Probe ===")
	fmt.Printf("Ontology:   %s (%s)\n", p.OntologyPrefix, p.OntologyID)
	fmt.Printf("Namespace:  %s\n", p.Namespace)
	fmt.Printf("Version:    %s (id %s)%s\n", p.VersionString, p.VersionID, existingSuffix(p.ExistingVersion))
	fmt.Printf("File:       %s (%d bytes)\n", p.Filename, p.FileSize)
	fmt.Printf("Parsed:     %d classes, %d properties, %d relations\n", p.ClassCount, p.PropertyCount, p.RelationCount)
	if len(p.MissingNamespaces) > 0 {
		fmt.Printf("\nMissing namespace bindings (%d):\n", len(p.MissingNamespaces))
		for _, m := range p.MissingNamespaces {
			fmt.Printf("  - %s\n", m.Namespace)
		}
	}
	if len(p.Warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(p.Warnings))
		for _, w := range p.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	fmt.Printf("\ncan_import: %v\n", p.CanImport)
}

func existingSuffix(existing bool) string {
	if existing {
		return "  [REPLACES existing version]"
	}
	return ""
}
