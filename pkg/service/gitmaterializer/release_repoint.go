package gitmaterializer

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
)

// ReleaseRefState records exact Git objects, including an annotated tag object.
type ReleaseRefState struct {
	ProjectID string `json:"project_id"`
	Version   string `json:"version"`
	Branch    string `json:"branch"`
	Tag       string `json:"tag"`
}

// PreparedRelease keeps replacement objects separate from public refs.
type PreparedRelease struct {
	Expected ReleaseRefState `json:"expected"`
	Commit   string          `json:"commit"`
	Tag      string          `json:"tag"`
}

var rebuildProjectID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var rebuildVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

func (m *Materializer) rebuildPath(project, version string) (string, error) {
	if !rebuildProjectID.MatchString(project) || !rebuildVersion.MatchString(version) {
		return "", fmt.Errorf("unsafe project/version for release repoint: %q@%q", project, version)
	}
	root, err := filepath.Abs(m.baseDir)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, project)
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		return "", fmt.Errorf("project repository must not be a symlink: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if info, err := os.Lstat(filepath.Join(path, ".git")); err == nil && !info.IsDir() {
		return "", fmt.Errorf("repoint requires a dedicated project repository: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func rebuildPrefix(operation string) (string, error) {
	if len(operation) != 64 {
		return "", fmt.Errorf("invalid rebuild operation identity")
	}
	if _, err := hex.DecodeString(operation); err != nil {
		return "", err
	}
	return "refs/pletka-rebuild/" + operation + "/", nil
}

// InspectReleaseRefs is read-only and never initializes a missing repository.
func (m *Materializer) InspectReleaseRefs(ctx context.Context, project, version string) (ReleaseRefState, error) {
	state := ReleaseRefState{ProjectID: project, Version: version}
	path, err := m.rebuildPath(project, version)
	if err != nil {
		return state, err
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		return state, nil
	} else if err != nil {
		return state, err
	}
	git := newGitRunner(path, m.logger)
	state.Branch, _, err = git.RefSHA(ctx, releasesBranchRef)
	if err != nil {
		return state, err
	}
	state.Tag, _, err = git.RefSHA(ctx, "refs/tags/v"+version)
	return state, err
}

// CheckReleaseRepoint rejects drift from the reviewed refs. During recovery the
// atomically published prepared pair is accepted as well as the original pair.
func (m *Materializer) CheckReleaseRepoint(ctx context.Context, operation string, expected ReleaseRefState, resume bool) error {
	if _, err := rebuildPrefix(operation); err != nil {
		return err
	}
	for _, object := range []string{expected.Branch, expected.Tag} {
		if object == "" {
			continue
		}
		if len(object) != 40 && len(object) != 64 {
			return fmt.Errorf("invalid expected Git object")
		}
		if _, err := hex.DecodeString(object); err != nil {
			return fmt.Errorf("invalid expected Git object: %w", err)
		}
	}
	actual, err := m.InspectReleaseRefs(ctx, expected.ProjectID, expected.Version)
	if err != nil {
		return err
	}
	if actual == expected {
		return nil
	}
	if resume {
		prefix, err := rebuildPrefix(operation)
		if err != nil {
			return err
		}
		path, err := m.rebuildPath(expected.ProjectID, expected.Version)
		if err != nil {
			return err
		}
		git := newGitRunner(path, m.logger)
		commit, _, err := git.RefSHA(ctx, prefix+"prepared-commit")
		if err != nil {
			return err
		}
		tag, _, err := git.RefSHA(ctx, prefix+"prepared-tag")
		if err != nil {
			return err
		}
		if commit != "" && tag != "" && actual.Branch == commit && actual.Tag == tag {
			return nil
		}
	}
	return fmt.Errorf("git refs changed for %s@%s; refusing to overwrite unreviewed refs", expected.ProjectID, expected.Version)
}

// PrepareReleaseRepoint writes replacement objects and recoverable backup refs.
// All projects should be prepared before any public ref is moved. This uses the
// complete format for new objects; resumed objects keep their stored format.
func (m *Materializer) PrepareReleaseRepoint(ctx context.Context, operation string, expected ReleaseRefState) (PreparedRelease, error) {
	return m.prepareReleaseRepoint(ctx, operation, expected, 1)
}

// PrepareLegacyReleaseRepoint prepares a contract-2 cut with exact legacy
// ontology evidence. It shares ref checks and recovery with verified repoints.
func (m *Materializer) PrepareLegacyReleaseRepoint(ctx context.Context, operation string, expected ReleaseRefState) (PreparedRelease, error) {
	return m.prepareReleaseRepoint(ctx, operation, expected, 2)
}

func (m *Materializer) prepareReleaseRepoint(ctx context.Context, operation string, expected ReleaseRefState, contract int16) (PreparedRelease, error) {
	prepared := PreparedRelease{Expected: expected}
	prefix, err := rebuildPrefix(operation)
	if err != nil {
		return prepared, err
	}
	path, err := m.rebuildPath(expected.ProjectID, expected.Version)
	if err != nil {
		return prepared, err
	}
	unlock, err := acquireProjectLock(m.baseDir, expected.ProjectID)
	if err != nil {
		return prepared, err
	}
	defer unlock()
	if err := m.CheckReleaseRepoint(ctx, operation, expected, true); err != nil {
		return prepared, err
	}
	if err := readsession.NewFactory(m.pool).Read(ctx, auth.Release(expected.Version), expected.ProjectID, func(s *readsession.Session) error {
		return requireRepointContent(ctx, s, contract)
	}); err != nil {
		return prepared, err
	}
	git := newGitRunner(path, m.logger)
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		if err := git.Init(ctx); err != nil {
			return prepared, err
		}
	}
	tmp, err := os.MkdirTemp("", "pletka-repoint-")
	if err != nil {
		return prepared, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	prepared.Commit, _, err = git.RefSHA(ctx, prefix+"prepared-commit")
	if err != nil {
		return prepared, err
	}
	prepared.Tag, _, err = git.RefSHA(ctx, prefix+"prepared-tag")
	if err != nil {
		return prepared, err
	}
	if prepared.Commit != "" || prepared.Tag != "" {
		if prepared.Commit == "" || prepared.Tag == "" {
			return prepared, fmt.Errorf("incomplete prepared release for %s", expected.ProjectID)
		}
		// Resume the exact stored format and catalog, even after a writer upgrade
		// or mutable title/retirement edit. Never reinterpret prepared bytes.
		if err := m.writeVerificationTree(ctx, git, prepared.Commit, tmp, expected.ProjectID, expected.Version); err != nil {
			return prepared, err
		}
		tree, err := git.WriteTreeFrom(ctx, tmp)
		if err != nil {
			return prepared, err
		}
		actualTree, _, err := git.TreeOfRev(ctx, prepared.Commit)
		if err != nil {
			return prepared, err
		}
		target, _, err := git.RefSHA(ctx, prepared.Tag+"^{}")
		if err != nil {
			return prepared, err
		}
		if actualTree != tree || target != prepared.Commit || prepared.Tag == "" {
			return prepared, fmt.Errorf("prepared content differs for %s", expected.ProjectID)
		}
		return prepared, nil
	}
	if err := m.buildReleaseTree(ctx, tmp, expected.ProjectID, expected.Version); err != nil {
		return prepared, err
	}
	tree, err := git.WriteTreeFrom(ctx, tmp)
	if err != nil {
		return prepared, err
	}
	return prepareRepointObjects(ctx, git, operation, prefix, tree, expected)
}

func prepareRepointObjects(ctx context.Context, git *gitRunner, operation, prefix, tree string, expected ReleaseRefState) (PreparedRelease, error) {
	prepared := PreparedRelease{Expected: expected}
	var err error
	prepared.Commit, err = git.CommitTree(ctx, tree, expected.Branch, "Rebuild release v"+expected.Version+"\n\nRebuild-Operation: "+operation, releaseSystemName, releaseSystemEmail)
	if err != nil {
		return prepared, err
	}
	// A tag object is prepared without creating/replacing its public tag ref.
	tagBody := fmt.Sprintf("object %s\ntype commit\ntag v%s\ntagger %s <%s> 1 +0000\n\nRebuilt release v%s\nOperation: %s\n", prepared.Commit, expected.Version, releaseSystemName, releaseSystemEmail, expected.Version, operation)
	prepared.Tag, err = git.runOutStdin(ctx, nil, []byte(tagBody), "mktag")
	if err != nil {
		return prepared, err
	}
	zero := strings.Repeat("0", len(prepared.Commit))
	branch, tag := expected.Branch, expected.Tag
	if branch == "" {
		branch = zero
	}
	if tag == "" {
		tag = zero
	}
	commands := "start\nverify " + releasesBranchRef + " " + branch + "\nverify refs/tags/v" + expected.Version + " " + tag + "\ncreate " + prefix + "prepared-commit " + prepared.Commit + "\ncreate " + prefix + "prepared-tag " + prepared.Tag + "\n"
	if expected.Branch != "" {
		commands += "create " + prefix + "previous-branch " + expected.Branch + "\n"
	}
	if expected.Tag != "" {
		commands += "create " + prefix + "previous-tag " + expected.Tag + "\n"
	}
	_, err = git.runOutStdin(ctx, nil, []byte(commands+"prepare\ncommit\n"), "update-ref", "--stdin")
	return prepared, err
}

// PublishReleaseRepoint moves the branch/tag together with compare-and-swap.
// A retry after publication is a no-op; unrelated ref movement is an error.
func (m *Materializer) PublishReleaseRepoint(ctx context.Context, operation string, prepared PreparedRelease) error {
	expected := prepared.Expected
	prefix, err := rebuildPrefix(operation)
	if err != nil {
		return err
	}
	path, err := m.rebuildPath(expected.ProjectID, expected.Version)
	if err != nil {
		return err
	}
	unlock, err := acquireProjectLock(m.baseDir, expected.ProjectID)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.CheckReleaseRepoint(ctx, operation, expected, true); err != nil {
		return err
	}
	git := newGitRunner(path, m.logger)
	for ref, want := range map[string]string{prefix + "prepared-tag": prepared.Tag, prefix + "prepared-commit": prepared.Commit} {
		actual, ok, err := git.RefSHA(ctx, ref)
		if err != nil {
			return err
		}
		if !ok || actual != want {
			return fmt.Errorf("unrecorded prepared object %s", ref)
		}
	}
	actual, err := m.InspectReleaseRefs(ctx, expected.ProjectID, expected.Version)
	if err != nil {
		return err
	}
	if actual.Branch == prepared.Commit && actual.Tag == prepared.Tag {
		return nil
	}
	zero := strings.Repeat("0", len(prepared.Commit))
	branch, tag := expected.Branch, expected.Tag
	if branch == "" {
		branch = zero
	}
	if tag == "" {
		tag = zero
	}
	commands := fmt.Sprintf("start\nupdate %s %s %s\nupdate refs/tags/v%s %s %s\nprepare\ncommit\n", releasesBranchRef, prepared.Commit, branch, expected.Version, prepared.Tag, tag)
	_, err = git.runOutStdin(ctx, nil, []byte(commands), "update-ref", "--stdin")
	return err
}

func requireRepointContent(ctx context.Context, s *readsession.Session, contract int16) error {
	if contract == 2 {
		_, err := s.LegacyContent(ctx)
		return err
	}
	_, err := s.Content(ctx)
	return err
}
