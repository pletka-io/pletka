package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app"
	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func newWeaveCommand(renderers []generators.Renderer) *cobra.Command {
	weaveCmd := &cobra.Command{
		Use:   "weave",
		Short: "Weave data and generator commands",
	}
	weaveCmd.AddCommand(
		newWeaveGenerateCommand(renderers),
		newWeaveVerifyPathsCommand(),
		newWeaveOntologyImportCommand(),
	)
	return weaveCmd
}

func newWeaveGenerateCommand(renderers []generators.Renderer) *cobra.Command {
	available := availableFormats(renderers)
	weaveGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate output from weave entities",
		Long: `Generate CSV, Turtle, JSON-LD, Mermaid, or export graph output from the weave generator service.

Examples:
  pletka weave generate --project LA --kind model --id LAM.9 --format turtle --out tmp/LAM.9.weave.ttl
  pletka weave generate --project LA --kind model --id LAM.9 --format mermaid --out tmp/LAM.9.weave.mmd
  pletka weave generate --project LA --kind model --id LAM.9 --format exportgraph --out tmp/LAM.9.exportgraph.json
  pletka weave generate --project LA --kind collection --id LAC.1 --format csv
  pletka weave generate --project LA --kind model --id LAM.1 --format x3ml --out tmp/LAM.1.a.x3ml
  pletka weave generate --project LA --kind model --id LAM.1 --format x3ml-b --out tmp/LAM.1.b.x3ml
  pletka weave generate --project LA --kind model --id LAM.9 --format snapshot --out tmp/LAM.9.snapshot.json
  pletka weave generate --project LA --kind field --id LAF.4 --format turtle --base-prefix ex --base-uri https://example.org/ns/la/
  pletka weave generate --project LA --kind collection --id LAC.1 --format turtle --rdf-node-mode resource
  pletka weave generate --project LA --kind collection --id LAC.1 --format jsonld`,
		RunE: runWeaveGenerate(renderers),
	}

	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.projectID, "project", "", "project ID, e.g. LA")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.kind, "kind", "", "entity kind: model, collection, or field")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.id, "id", "", "entity ID, semantic ID, or system name (single-entity mode)")
	weaveGenerateCmd.Flags().BoolVar(&weaveGenerateOpts.all, "all", false, "render every entity of the given --kind for the project (currently --kind model only); writes one file per entity into --out-dir")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.outDir, "out-dir", "", "output directory for --all (one <slug>.json per entity); required when --all is set")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.format, "format", "turtle", fmt.Sprintf("output format: %s, snapshot (Snapshot/tree as JSON), ascii (human-readable tree with pn ids), or paths (per-leaf -> notation with old/new id brackets, sorted like the ascii tree)", strings.Join(sortedFormatNames(available), ", ")))
	weaveGenerateCmd.Flags().StringVarP(&weaveGenerateOpts.out, "out", "o", "", "output file path; stdout when omitted")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.basePrefix, "base-prefix", "ex", "RDF base namespace prefix alias")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.baseURI, "base-uri", "", "RDF base URI override; project namespace when omitted")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.rdfNode, "rdf-node-mode", string(generators.RDFNodeModeBlankNode), "RDF node mode: blank_node or resource")
	weaveGenerateCmd.Flags().StringVar(&weaveGenerateOpts.mermaid, "mermaid-mode", string(generators.MermaidModeOntology), "Mermaid mode: ontology or instance")
	weaveGenerateCmd.Flags().BoolVar(&weaveGenerateOpts.composeCollections, "compose-collections", false, "graft every collection that anchors at class X under every Collection-stub field whose path ends at X (snap-level pass; affects every renderer that consumes snap.Fields)")
	weaveGenerateCmd.Flags().IntVar(&weaveGenerateOpts.composeMaxDepth, "compose-max-depth", 0, "cap collection composition recursion (0 = built-in default, currently 10)")

	return weaveGenerateCmd
}

var weaveGenerateOpts struct {
	projectID          string
	kind               string
	id                 string
	all                bool
	outDir             string
	format             string
	out                string
	basePrefix         string
	baseURI            string
	rdfNode            string
	mermaid            string
	composeCollections bool
	composeMaxDepth    int
}

func runWeaveGenerate(renderers []generators.Renderer) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if weaveGenerateOpts.projectID == "" {
			return fmt.Errorf("--project is required")
		}
		if weaveGenerateOpts.kind == "" {
			return fmt.Errorf("--kind is required")
		}
		if weaveGenerateOpts.all {
			if weaveGenerateOpts.id != "" {
				return fmt.Errorf("--all and --id are mutually exclusive")
			}
			if weaveGenerateOpts.outDir == "" {
				return fmt.Errorf("--out-dir is required when --all is set")
			}
		} else if weaveGenerateOpts.id == "" {
			return fmt.Errorf("--id is required (or use --all)")
		}

		// "snapshot" / "tree" dump the renderer-neutral Snapshot itself (the
		// input every generator consumes) rather than running a renderer.
		dumpSnapshot := isSnapshotFormat(weaveGenerateOpts.format)
		var format generators.Format
		if !dumpSnapshot {
			parsed, err := parseGeneratorFormat(weaveGenerateOpts.format, availableFormats(renderers))
			if err != nil {
				return err
			}
			format = parsed
		}
		kind, err := parseGeneratorKind(weaveGenerateOpts.kind)
		if err != nil {
			return err
		}
		rdfNodeMode, err := parseRDFNodeMode(weaveGenerateOpts.rdfNode)
		if err != nil {
			return err
		}
		mermaidMode, err := parseMermaidMode(weaveGenerateOpts.mermaid)
		if err != nil {
			return err
		}

		ctx := auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true})
		pool, err := cliruntime.OpenPingedPGXPool(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		generatorRuntime, err := newGeneratorCLIRuntime(pool, renderers)
		if err != nil {
			return err
		}

		var w io.Writer = cmd.OutOrStdout()
		var f *os.File
		if weaveGenerateOpts.out != "" {
			f, err = os.Create(weaveGenerateOpts.out)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()
			w = f
		}

		genOpts := generators.Options{
			BasePrefix:         weaveGenerateOpts.basePrefix,
			BaseURI:            weaveGenerateOpts.baseURI,
			RDFNodeMode:        rdfNodeMode,
			MermaidMode:        mermaidMode,
			ComposeCollections: weaveGenerateOpts.composeCollections,
			ComposeMaxDepth:    weaveGenerateOpts.composeMaxDepth,
		}
		projectID := weaveGenerateOpts.projectID
		id := weaveGenerateOpts.id

		// --all path: iterate every entity of the requested kind and write
		// one file per entity into --out-dir. Currently scoped to --kind model
		// since that's the only kind with a list-by-project surface on the
		// resolver. Adding collection/field is a copy-paste away if needed.
		if weaveGenerateOpts.all {
			if kind != generators.EntityModel {
				return fmt.Errorf("--all currently supports --kind model only")
			}
			return runWeaveGenerateAll(ctx, generatorRuntime, projectID, format, dumpSnapshot, genOpts)
		}

		var generateErr error
		switch kind {
		case generators.EntityModel:
			modelID, err := generatorRuntime.ModelID(ctx, projectID, id)
			if err != nil {
				return err
			}
			if dumpSnapshot {
				snap, serr := generatorRuntime.Service.SnapshotForModel(ctx, projectID, modelID, genOpts)
				generateErr = writeSnapshotDump(w, snap, serr, weaveGenerateOpts.format)
			} else {
				generateErr = generatorRuntime.Service.GenerateModel(ctx, projectID, modelID, format, w, genOpts)
			}
		case generators.EntityCollection:
			collectionID, err := generatorRuntime.CollectionID(ctx, projectID, id)
			if err != nil {
				return err
			}
			if dumpSnapshot {
				snap, serr := generatorRuntime.Service.SnapshotForCollection(ctx, projectID, collectionID, genOpts)
				generateErr = writeSnapshotDump(w, snap, serr, weaveGenerateOpts.format)
			} else {
				generateErr = generatorRuntime.Service.GenerateCollection(ctx, projectID, collectionID, format, w, genOpts)
			}
		case generators.EntityField:
			fieldID, err := generatorRuntime.FieldID(ctx, projectID, id)
			if err != nil {
				return err
			}
			if dumpSnapshot {
				snap, serr := generatorRuntime.Service.SnapshotForField(ctx, projectID, fieldID, genOpts)
				generateErr = writeSnapshotDump(w, snap, serr, weaveGenerateOpts.format)
			} else {
				generateErr = generatorRuntime.Service.GenerateField(ctx, projectID, fieldID, format, w, genOpts)
			}
		default:
			return fmt.Errorf("unsupported entity kind %q", kind)
		}
		if generateErr != nil {
			return fmt.Errorf("generate %s %s as %s: %w", kind, id, weaveGenerateOpts.format, generateErr)
		}
		return nil
	}
}

// runWeaveGenerateAll lists every model in the project and writes one
// rendered file per model into --out-dir. Filenames are derived from the
// model's system_name (or ID when unset), suffixed with the format's
// file extension. Output matches the layout `archessql graph import
// --per-graph-dir` expects, so the canonical multi-model workflow is
// just: pletka weave generate --all -> archessql graph import.
func runWeaveGenerateAll(
	ctx context.Context,
	generatorRuntime *app.GeneratorRuntime,
	projectID string,
	format generators.Format,
	dumpSnapshot bool,
	genOpts generators.Options,
) error {
	models, err := generatorRuntime.ListModels(ctx, projectID)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("project %s has no models", projectID)
	}
	if err := os.MkdirAll(weaveGenerateOpts.outDir, 0o755); err != nil {
		return fmt.Errorf("create out-dir %s: %w", weaveGenerateOpts.outDir, err)
	}
	ext := ".json"
	if !dumpSnapshot {
		ext = extensionFor(format)
	} else if isSnapshotASCIIFormat(weaveGenerateOpts.format) {
		ext = ".txt"
	}
	for _, m := range models {
		if m == nil || m.ID == "" {
			continue
		}
		base := strings.TrimSpace(m.SystemName)
		if base == "" {
			base = m.ID
		}
		filename := safeFilename(base) + ext
		path := filepath.Join(weaveGenerateOpts.outDir, filename)
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		var genErr error
		if dumpSnapshot {
			snap, serr := generatorRuntime.Service.SnapshotForModel(ctx, projectID, m.ID, genOpts)
			genErr = writeSnapshotDump(f, snap, serr, weaveGenerateOpts.format)
		} else {
			genErr = generatorRuntime.Service.GenerateModel(ctx, projectID, m.ID, format, f, genOpts)
		}
		f.Close()
		if genErr != nil {
			return fmt.Errorf("generate model %s -> %s: %w", m.ID, path, genErr)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	}
	fmt.Fprintf(os.Stderr, "wrote %d models to %s\n", len(models), weaveGenerateOpts.outDir)
	return nil
}

// extensionFor returns the file extension a registered renderer claims
// via its FormatSpec. Falls back to ".out" so callers always get a
// deterministic name.
func extensionFor(format generators.Format) string {
	switch format {
	case generators.FormatJSONLD, generators.FormatExportGraph:
		return ".json"
	case generators.FormatTurtle:
		return ".ttl"
	case generators.FormatCSV:
		return ".csv"
	case generators.FormatMermaid:
		return ".mmd"
	case generators.FormatX3ML, generators.FormatX3MLB:
		return ".x3ml"
	}
	return ".out"
}

// safeFilename normalises a free-form identifier into a filesystem-safe
// basename. Any character outside [A-Za-z0-9._-] becomes an underscore;
// repeated underscores collapse to one.
var safeFilenameRegex = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeFilename(s string) string {
	if s == "" {
		return "model"
	}
	out := safeFilenameRegex.ReplaceAllString(s, "_")
	return strings.Trim(out, "_")
}

// isSnapshotFormat reports whether the requested format is a request to
// dump the Snapshot itself rather than run a renderer.
func isSnapshotFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "snapshot", "tree", "ascii", "ascii-tree", "tree-ascii", "paths", "path-list", "arrows":
		return true
	default:
		return false
	}
}

// isSnapshotASCIIFormat reports whether the snapshot dump should be the
// human-readable ASCII tree (for curator review of node ids) rather
// than the raw JSON shape.
func isSnapshotASCIIFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ascii", "ascii-tree", "tree-ascii":
		return true
	default:
		return false
	}
}

// isSnapshotPathsFormat reports whether the snapshot dump should be the
// per-leaf arrow notation with old/new id brackets (PathNodeID drift
// review) rather than the ASCII tree.
func isSnapshotPathsFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "paths", "path-list", "arrows":
		return true
	default:
		return false
	}
}

// writeSnapshotJSON marshals the renderer-neutral Snapshot — the input
// every generator consumes, including the views.Tree and the resolved
// field paths — as indented JSON.
func writeSnapshotJSON(w io.Writer, snap *generators.Snapshot, err error) error {
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

// writeSnapshotDump dispatches a snapshot dump to the JSON or ASCII
// writer based on the requested format.
func writeSnapshotDump(w io.Writer, snap *generators.Snapshot, err error, format string) error {
	if err != nil {
		return err
	}
	if isSnapshotASCIIFormat(format) {
		return generators.WriteSnapshotASCII(w, snap)
	}
	if isSnapshotPathsFormat(format) {
		return generators.WriteSnapshotPaths(w, snap)
	}
	return writeSnapshotJSON(w, snap, nil)
}

// availableFormats indexes the renderer set's canonical formats so
// parseGeneratorFormat can check whether a host-injected format (one with
// no alias entry below) is actually wired in.
func availableFormats(renderers []generators.Renderer) map[generators.Format]bool {
	m := make(map[generators.Format]bool, len(renderers))
	for _, r := range renderers {
		m[r.Spec().Format] = true
	}
	return m
}

// sortedFormatNames returns the available formats' canonical names, sorted,
// for embedding in the --format flag's help text.
func sortedFormatNames(available map[generators.Format]bool) []string {
	names := make([]string, 0, len(available))
	for f := range available {
		names = append(names, string(f))
	}
	sort.Strings(names)
	return names
}

func parseGeneratorFormat(value string, available map[generators.Format]bool) (generators.Format, error) {
	var f generators.Format
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "csv":
		f = generators.FormatCSV
	case "json-ld", "jsonld", "ld+json":
		f = generators.FormatJSONLD
	case "ttl", "turtle", "rdf":
		f = generators.FormatTurtle
	case "mermaid", "mmd", "diagram":
		f = generators.FormatMermaid
	case "exportgraph", "export-graph", "graph", "templategraph", "template-graph":
		f = generators.FormatExportGraph
	case "x3ml", "x3ml-a", "x3mla":
		f = generators.FormatX3ML
	case "x3ml-b", "x3mlb":
		f = generators.FormatX3MLB
	}
	// Direct-name fallback: a host-injected renderer (or a core renderer
	// with no alias entry above, e.g. sparql/researchspace) is parseable
	// by its canonical Format name with no alias switch entry required.
	if f == "" {
		if candidate := generators.Format(strings.ToLower(strings.TrimSpace(value))); available[candidate] {
			return candidate, nil
		}
		return "", fmt.Errorf("unsupported format %q", value)
	}
	if !available[f] {
		return "", fmt.Errorf("unsupported format %q", value)
	}
	return f, nil
}

func parseRDFNodeMode(value string) (generators.RDFNodeMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(generators.RDFNodeModeBlankNode), "blank", "blank_nodes":
		return generators.RDFNodeModeBlankNode, nil
	case string(generators.RDFNodeModeResource), "resources":
		return generators.RDFNodeModeResource, nil
	default:
		return "", fmt.Errorf("unsupported RDF node mode %q", value)
	}
}

func parseMermaidMode(value string) (generators.MermaidMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(generators.MermaidModeOntology), "class", "classes":
		return generators.MermaidModeOntology, nil
	case string(generators.MermaidModeInstance), "instances", "resource", "resources":
		return generators.MermaidModeInstance, nil
	default:
		return "", fmt.Errorf("unsupported Mermaid mode %q", value)
	}
}

func parseGeneratorKind(value string) (generators.EntityKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "model", "models":
		return generators.EntityModel, nil
	case "collection", "collections":
		return generators.EntityCollection, nil
	case "field", "fields":
		return generators.EntityField, nil
	default:
		return "", fmt.Errorf("unsupported kind %q", value)
	}
}

func newGeneratorCLIRuntime(pool *pgxpool.Pool, renderers []generators.Renderer) (*app.GeneratorRuntime, error) {
	opts := app.Options{Pool: pool, Logger: logger}
	opts.GeneratorRenderers = renderers
	return app.NewGeneratorRuntime(opts)
}
