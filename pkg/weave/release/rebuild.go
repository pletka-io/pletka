package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/rebuildlease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/txscope"
)

// LegacyRebuildPlan names the explicit preservation contract, not ordinary publication.
type LegacyRebuildPlan RebuildPlan

// LegacyRebuildRequest cannot be passed to the ordinary rebuild entry point.
type LegacyRebuildRequest struct {
	Plan        LegacyRebuildPlan `json:"plan"`
	Publication json.RawMessage   `json:"publication"`
}

type rebuildContract struct {
	format         int
	contentVersion int16
	source         string
	// capture cuts or preserves one release inside the rebuild transaction.
	capture func(ctx context.Context, tx pgx.Tx, p RebuildProject, actorID, operation string) error
}

func (c rebuildContract) preserves() bool { return c.source == "preserved-archive" }

func (s *Service) verifiedRebuild() rebuildContract {
	return rebuildContract{format: 1, contentVersion: 1, source: "current-drafts", capture: func(ctx context.Context, tx pgx.Tx, p RebuildProject, actorID, _ string) error {
		_, err := s.createInTx(ctx, tx, p.ProjectID, CreateInput{Version: p.Version, Title: p.Title, Description: p.Description}, actorID)
		return err
	}}
}

// legacyRebuild preserves the archived rows of each selected release and
// rebuilds only its evidence. It never reads the working draft.
func (s *Service) legacyRebuild() rebuildContract {
	return rebuildContract{format: 2, contentVersion: 2, source: "preserved-archive", capture: s.preserveLegacyInTx}
}

// PlanLegacyRebuild reviews current drafts for immutable legacy preservation.
// This does not enable an ordinary release request to bypass semantic validation.
func (s *Service) PlanLegacyRebuild(ctx context.Context, projectIDs []string) (LegacyRebuildPlan, error) {
	plan, err := s.planRebuildSession(ctx, projectIDs, s.legacyRebuild())
	return LegacyRebuildPlan(plan), err
}

// LegacyRebuildID hashes the exact legacy plan and publication inventory.
func LegacyRebuildID(request LegacyRebuildRequest) (string, error) {
	return RebuildID(legacyRequest(request))
}
func legacyRequest(request LegacyRebuildRequest) RebuildRequest {
	return RebuildRequest{Plan: RebuildPlan(request.Plan), Publication: request.Publication}
}

// RunLegacyRebuild requires a publisher that preserves legacy snapshot evidence.
// Git hosts use PrepareLegacyReleaseRepoint, which checks contract 2 and writes
// the exact captured ontology evidence through the portable schema 2 format.
func (s *Service) RunLegacyRebuild(ctx context.Context, request LegacyRebuildRequest, check func(context.Context, string, bool) error, publish func(context.Context, RebuildOperation) (map[string]string, error)) (RebuildOperation, error) {
	return s.runRebuild(ctx, legacyRequest(request), check, publish, s.legacyRebuild())
}

// RebuildProject names one release. Format 1 replaces its content from the
// draft; format 2 preserves its archive, whose digest the apply re-checks.
type RebuildProject struct {
	ProjectID     string    `json:"project_id"`
	Version       string    `json:"version"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedByID   string    `json:"created_by_id"`
	ArchiveDigest string    `json:"archive_digest,omitempty"`
}

// RebuildPin explicitly converts a draft-following inheritance edge to a release.
type RebuildPin struct {
	ProjectID       string `json:"project_id"`
	ParentProjectID string `json:"parent_project_id"`
	Version         string `json:"version"`
}

// RebuildPlan is a reviewable, fingerprinted plan over current drafts (format 1) or the retained archives (format 2).
type RebuildPlan struct {
	Format       int              `json:"format"`
	Source       string           `json:"source"`
	ActorID      string           `json:"actor_id"`
	ModuleHost   string           `json:"module_host"`
	OntologyHost string           `json:"ontology_host"`
	Fingerprint  string           `json:"fingerprint"`
	Projects     []RebuildProject `json:"projects"`     // source-first capture order
	DeleteOrder  []string         `json:"delete_order"` // consumer-first retained dependency order
	Pins         []RebuildPin     `json:"inheritance_pins"`
}

// RebuildRequest keeps the complete publication inventory in the durable journal.
type RebuildRequest struct {
	Plan        RebuildPlan     `json:"plan"`
	Publication json.RawMessage `json:"publication"`
}

// RebuiltRelease records the evidence and outbox entry produced by a real cut.
type RebuiltRelease struct {
	ProjectID   string          `json:"project_id"`
	Version     string          `json:"version"`
	Receipt     json.RawMessage `json:"receipt"`
	ChangeSetID int64           `json:"change_set_id"`
}

// RebuildOperation survives interruption between database replacement and Git publication.
type RebuildOperation struct {
	ID       string           `json:"id"`
	Request  RebuildRequest   `json:"request"`
	Releases []RebuiltRelease `json:"releases"`
	Complete bool             `json:"complete"`
}

func (s *Service) requireRebuild(ctx context.Context) (string, error) {
	p := auth.PrincipalFromContext(ctx)
	if !auth.FromContext(ctx).IsSuperAdmin || p == nil || p.ActorID == "" {
		return "", errors.New("release rebuilding requires a super-admin principal")
	}
	return p.ActorID, nil
}

// PlanRebuild is read-only. Empty projectIDs selects all legacy releases; every
// selected project must have exactly one active, unverified release.
func (s *Service) PlanRebuild(ctx context.Context, projectIDs []string) (RebuildPlan, error) {
	return s.planRebuildSession(ctx, projectIDs, s.verifiedRebuild())
}

func (s *Service) planRebuildSession(ctx context.Context, projectIDs []string, contract rebuildContract) (RebuildPlan, error) {
	actor, err := s.requireRebuild(ctx)
	if err != nil {
		return RebuildPlan{}, err
	}
	tx, err := txscope.New(s.pool).BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return RebuildPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return s.planRebuild(ctx, sqlcgen.New(tx), projectIDs, actor, contract)
}

func (s *Service) planRebuild(ctx context.Context, q *sqlcgen.Queries, projectIDs []string, actor string, contract rebuildContract) (RebuildPlan, error) {
	plan := RebuildPlan{Format: contract.format, Source: contract.source, ActorID: actor, ModuleHost: s.hosts.Project, OntologyHost: s.hosts.Ontology, Pins: []RebuildPin{}}
	catalog, err := q.WeaveRebuildCatalog(ctx)
	if err != nil {
		return plan, err
	}
	byProject := map[string][]sqlcgen.WeaveRebuildCatalogRow{}
	for _, r := range catalog {
		byProject[r.ProjectID] = append(byProject[r.ProjectID], r)
	}
	ids, selected, projects, err := selectRebuildProjects(byProject, projectIDs)
	if err != nil {
		return plan, err
	}
	if contract.preserves() {
		if err := preserveDigests(ctx, q, ids, projects); err != nil {
			return plan, err
		}
	}
	oldEdges := map[string][]string{}
	retained, err := q.WeaveRebuildRetainedSources(ctx)
	if err != nil {
		return plan, err
	}
	for _, r := range retained {
		if selected[r.SourceProjectID] && !selected[r.ProjectID] {
			return plan, fmt.Errorf("unselected consumer %s retains selected source %s", r.ProjectID, r.SourceProjectID)
		}
		if selected[r.ProjectID] && selected[r.SourceProjectID] {
			oldEdges[r.ProjectID] = append(oldEdges[r.ProjectID], r.SourceProjectID)
		}
	}
	inheritance, err := q.WeaveRebuildInheritance(ctx, ids)
	if err != nil {
		return plan, err
	}
	for _, r := range inheritance {
		rows := byProject[r.ParentProjectID]
		if len(rows) != 1 {
			return plan, fmt.Errorf("inheritance source %s needs exactly one release", r.ParentProjectID)
		}
		plan.Pins = append(plan.Pins, RebuildPin{r.ProjectID, r.ParentProjectID, rows[0].Version})
	}
	refsFor, ownerOf, err := rebuildInputs(ctx, q, contract, ids, projects)
	if err != nil {
		return plan, err
	}
	edges, err := rebuildSourceEdges(ctx, refsFor, ownerOf, ids, byProject, selected, inheritance, contract.contentVersion)
	if err != nil {
		return plan, err
	}
	order, err := rebuildOrder(ids, edges)
	if err != nil {
		return plan, err
	}
	oldOrder, err := rebuildOrder(ids, oldEdges)
	if err != nil {
		return plan, fmt.Errorf("retained graph: %w", err)
	}
	slices.Reverse(oldOrder)
	plan.DeleteOrder = oldOrder
	for _, id := range order {
		plan.Projects = append(plan.Projects, projects[id])
	}
	plan.Fingerprint, err = q.WeaveRebuildFingerprint(ctx, ids)
	return plan, err
}

// preserveDigests checks that each selected release has exactly one archived
// project row and records the digest of its archive for the run to verify.
func preserveDigests(ctx context.Context, q *sqlcgen.Queries, ids []string, projects map[string]RebuildProject) error {
	for _, id := range ids {
		count, err := q.WeavePreservedArchiveState(ctx, sqlcgen.WeavePreservedArchiveStateParams{ProjectID: id, Version: projects[id].Version})
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("project %s has no archived rows for release %s; nothing to preserve", id, projects[id].Version)
		}
		digest, err := q.WeaveLegacyArchiveDigest(ctx, sqlcgen.WeaveLegacyArchiveDigestParams{ProjectID: id, Version: projects[id].Version})
		if err != nil {
			return err
		}
		p := projects[id]
		p.ArchiveDigest = digest
		projects[id] = p
	}
	return nil
}

func selectRebuildProjects(byProject map[string][]sqlcgen.WeaveRebuildCatalogRow, projectIDs []string) ([]string, map[string]bool, map[string]RebuildProject, error) {
	ids := slices.Clone(projectIDs)
	if len(ids) == 0 {
		for id, rows := range byProject {
			for _, r := range rows {
				if r.ContentCaptureVersion == 0 {
					ids = append(ids, id)
					break
				}
			}
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil, nil, nil, errors.New("no legacy releases selected")
	}
	selected := map[string]bool{}
	projects := map[string]RebuildProject{}
	for _, id := range ids {
		rows := byProject[id]
		if len(rows) != 1 {
			return nil, nil, nil, fmt.Errorf("project %s has %d releases; expected exactly one", id, len(rows))
		}
		r := rows[0]
		if r.ContentCaptureVersion != 0 || r.ArchivedAt.Valid || r.OntologySnapshotCaptured {
			return nil, nil, nil, fmt.Errorf("project %s release must be active, unverified and have no retained ontology snapshot", id)
		}
		if errs := validateCreate(CreateInput{Version: r.Version}); len(errs) > 0 {
			return nil, nil, nil, fmt.Errorf("project %s has unsupported release version %q", id, r.Version)
		}
		selected[id] = true
		projects[id] = RebuildProject{ProjectID: id, Version: r.Version, Title: r.Title, Description: r.Description, CreatedAt: r.CreatedAt.UTC(), CreatedByID: r.CreatedByID}
	}
	return ids, selected, projects, nil
}

type (
	rebuildRefs  func(ctx context.Context, id string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error)
	rebuildOwner func(ctx context.Context, kind, id string) (string, error)
)

// rebuildInputs chooses where the capture order and source checks read
// references: the working drafts for format 1, each release's archive for
// preserve mode, which captures from the archive. Preserve refuses archived
// inheritance that still follows a parent draft, for every project at once.
func rebuildInputs(ctx context.Context, q *sqlcgen.Queries, contract rebuildContract, ids []string, projects map[string]RebuildProject) (rebuildRefs, rebuildOwner, error) {
	liveOwner := func(ctx context.Context, kind, id string) (string, error) {
		return q.WeaveDraftDependencyEntityOwner(ctx, sqlcgen.WeaveDraftDependencyEntityOwnerParams{EntityType: kind, EntityID: id})
	}
	if !contract.preserves() {
		return q.WeaveDraftDependencyReferences, liveOwner, nil
	}
	following := []string{}
	for _, id := range ids {
		parents, err := q.WeaveArchivedFollowingDependencies(ctx, sqlcgen.WeaveArchivedFollowingDependenciesParams{ProjectID: id, Version: projects[id].Version})
		if err != nil {
			return nil, nil, err
		}
		for _, parent := range parents {
			following = append(following, id+"@"+projects[id].Version+" -> "+parent)
		}
	}
	if len(following) > 0 {
		return nil, nil, fmt.Errorf("archived inheritance still follows the parent draft; repair the archive before preserving: %s", strings.Join(following, ", "))
	}
	refs := func(ctx context.Context, id string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error) {
		rows, err := q.WeaveArchivedDependencyReferences(ctx, sqlcgen.WeaveArchivedDependencyReferencesParams{ProjectID: id, Version: projects[id].Version})
		if err != nil {
			return nil, err
		}
		out := make([]sqlcgen.WeaveDraftDependencyReferencesRow, len(rows))
		for i, row := range rows {
			out[i] = sqlcgen.WeaveDraftDependencyReferencesRow(row)
		}
		return out, nil
	}
	// Same owner chain as releasedeps.archivedDraft: live rows, then archived rows.
	owner := func(ctx context.Context, kind, id string) (string, error) {
		source, err := liveOwner(ctx, kind, id)
		if !errors.Is(err, pgx.ErrNoRows) {
			return source, err
		}
		return q.WeaveArchivedDependencyEntityOwner(ctx, sqlcgen.WeaveArchivedDependencyEntityOwnerParams{EntityType: kind, EntityID: id})
	}
	return refs, owner, nil
}

func rebuildSourceEdges(ctx context.Context, refsFor rebuildRefs, ownerOf rebuildOwner, ids []string, byProject map[string][]sqlcgen.WeaveRebuildCatalogRow, selected map[string]bool, inheritance []sqlcgen.WeaveRebuildInheritanceRow, contentVersion int16) (map[string][]string, error) {
	edges := map[string][]string{}
	for _, id := range ids {
		if parent := byProject[id][0].LegacyParent; parent != "" && !slices.ContainsFunc(inheritance, func(r sqlcgen.WeaveRebuildInheritanceRow) bool {
			return r.ProjectID == id && r.ParentProjectID == parent
		}) {
			return nil, fmt.Errorf("project %s has implicit parent %s; declare its inheritance edge before rebuilding", id, parent)
		}
		refs, err := refsFor(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			source := ref.SourceProjectID
			if source == "" {
				source, err = ownerOf(ctx, ref.EntityType, ref.EntityID)
				if err != nil {
					return nil, fmt.Errorf("project %s: unresolved %s %s in current drafts: %w", id, ref.EntityType, ref.EntityID, err)
				}
			}
			if source == id {
				continue
			}
			rows := byProject[source]
			if len(rows) != 1 {
				return nil, fmt.Errorf("project %s source %s needs exactly one release", id, source)
			}
			r := rows[0]
			if ref.SourceVersion != "" && ref.SourceVersion != r.Version {
				return nil, fmt.Errorf("project %s pins %s@%s, but its sole release is %s", id, source, ref.SourceVersion, r.Version)
			}
			if !selected[source] && r.ContentCaptureVersion != contentVersion {
				return nil, fmt.Errorf("include source project %s required by %s with matching capture evidence", source, id)
			}
			if r.ArchivedAt.Valid {
				return nil, fmt.Errorf("source %s is retired", source)
			}
			if selected[source] {
				edges[id] = append(edges[id], source)
			}
		}
	}
	return edges, nil
}

func rebuildOrder(ids []string, edges map[string][]string) ([]string, error) {
	state := map[string]int{}
	order := []string{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("release dependency cycle at %s", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		next := slices.Clone(edges[id])
		slices.Sort(next)
		for _, source := range slices.Compact(next) {
			if err := visit(source); err != nil {
				return err
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// RebuildID identifies the exact reviewed plan plus expected publication refs.
func RebuildID(request RebuildRequest) (string, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// GetRebuild reads an operation for status/recovery without mutating it.
func (s *Service) GetRebuild(ctx context.Context, id string) (RebuildOperation, error) {
	if _, err := s.requireRebuild(ctx); err != nil {
		return RebuildOperation{}, err
	}
	row, err := sqlcgen.New(txscope.New(s.pool)).WeaveGetRebuildOperation(ctx, id)
	if err != nil {
		return RebuildOperation{}, err
	}
	op := RebuildOperation{ID: id, Complete: row.CompletedAt.Valid}
	if err = json.Unmarshal([]byte(row.Document), &op.Request); err != nil {
		return op, err
	}
	if err = json.Unmarshal(row.Result, &op.Releases); err != nil {
		return op, err
	}
	return op, nil
}

// RunRebuild owns the exclusive maintenance lifetime. The database replacement
// commits atomically before publish runs; failures leave the durable runtime gate
// closed for resume. Only this path may replace legacy cuts; normal Create has
// no compatibility bypass. check must compare the reviewed Git refs before SQL.
func (s *Service) RunRebuild(ctx context.Context, request RebuildRequest, check func(context.Context, string, bool) error, publish func(context.Context, RebuildOperation) (map[string]string, error)) (RebuildOperation, error) {
	return s.runRebuild(ctx, request, check, publish, s.verifiedRebuild())
}

func (s *Service) runRebuild(ctx context.Context, request RebuildRequest, check func(context.Context, string, bool) error, publish func(context.Context, RebuildOperation) (map[string]string, error), contract rebuildContract) (RebuildOperation, error) {
	actor, err := s.requireRebuild(ctx)
	if err != nil {
		return RebuildOperation{}, err
	}
	if actor != request.Plan.ActorID || request.Plan.Format != contract.format || request.Plan.Source != contract.source || check == nil || publish == nil {
		return RebuildOperation{}, errors.New("invalid rebuild request, principal or publisher")
	}
	id, err := RebuildID(request)
	if err != nil {
		return RebuildOperation{}, err
	}
	unlock, err := rebuildlease.Rebuild(ctx, s.pool)
	if err != nil {
		return RebuildOperation{}, err
	}
	defer unlock()
	op, err := s.GetRebuild(ctx, id)
	resume := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return op, err
	}
	if resume && op.Complete {
		return op, nil
	}
	if err := check(ctx, id, resume); err != nil {
		return op, err
	}
	if !resume {
		op, err = s.captureRebuild(ctx, request, id, contract)
		if err != nil {
			return op, err
		}
	}
	for _, r := range op.Releases {
		valid, err := sqlcgen.New(txscope.New(s.pool)).WeaveVerifyRebuiltRelease(ctx, sqlcgen.WeaveVerifyRebuiltReleaseParams{ProjectID: r.ProjectID, Version: r.Version, Receipt: r.Receipt, ChangeSetID: r.ChangeSetID})
		if err != nil {
			return op, err
		}
		if !valid {
			return op, fmt.Errorf("recorded capture changed for %s@%s; refusing publication", r.ProjectID, r.Version)
		}
	}
	commits, err := publish(ctx, op)
	if err != nil {
		return op, fmt.Errorf("rebuild %s captured; publication incomplete, resume this operation: %w", id, err)
	}
	return s.completeRebuild(ctx, op, commits)
}

func (s *Service) completeRebuild(ctx context.Context, op RebuildOperation, commits map[string]string) (RebuildOperation, error) {
	if len(commits) != len(op.Releases) {
		return op, errors.New("publisher omitted rebuilt releases")
	}
	tx, err := txscope.New(s.pool).Begin(ctx)
	if err != nil {
		return op, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlcgen.New(tx)
	for _, r := range op.Releases {
		sha := commits[r.ProjectID]
		if len(sha) != 40 && len(sha) != 64 {
			return op, errors.New("publisher returned invalid commit identity")
		}
		if _, err := hex.DecodeString(sha); err != nil {
			return op, err
		}
		if err := q.WeaveCompleteRebuildOutbox(ctx, sqlcgen.WeaveCompleteRebuildOutboxParams{ID: r.ChangeSetID, CommitSha: sha}); err != nil {
			return op, err
		}
	}
	if err := q.WeaveCompleteRebuildOperation(ctx, op.ID); err != nil {
		return op, err
	}
	if err := tx.Commit(ctx); err != nil {
		return op, err
	}
	op.Complete = true
	return op, nil
}

func (s *Service) captureRebuild(ctx context.Context, request RebuildRequest, id string, contract rebuildContract) (RebuildOperation, error) {
	op := RebuildOperation{ID: id, Request: request}
	// LOCK TABLE is the first command and does not take an MVCC snapshot.
	// Validation therefore sees writers that committed while locks waited;
	// repeatable read then satisfies the normal capture contract.
	tx, err := txscope.New(s.pool).BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return op, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlcgen.New(tx)
	if err := q.WeaveLockRebuildInputs(ctx); err != nil {
		return op, err
	}
	ids := []string{}
	byID := map[string]RebuildProject{}
	for _, p := range request.Plan.Projects {
		ids = append(ids, p.ProjectID)
		byID[p.ProjectID] = p
	}
	current, err := s.planRebuild(ctx, q, ids, request.Plan.ActorID, contract)
	if err != nil {
		return op, err
	}
	want, _ := json.Marshal(request.Plan)
	actual, _ := json.Marshal(current)
	if string(want) != string(actual) {
		return op, errors.New("rebuild inputs or configuration changed; generate and review a new plan")
	}
	if pending, err := q.WeaveRebuildPending(ctx); err != nil || pending {
		if err != nil {
			return op, err
		}
		return op, errors.New("another rebuild is unfinished")
	}
	if err := replaceRebuildInputs(ctx, q, request, byID, ids, contract); err != nil {
		return op, err
	}
	for _, p := range request.Plan.Projects {
		if err := contract.capture(ctx, tx, p, request.Plan.ActorID, id); err != nil {
			return op, fmt.Errorf("rebuild %s@%s: %w", p.ProjectID, p.Version, err)
		}
		captured, err := q.WeaveRebuildCapturedResult(ctx, sqlcgen.WeaveRebuildCapturedResultParams{ProjectID: p.ProjectID, Version: p.Version})
		if err != nil {
			return op, err
		}
		op.Releases = append(op.Releases, RebuiltRelease{p.ProjectID, p.Version, captured.ContentCaptureReceipt, captured.ChangeSetID})
	}
	document, _ := json.Marshal(request)
	result, _ := json.Marshal(op.Releases)
	if err := q.WeaveInsertRebuildOperation(ctx, sqlcgen.WeaveInsertRebuildOperationParams{ID: id, Document: string(document), Result: result}); err != nil {
		return op, err
	}
	if err := tx.Commit(ctx); err != nil {
		return op, err
	}
	return op, nil
}

// replaceRebuildInputs drops the selected releases in retained-graph order,
// clears their archives unless the contract preserves them, re-pins
// inheritance and supersedes pending outbox work, ready for capture.
func replaceRebuildInputs(ctx context.Context, q *sqlcgen.Queries, request RebuildRequest, byID map[string]RebuildProject, ids []string, contract rebuildContract) error {
	for _, project := range request.Plan.DeleteOrder {
		p := byID[project]
		if err := q.WeaveRebuildDeleteRelease(ctx, sqlcgen.WeaveRebuildDeleteReleaseParams{ProjectID: project, Version: p.Version}); err != nil {
			return err
		}
	}
	// Preserve mode keeps the archive rows; FK cascades on the release row drop the old evidence.
	if !contract.preserves() {
		for _, p := range request.Plan.Projects {
			if err := q.WeaveRebuildClearArchive(ctx, sqlcgen.WeaveRebuildClearArchiveParams{ProjectID: p.ProjectID, Version: p.Version}); err != nil {
				return err
			}
		}
	}
	if err := pinRebuildInheritance(ctx, q, request.Plan.Pins); err != nil {
		return err
	}
	return q.WeaveRebuildSupersedeOutbox(ctx, ids)
}

func pinRebuildInheritance(ctx context.Context, q *sqlcgen.Queries, pins []RebuildPin) error {
	for _, pin := range pins {
		if err := q.WeaveRebuildPinInheritance(ctx, sqlcgen.WeaveRebuildPinInheritanceParams{ProjectID: pin.ProjectID, ParentProjectID: pin.ParentProjectID, Version: pin.Version}); err != nil {
			return err
		}
	}
	return nil
}
