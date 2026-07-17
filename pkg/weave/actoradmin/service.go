package actoradmin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/ids"
	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordIncorrect is returned by ChangePassword when the supplied
// current password does not match the stored hash.
var ErrPasswordIncorrect = errors.New("current password is incorrect")

type Service struct {
	store Store
	auth  domain.AuthStore
	log   *slog.Logger
}

func NewService(store Store, auth domain.AuthStore, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store: store,
		auth:  auth,
		log:   log,
	}
}

type UserBrowseInput struct {
	Search  string
	Role    string
	SortBy  string
	Page    int
	PerPage int
}

type InstitutionBrowseInput struct {
	Search  string
	SortBy  string
	Page    int
	PerPage int
}

type BrowseResult[T any] struct {
	Items []T
	Total int
}

type UserBrowseItem struct {
	ID                string  `json:"id"`
	DisplayName       string  `json:"display_name"`
	Slug              string  `json:"slug"`
	Email             *string `json:"email,omitempty"`
	Role              string  `json:"role"`
	InstitutionID     *string `json:"institution_id,omitempty"`
	InstitutionName   string  `json:"institution_name,omitempty"`
	Website           *string `json:"website,omitempty"`
	Country           *string `json:"country,omitempty"`
	Orcid             *string `json:"orcid,omitempty"`
	MembershipCount   int     `json:"membership_count"`
	OwnedProjectCount int     `json:"owned_project_count"`
	LastLoginDisplay  string  `json:"last_login_display,omitempty"`
}

type InstitutionBrowseItem struct {
	ID                string  `json:"id"`
	DisplayName       string  `json:"display_name"`
	Slug              string  `json:"slug"`
	Acronym           *string `json:"acronym,omitempty"`
	Website           *string `json:"website,omitempty"`
	Country           *string `json:"country,omitempty"`
	Visibility        string  `json:"visibility"`
	OwnedProjectCount int     `json:"owned_project_count"`
	MemberCount       int     `json:"member_count"`
}

type UserEditInput struct {
	DisplayName string
	Slug        string
	Email       *string
	Role        string
	ParentID    *string
	FirstName   *string
	LastName    *string
	Country     *string
	Website     *string
	Orcid       *string
}

// UserCreateInput mirrors UserEditInput; kept as a separate type so the
// signature stays distinct and future create-only fields (e.g. an
// initial password / invite token) can land without touching edits.
type UserCreateInput struct {
	DisplayName string
	Slug        string
	Email       *string
	Role        string
	ParentID    *string
	FirstName   *string
	LastName    *string
	Country     *string
	Website     *string
	Orcid       *string
	// Password is the admin-set initial password. When nil/empty the
	// service generates one and returns it from CreateUser.
	Password    *string
}

type InstitutionEditInput struct {
	DisplayName string
	Slug        string
	Acronym     *string
	Country     *string
	Website     *string
	// Visibility is "public" or "private". Empty preserves the current row.
	Visibility string
}

type InstitutionCreateInput struct {
	DisplayName string
	Slug        string
	Acronym     *string
	Country     *string
	Website     *string
	// Visibility is "public" or "private". Empty falls back to "public" —
	// new institutions are discoverable by default, matching the import
	// behaviour. Curators can flip to private from the same form.
	Visibility string
}

func (s *Service) BrowseUsers(ctx context.Context, in UserBrowseInput) (*BrowseResult[UserBrowseItem], error) {
	actors, err := s.store.ListActorsByType(ctx, "person")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	institutions, err := s.store.ListActorsByType(ctx, "organization")
	if err != nil {
		return nil, fmt.Errorf("list institutions for users: %w", err)
	}
	instNames := make(map[string]string, len(institutions))
	for _, inst := range institutions {
		instNames[inst.ID] = inst.DisplayName
	}

	filtered := make([]sqlcgen.WeaveActor, 0, len(actors))
	search := strings.ToLower(strings.TrimSpace(in.Search))
	roleFilter := strings.TrimSpace(in.Role)
	for _, actor := range actors {
		if roleFilter != "" && actor.Role != roleFilter {
			continue
		}
		if search != "" && !matchesUserSearch(actor, search, instNames) {
			continue
		}
		filtered = append(filtered, actor)
	}

	sortUsers(filtered, instNames, in.SortBy)
	page, perPage := sanitizePage(in.Page, in.PerPage)
	window, total := paginateActors(filtered, page, perPage)
	idsPage := actorIDs(window)
	membershipCounts, err := s.store.ProjectCounts(ctx, idsPage)
	if err != nil {
		return nil, err
	}
	ownedCounts, err := s.store.OwnedProjectCounts(ctx, idsPage)
	if err != nil {
		return nil, err
	}
	lastLogins, err := s.store.LastLogins(ctx, idsPage)
	if err != nil {
		return nil, err
	}

	items := make([]UserBrowseItem, 0, len(window))
	for _, actor := range window {
		var institutionName string
		if actor.ParentID != nil {
			institutionName = instNames[*actor.ParentID]
		}
		items = append(items, UserBrowseItem{
			ID:                actor.ID,
			DisplayName:       actor.DisplayName,
			Slug:              actor.Slug,
			Email:             actor.Email,
			Role:              actor.Role,
			InstitutionID:     actor.ParentID,
			InstitutionName:   institutionName,
			Website:           actor.Website,
			Country:           actor.Country,
			Orcid:             actor.Orcid,
			MembershipCount:   membershipCounts[actor.ID],
			OwnedProjectCount: ownedCounts[actor.ID],
			LastLoginDisplay:  formatLastLogin(lastLogins[actor.ID]),
		})
	}

	return &BrowseResult[UserBrowseItem]{Items: items, Total: total}, nil
}

func (s *Service) BrowseInstitutions(ctx context.Context, in InstitutionBrowseInput) (*BrowseResult[InstitutionBrowseItem], error) {
	actors, err := s.store.ListActorsByType(ctx, "organization")
	if err != nil {
		return nil, fmt.Errorf("list institutions: %w", err)
	}

	filtered := make([]sqlcgen.WeaveActor, 0, len(actors))
	search := strings.ToLower(strings.TrimSpace(in.Search))
	for _, actor := range actors {
		if search != "" && !matchesInstitutionSearch(actor, search) {
			continue
		}
		filtered = append(filtered, actor)
	}

	sortInstitutions(filtered, in.SortBy)
	page, perPage := sanitizePage(in.Page, in.PerPage)
	window, total := paginateActors(filtered, page, perPage)
	idsPage := actorIDs(window)
	ownedCounts, err := s.store.OwnedProjectCounts(ctx, idsPage)
	if err != nil {
		return nil, err
	}
	memberCounts, err := s.store.MemberCounts(ctx, idsPage)
	if err != nil {
		return nil, err
	}

	items := make([]InstitutionBrowseItem, 0, len(window))
	for _, actor := range window {
		items = append(items, InstitutionBrowseItem{
			ID:                actor.ID,
			DisplayName:       actor.DisplayName,
			Slug:              actor.Slug,
			Acronym:           actor.Acronym,
			Website:           actor.Website,
			Country:           actor.Country,
			Visibility:        actor.Visibility,
			OwnedProjectCount: ownedCounts[actor.ID],
			MemberCount:       memberCounts[actor.ID],
		})
	}

	return &BrowseResult[InstitutionBrowseItem]{Items: items, Total: total}, nil
}

func (s *Service) GetUser(ctx context.Context, id string) (*sqlcgen.WeaveActor, error) {
	row, err := s.store.GetActorByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if row == nil || row.Type != "person" {
		return nil, nil
	}
	return row, nil
}

func (s *Service) GetInstitution(ctx context.Context, id string) (*sqlcgen.WeaveActor, error) {
	row, err := s.store.GetActorByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get institution: %w", err)
	}
	if row == nil || row.Type != "organization" {
		return nil, nil
	}
	return row, nil
}

func (s *Service) InstitutionOptions(ctx context.Context) ([]formschema.SelectOption, error) {
	rows, err := s.store.ListActorsByType(ctx, "organization")
	if err != nil {
		return nil, fmt.Errorf("institution options: %w", err)
	}
	opts := make([]formschema.SelectOption, 0, len(rows))
	for _, row := range rows {
		label := row.DisplayName
		if row.Acronym != nil && *row.Acronym != "" {
			label = fmt.Sprintf("%s (%s)", row.DisplayName, *row.Acronym)
		}
		opts = append(opts, formschema.SelectOption{
			Value: row.ID,
			Label: domain.Translations{"en": label},
		})
	}
	slices.SortFunc(opts, func(a, b formschema.SelectOption) int {
		return strings.Compare(optionLabel(a), optionLabel(b))
	})
	return opts, nil
}

// SelfProfileInput carries the fields a logged-in user is allowed to
// edit on their own profile via PUT /me. Slug, email, role, parent_id,
// type, staging_id are not editable from this surface — the admin
// UpdateUser path owns those.
type SelfProfileInput struct {
	DisplayName string
	Country     *string
	Website     *string
	Orcid       *string
}

// UpdateSelf writes a self-edit to weave_actors. Reuses the WeaveCreateActor
// upsert with the current row's identity-bearing fields preserved
// (slug/role/email/type/parent_id) so the user can't escalate or
// rebrand. Returns the updated row, or nil if actorID has no row.
func (s *Service) UpdateSelf(ctx context.Context, actorID string, in SelfProfileInput) (*sqlcgen.WeaveActor, error) {
	current, err := s.GetUser(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	row, err := s.store.CreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          current.ID,
		Type:        current.Type,
		DisplayName: strings.TrimSpace(in.DisplayName),
		SystemName:  current.SystemName,
		Slug:        current.Slug,
		FirstName:   current.FirstName,
		LastName:    current.LastName,
		Acronym:     current.Acronym,
		Email:       current.Email,
		Country:     dbutil.TrimNullable(in.Country),
		Website:     dbutil.TrimNullable(in.Website),
		Orcid:       dbutil.TrimNullable(in.Orcid),
		Role:        current.Role,
		ParentID:    current.ParentID,
		StagingID:   current.StagingID,
	})
	if err != nil {
		return nil, fmt.Errorf("update self: %w", err)
	}
	return row, nil
}

// ChangePassword verifies the actor's current password against the
// stored bcrypt hash and writes a new hash on match. Returns
// ErrPasswordIncorrect on mismatch so the handler can surface a 422
// without leaking timing information about whether the actor row exists.
func (s *Service) ChangePassword(ctx context.Context, actorID, current, next string) error {
	if s.auth == nil {
		return fmt.Errorf("auth store not configured")
	}
	rec, err := s.auth.GetByActorID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("load auth record: %w", err)
	}
	if rec == nil {
		return ErrPasswordIncorrect
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(current)); err != nil {
		return ErrPasswordIncorrect
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := s.auth.UpdatePassword(ctx, actorID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (s *Service) UpdateUser(ctx context.Context, id string, in UserEditInput) (*sqlcgen.WeaveActor, error) {
	current, err := s.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	row, err := s.store.CreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          current.ID,
		Type:        "person",
		DisplayName: strings.TrimSpace(in.DisplayName),
		SystemName:  dbutil.TrimEmptyToNil(strings.TrimSpace(in.Slug)),
		Slug:        strings.TrimSpace(in.Slug),
		FirstName:   dbutil.TrimNullable(in.FirstName),
		LastName:    dbutil.TrimNullable(in.LastName),
		Acronym:     current.Acronym,
		Email:       dbutil.TrimNullable(in.Email),
		Country:     dbutil.TrimNullable(in.Country),
		Website:     dbutil.TrimNullable(in.Website),
		Orcid:       dbutil.TrimNullable(in.Orcid),
		Role:        strings.TrimSpace(in.Role),
		ParentID:    dbutil.TrimNullable(in.ParentID),
		StagingID:   current.StagingID,
	})
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return row, nil
}

// CreateUser creates the actor AND its weave_auth credential row. When
// in.Password is nil/empty a password is generated and returned as the
// second value so the admin can share it once; when a password is supplied
// the returned string is empty. A user without a credential row cannot log
// in or be reset, which is why the two writes are coupled here.
func (s *Service) CreateUser(ctx context.Context, in UserCreateInput) (*sqlcgen.WeaveActor, string, error) {
	row, err := s.store.CreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ids.GenerateULID(),
		Type:        "person",
		DisplayName: strings.TrimSpace(in.DisplayName),
		SystemName:  dbutil.TrimEmptyToNil(strings.TrimSpace(in.Slug)),
		Slug:        strings.TrimSpace(in.Slug),
		FirstName:   dbutil.TrimNullable(in.FirstName),
		LastName:    dbutil.TrimNullable(in.LastName),
		Email:       dbutil.TrimNullable(in.Email),
		Country:     dbutil.TrimNullable(in.Country),
		Website:     dbutil.TrimNullable(in.Website),
		Orcid:       dbutil.TrimNullable(in.Orcid),
		Role:        strings.TrimSpace(in.Role),
		ParentID:    dbutil.TrimNullable(in.ParentID),
	})
	if err != nil {
		return nil, "", fmt.Errorf("create user: %w", err)
	}

	if s.auth == nil {
		return row, "", fmt.Errorf("auth store not configured")
	}

	generated := ""
	plain := ""
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		plain = *in.Password
	} else {
		pw, err := generatePassword(14)
		if err != nil {
			return row, "", err
		}
		plain, generated = pw, pw
	}
	hash, err := hashPassword(plain)
	if err != nil {
		return row, "", err
	}
	if _, err := s.auth.Create(ctx, row.ID, hash, nil); err != nil {
		return row, "", fmt.Errorf("create auth record: %w", err)
	}
	return row, generated, nil
}

func (s *Service) CreateInstitution(ctx context.Context, in InstitutionCreateInput) (*sqlcgen.WeaveActor, error) {
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = "public"
	}
	row, err := s.store.CreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ids.GenerateULID(),
		Type:        "organization",
		DisplayName: strings.TrimSpace(in.DisplayName),
		SystemName:  dbutil.TrimEmptyToNil(strings.TrimSpace(in.Slug)),
		Slug:        strings.TrimSpace(in.Slug),
		Acronym:     dbutil.TrimNullable(in.Acronym),
		Country:     dbutil.TrimNullable(in.Country),
		Website:     dbutil.TrimNullable(in.Website),
		Role:        "contributor",
		Visibility:  visibility,
	})
	if err != nil {
		return nil, fmt.Errorf("create institution: %w", err)
	}
	return row, nil
}

func (s *Service) UpdateInstitution(ctx context.Context, id string, in InstitutionEditInput) (*sqlcgen.WeaveActor, error) {
	current, err := s.GetInstitution(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	// Preserve the current visibility when the caller didn't send a value.
	// WeaveCreateActor uses ON CONFLICT DO UPDATE so any missing field
	// would otherwise be overwritten with the Go zero value.
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = current.Visibility
	}
	row, err := s.store.CreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          current.ID,
		Type:        "organization",
		DisplayName: strings.TrimSpace(in.DisplayName),
		SystemName:  dbutil.TrimEmptyToNil(strings.TrimSpace(in.Slug)),
		Slug:        strings.TrimSpace(in.Slug),
		Acronym:     dbutil.TrimNullable(in.Acronym),
		Country:     dbutil.TrimNullable(in.Country),
		Website:     dbutil.TrimNullable(in.Website),
		Role:        current.Role,
		StagingID:   current.StagingID,
		Visibility:  visibility,
	})
	if err != nil {
		return nil, fmt.Errorf("update institution: %w", err)
	}
	return row, nil
}

func matchesUserSearch(actor sqlcgen.WeaveActor, search string, instNames map[string]string) bool {
	return containsAny(search,
		actor.DisplayName,
		actor.Slug,
		deref(actor.Email),
		deref(actor.Country),
		instNames[deref(actor.ParentID)],
	)
}

func matchesInstitutionSearch(actor sqlcgen.WeaveActor, search string) bool {
	return containsAny(search,
		actor.DisplayName,
		actor.Slug,
		deref(actor.Acronym),
		deref(actor.Country),
		deref(actor.Website),
	)
}

func sortUsers(actors []sqlcgen.WeaveActor, instNames map[string]string, sortBy string) {
	slices.SortFunc(actors, func(a, b sqlcgen.WeaveActor) int {
		switch sortBy {
		case "email":
			return compareStrings(deref(a.Email), deref(b.Email))
		case "role":
			return compareStrings(a.Role, b.Role)
		case "organization":
			return compareStrings(instNames[deref(a.ParentID)], instNames[deref(b.ParentID)])
		default:
			return compareStrings(a.DisplayName, b.DisplayName)
		}
	})
}

func sortInstitutions(actors []sqlcgen.WeaveActor, sortBy string) {
	slices.SortFunc(actors, func(a, b sqlcgen.WeaveActor) int {
		switch sortBy {
		case "country":
			return compareStrings(deref(a.Country), deref(b.Country))
		case "slug":
			return compareStrings(a.Slug, b.Slug)
		default:
			return compareStrings(a.DisplayName, b.DisplayName)
		}
	})
}

func paginateActors(rows []sqlcgen.WeaveActor, page, perPage int) ([]sqlcgen.WeaveActor, int) {
	total := len(rows)
	start := (page - 1) * perPage
	if start >= total {
		return []sqlcgen.WeaveActor{}, total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return rows[start:end], total
}

func actorIDs(rows []sqlcgen.WeaveActor) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func sanitizePage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	switch {
	case perPage <= 0:
		perPage = 25
	case perPage > 100:
		perPage = 100
	}
	return page, perPage
}

func formatLastLogin(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return ts.Format("2006-01-02")
}

func containsAny(search string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), search) {
			return true
		}
	}
	return false
}

func compareStrings(a, b string) int {
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}


func optionLabel(opt formschema.SelectOption) string {
	// Label is typed `any` to hold either a raw domain.Translations or
	// an i18n.LocalizedText. Both expose an embedded Translations map
	// (LocalizedText embeds it), so we type-switch and read the "en"
	// key from whichever shape is present.
	var t domain.Translations
	switch v := opt.Label.(type) {
	case domain.Translations:
		t = v
	case i18n.LocalizedText:
		t = v.Translations
	default:
		return ""
	}
	if v, ok := t["en"]; ok {
		return v
	}
	for _, v := range t {
		return v
	}
	return ""
}

