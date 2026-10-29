package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/rebuildlease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
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
	create         func(context.Context, pgx.Tx, string, CreateInput, string) (*Release, error)
}

func (s *Service) verifiedRebuild() rebuildContract { return rebuildContract{1, 1, s.createInTx} }
func (s *Service) legacyRebuild() rebuildContract   { return rebuildContract{2, 2, s.createLegacyInTx} }

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

// RebuildProject preserves a legacy release name while replacing its content.
type RebuildProject struct {
	ProjectID   string `json:"project_id"`
	Version     string `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// RebuildPin explicitly converts a draft-following inheritance edge to a release.
type RebuildPin struct {
	ProjectID       string `json:"project_id"`
	ParentProjectID string `json:"parent_project_id"`
	Version         string `json:"version"`
}

// RebuildPlan is a reviewable, fingerprinted plan over current working content.
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
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return RebuildPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return s.planRebuild(ctx, sqlcgen.New(tx), projectIDs, actor, contract)
}

func (s *Service) planRebuild(ctx context.Context, q *sqlcgen.Queries, projectIDs []string, actor string, contract rebuildContract) (RebuildPlan, error) {
	plan := RebuildPlan{Format: contract.format, Source: "current-drafts", ActorID: actor, ModuleHost: s.hosts.Project, OntologyHost: s.hosts.Ontology, Pins: []RebuildPin{}}
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
	edges, err := rebuildSourceEdges(ctx, q, ids, byProject, selected, inheritance, contract.contentVersion)
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
		projects[id] = RebuildProject{id, r.Version, r.Title, r.Description}
	}
	return ids, selected, projects, nil
}

func rebuildSourceEdges(ctx context.Context, q *sqlcgen.Queries, ids []string, byProject map[string][]sqlcgen.WeaveRebuildCatalogRow, selected map[string]bool, inheritance []sqlcgen.WeaveRebuildInheritanceRow, contentVersion int16) (map[string][]string, error) {
	edges := map[string][]string{}
	for _, id := range ids {
		if parent := byProject[id][0].LegacyParent; parent != "" && !slices.ContainsFunc(inheritance, func(r sqlcgen.WeaveRebuildInheritanceRow) bool {
			return r.ProjectID == id && r.ParentProjectID == parent
		}) {
			return nil, fmt.Errorf("project %s has implicit parent %s; declare its inheritance edge before rebuilding", id, parent)
		}
		refs, err := q.WeaveDraftDependencyReferences(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			source := ref.SourceProjectID
			if source == "" {
				source, err = q.WeaveDraftDependencyEntityOwner(ctx, sqlcgen.WeaveDraftDependencyEntityOwnerParams{EntityType: ref.EntityType, EntityID: ref.EntityID})
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
	row, err := sqlcgen.New(s.pool).WeaveGetRebuildOperation(ctx, id)
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
	if actor != request.Plan.ActorID || request.Plan.Format != contract.format || request.Plan.Source != "current-drafts" || check == nil || publish == nil {
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
		valid, err := sqlcgen.New(s.pool).WeaveVerifyRebuiltRelease(ctx, sqlcgen.WeaveVerifyRebuiltReleaseParams{ProjectID: r.ProjectID, Version: r.Version, Receipt: r.Receipt, ChangeSetID: r.ChangeSetID})
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
	tx, err := s.pool.Begin(ctx)
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
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
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
	for _, project := range request.Plan.DeleteOrder {
		p := byID[project]
		if err := q.WeaveRebuildDeleteRelease(ctx, sqlcgen.WeaveRebuildDeleteReleaseParams{ProjectID: project, Version: p.Version}); err != nil {
			return op, err
		}
	}
	for _, p := range request.Plan.Projects {
		if err := q.WeaveRebuildClearArchive(ctx, sqlcgen.WeaveRebuildClearArchiveParams{ProjectID: p.ProjectID, Version: p.Version}); err != nil {
			return op, err
		}
	}
	if err := pinRebuildInheritance(ctx, q, request.Plan.Pins); err != nil {
		return op, err
	}
	if err := q.WeaveRebuildSupersedeOutbox(ctx, ids); err != nil {
		return op, err
	}
	for _, p := range request.Plan.Projects {
		if _, err := contract.create(ctx, tx, p.ProjectID, CreateInput{Version: p.Version, Title: p.Title, Description: p.Description}, request.Plan.ActorID); err != nil {
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

func pinRebuildInheritance(ctx context.Context, q *sqlcgen.Queries, pins []RebuildPin) error {
	for _, pin := range pins {
		if err := q.WeaveRebuildPinInheritance(ctx, sqlcgen.WeaveRebuildPinInheritanceParams{ProjectID: pin.ProjectID, ParentProjectID: pin.ParentProjectID, Version: pin.Version}); err != nil {
			return err
		}
	}
	return nil
}
