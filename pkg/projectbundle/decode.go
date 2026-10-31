package projectbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

// ImportedProject is structurally validated transport, not a locally captured
// release or authorization grant. File hashes detect inconsistency; they do not
// authenticate the sender or validate PostgreSQL receipt hashes against rows.
// A future storage importer must explicitly preserve this provenance boundary.
type ImportedProject struct {
	value projectsnapshot.CompleteProject
}

// Snapshot returns detached content for inspection or rendering, without granting
// a database write/capture capability. Its zero value remains invalid.
func (p ImportedProject) Snapshot() projectsnapshot.CompleteProject { return p.value }

// Decode accepts only canonical format 3. It checks paths, budgets, file hashes,
// explicit section/column inventories and the complete graph/evidence contracts.
// Re-encoding must reproduce every supplied byte: duplicate/unknown/missing
// fields, noncanonical scalars, extra files and mixed-format trees are errors.
// Historical formats 1/2 belong to the separate legacy loader.
func Decode(files Files, limits Limits) (ImportedProject, error) {
	if err := checkLimits(files, limits); err != nil {
		return ImportedProject{}, err
	}
	var index manifest
	if err := json.Unmarshal(files[ManifestPath], &index); err != nil {
		return ImportedProject{}, err
	}
	if index.SchemaVersion != SchemaVersion {
		return ImportedProject{}, fmt.Errorf("unsupported complete-project schema %d", index.SchemaVersion)
	}
	if len(index.Projects) == 0 || len(index.Projects) > limits.MaxProjects {
		return ImportedProject{}, fmt.Errorf("bundle project budget exceeded or empty inventory")
	}
	decoder := bundleDecoder{files: files, remaining: limits.MaxBytes, consumed: map[string]bool{}}
	rootID, err := index.Root.identity()
	if err != nil {
		return ImportedProject{}, err
	}
	var root projectsnapshot.ProjectInput
	sources := make([]projectsnapshot.ProjectInput, 0, len(index.Projects)-1)
	foundRoot := false
	for _, ref := range index.Projects {
		in, err := decoder.context(ref)
		if err != nil {
			return ImportedProject{}, fmt.Errorf("decode context %s: %w", ref.Path, err)
		}
		if in.Evidence.Identity() == rootID {
			if foundRoot {
				return ImportedProject{}, fmt.Errorf("duplicate root context")
			}
			root, foundRoot = in, true
		} else {
			sources = append(sources, in)
		}
	}
	if !foundRoot {
		return ImportedProject{}, fmt.Errorf("bundle root is absent")
	}
	value, err := projectsnapshot.NewComplete(root, sources)
	if err != nil {
		return ImportedProject{}, err
	}
	canonicalFiles, err := Encode(value)
	if err != nil {
		return ImportedProject{}, err
	}
	if len(canonicalFiles) != len(files) {
		return ImportedProject{}, fmt.Errorf("bundle contains unlisted files")
	}
	for path, data := range canonicalFiles {
		if !bytes.Equal(data, files[path]) {
			return ImportedProject{}, fmt.Errorf("noncanonical or incomplete bundle file %q", path)
		}
	}
	return ImportedProject{value: value}, nil
}

func checkLimits(files Files, limits Limits) error {
	if limits.MaxFiles <= 0 || limits.MaxBytes <= 0 || limits.MaxProjects <= 0 {
		return fmt.Errorf("bundle limits must be positive")
	}
	if len(files) > limits.MaxFiles {
		return fmt.Errorf("bundle file budget exceeded")
	}
	var size int64
	for path, data := range files {
		if !validPath(path) {
			return fmt.Errorf("unsafe bundle path %q", path)
		}
		// Compare before addition, including path bytes, to avoid overflow.
		for _, n := range []int{len(path), len(data)} {
			if int64(n) > limits.MaxBytes-size {
				return fmt.Errorf("bundle byte budget exceeded")
			}
			size += int64(n)
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("bundle file %q is not UTF-8", path)
		}
	}
	return nil
}

func (d *bundleDecoder) context(ref fileRef) (projectsnapshot.ProjectInput, error) {
	data, err := d.read(ref, true)
	if err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	var c contextFile
	if err := json.Unmarshal(data, &c); err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	id, err := c.Identity.identity()
	if err != nil {
		return projectsnapshot.ProjectInput{}, err
	}
	in := projectsnapshot.ProjectInput{Counts: map[string]int64{}, Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}
	if c.Dependencies == nil || c.Membership == nil {
		return in, fmt.Errorf("dependency or membership inventory is absent")
	}
	for _, dep := range c.Dependencies {
		selected, err := dep.identity()
		if err != nil {
			return in, err
		}
		in.Dependencies = append(in.Dependencies, selected)
	}
	for _, member := range c.Membership {
		source, err := member.Source.identity()
		if err != nil {
			return in, err
		}
		in.Membership = append(in.Membership, projectsnapshot.Membership{Kind: member.Kind, ID: member.ID, Source: source})
	}
	in.Evidence, err = d.evidence(id, c.Evidence)
	if err != nil {
		return in, err
	}
	sections := map[string][]json.RawMessage{}
	if len(c.Sections) != len(projectsnapshot.Coverage()) {
		return in, fmt.Errorf("section inventory differs")
	}
	for _, section := range projectsnapshot.Coverage() {
		refs, ok := c.Sections[section.Name]
		if !ok || refs == nil {
			return in, fmt.Errorf("section %s is absent", section.Name)
		}
		rows := make([]json.RawMessage, 0, len(refs))
		for _, ref := range refs {
			data, err := d.read(ref, true)
			if err != nil {
				return in, err
			}
			row, err := internalRow(data, section)
			if err != nil {
				return in, err
			}
			rows = append(rows, row)
		}
		sections[section.Name] = rows
		in.Counts[section.Name] = int64(len(rows))
	}
	data, err = json.Marshal(sections)
	if err != nil {
		return in, err
	}
	err = json.Unmarshal(data, &in.Content)
	return in, err
}

func (d *bundleDecoder) evidence(id projectsnapshot.Identity, e evidenceFile) (projectsnapshot.Evidence, error) {
	data, err := json.Marshal(e.Descriptors)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	descriptors, err := releasedescriptor.Decode(data, id.ProjectID(), id.Version())
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	if id.IsDraft() != (e.Contract == 0) {
		return projectsnapshot.Evidence{}, fmt.Errorf("evidence contract differs from selection")
	}
	if e.Contract == 2 {
		return d.legacy(id, descriptors, e)
	}
	if e.Legacy != nil || e.Publications == nil || e.References == nil {
		return projectsnapshot.Evidence{}, fmt.Errorf("invalid published evidence inventory")
	}
	publications := make([]ontologyartifact.ProjectArtifact, 0, len(e.Publications))
	for _, p := range e.Publications {
		data, err := d.read(p.Artifact, false)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		publications = append(publications, ontologyartifact.ProjectArtifact{Artifact: ontologyartifact.Artifact{JSON: string(data), Digest: p.Artifact.SHA256}, Linked: p.Linked, Primary: p.Primary})
	}
	if e.Contract == 0 {
		if e.Receipt != nil {
			return projectsnapshot.Evidence{}, fmt.Errorf("draft cannot carry a capture receipt")
		}
		return projectsnapshot.DraftEvidence(id, descriptors, publications, e.References)
	}
	if e.Contract != 1 || e.Receipt == nil {
		return projectsnapshot.Evidence{}, fmt.Errorf("unsupported or absent capture contract")
	}
	data, err = json.Marshal(e.Receipt)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	receipt, err := releasecontent.DecodeVerified(data, id.ProjectID(), id.Version())
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	return projectsnapshot.VerifiedEvidence(receipt, descriptors, publications, e.References)
}

func (d *bundleDecoder) legacy(id projectsnapshot.Identity, descriptors releasedescriptor.Snapshot, e evidenceFile) (projectsnapshot.Evidence, error) {
	if e.Legacy == nil || e.Receipt == nil || e.Publications != nil || e.References != nil {
		return projectsnapshot.Evidence{}, fmt.Errorf("invalid legacy evidence inventory")
	}
	data, err := d.read(*e.Legacy, false)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	artifact := ontologyevidence.Artifact{JSON: string(data), Digest: e.Legacy.SHA256}
	receiptJSON, err := json.Marshal(e.Receipt)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	receipt, err := releasecontent.DecodeLegacy(receiptJSON, id.ProjectID(), id.Version())
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	return projectsnapshot.LegacyEvidence(receipt, descriptors, artifact)
}

// Count repeated evidence reads too: sharing a huge artifact across many tiny
// manifests must not amplify decoding work beyond the explicit byte budget.
type bundleDecoder struct {
	files     Files
	remaining int64
	consumed  map[string]bool
}

func (d *bundleDecoder) read(ref fileRef, unique bool) ([]byte, error) {
	data, ok := d.files[ref.Path]
	if !ok {
		return nil, fmt.Errorf("missing bundle file %q", ref.Path)
	}
	if unique && d.consumed[ref.Path] {
		return nil, fmt.Errorf("repeated structural file %q", ref.Path)
	}
	if int64(len(data)) > d.remaining {
		return nil, fmt.Errorf("bundle decoding byte budget exceeded")
	}
	d.remaining -= int64(len(data))
	d.consumed[ref.Path] = true
	if hash(data) != ref.SHA256 {
		return nil, fmt.Errorf("changed bundle file %q", ref.Path)
	}
	return data, nil
}
