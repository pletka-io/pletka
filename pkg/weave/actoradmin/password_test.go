package actoradmin

import (
	"context"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

// --- fakes ---

type fakeStore struct {
	created *sqlcgen.WeaveActor
}

func (f *fakeStore) ListActorsByType(context.Context, string) ([]sqlcgen.WeaveActor, error) {
	return nil, nil
}
func (f *fakeStore) GetActorByID(_ context.Context, id string) (*sqlcgen.WeaveActor, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, nil
}
func (f *fakeStore) CreateActor(_ context.Context, p sqlcgen.WeaveCreateActorParams) (*sqlcgen.WeaveActor, error) {
	f.created = &sqlcgen.WeaveActor{ID: p.ID, DisplayName: p.DisplayName, Role: p.Role}
	return f.created, nil
}
func (f *fakeStore) ProjectCounts(context.Context, []string) (map[string]int, error) { return nil, nil }
func (f *fakeStore) OwnedProjectCounts(context.Context, []string) (map[string]int, error) {
	return nil, nil
}
func (f *fakeStore) MemberCounts(context.Context, []string) (map[string]int, error) { return nil, nil }
func (f *fakeStore) LastLogins(context.Context, []string) (map[string]*time.Time, error) {
	return nil, nil
}

// fakeAuth records password hashes keyed by actor ID.
type fakeAuth struct {
	hashes    map[string]string
	createErr error
}

func newFakeAuth() *fakeAuth { return &fakeAuth{hashes: map[string]string{}} }

func (f *fakeAuth) RegisterPerson(context.Context, domain.RegisterPersonParams) (*domain.AuthWithActor, error) {
	return nil, nil
}
func (f *fakeAuth) Create(_ context.Context, actorID, hash string, _ *time.Time) (*domain.AuthRecord, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.hashes[actorID] = hash
	return &domain.AuthRecord{ActorID: actorID, PasswordHash: hash}, nil
}
func (f *fakeAuth) GetByActorID(_ context.Context, actorID string) (*domain.AuthRecord, error) {
	h, ok := f.hashes[actorID]
	if !ok {
		return nil, nil
	}
	return &domain.AuthRecord{ActorID: actorID, PasswordHash: h}, nil
}
func (f *fakeAuth) GetByEmail(context.Context, string) (*domain.AuthWithActor, error) {
	return nil, nil
}
func (f *fakeAuth) GetByEmailOrSlug(context.Context, string) (*domain.AuthWithActor, error) {
	return nil, nil
}
func (f *fakeAuth) GetProfileByActorID(context.Context, string) (*domain.AuthWithActor, error) {
	return nil, nil
}
func (f *fakeAuth) UpdatePassword(_ context.Context, actorID, hash string) error {
	if _, ok := f.hashes[actorID]; !ok {
		// mimic UPDATE affecting 0 rows: silently no-op, no error
		return nil
	}
	f.hashes[actorID] = hash
	return nil
}
func (f *fakeAuth) MarkLogin(context.Context, string) error                      { return nil }
func (f *fakeAuth) SetResetToken(context.Context, string, string, time.Time) error { return nil }
func (f *fakeAuth) GetByResetToken(context.Context, string) (*domain.AuthRecord, error) {
	return nil, nil
}
func (f *fakeAuth) ClearResetToken(context.Context, string) error   { return nil }
func (f *fakeAuth) BumpPermsVersion(context.Context, string) error { return nil }
func (f *fakeAuth) Delete(context.Context, string) error           { return nil }

// --- tests ---

func TestCreateUser_GeneratesAuthRowWhenNoPassword(t *testing.T) {
	store := &fakeStore{}
	auth := newFakeAuth()
	svc := NewService(store, auth, nil)

	row, pw, err := svc.CreateUser(context.Background(), UserCreateInput{
		DisplayName: "Ada Lovelace", Slug: "ada", Role: "contributor",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if row == nil || row.ID == "" {
		t.Fatal("expected created actor with id")
	}
	if pw == "" {
		t.Fatal("expected a generated password to be returned")
	}
	if len(pw) < 12 {
		t.Errorf("generated password too short: %d chars", len(pw))
	}
	hash, ok := auth.hashes[row.ID]
	if !ok {
		t.Fatal("expected a weave_auth row for the new user")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		t.Error("returned password does not match the stored hash")
	}
}

func TestCreateUser_UsesSuppliedPassword(t *testing.T) {
	store := &fakeStore{}
	auth := newFakeAuth()
	svc := NewService(store, auth, nil)

	supplied := "correct horse battery"
	row, pw, err := svc.CreateUser(context.Background(), UserCreateInput{
		DisplayName: "Grace Hopper", Slug: "grace", Role: "contributor",
		Password: &supplied,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if pw != "" {
		t.Errorf("supplied password: expected empty returned pw, got %q", pw)
	}
	if bcrypt.CompareHashAndPassword([]byte(auth.hashes[row.ID]), []byte(supplied)) != nil {
		t.Error("stored hash does not match the supplied password")
	}
}

func TestSetPassword_CreatesAuthRowWhenMissing(t *testing.T) {
	store := &fakeStore{}
	auth := newFakeAuth()
	svc := NewService(store, auth, nil)

	// Simulate a legacy admin-created user: actor exists, no auth row.
	store.created = &sqlcgen.WeaveActor{ID: "ACT.legacy"}

	if err := svc.SetPassword(context.Background(), "ACT.legacy", "newpassword123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	hash, ok := auth.hashes["ACT.legacy"]
	if !ok {
		t.Fatal("SetPassword must create the auth row when none exists")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpassword123")) != nil {
		t.Error("stored hash does not match")
	}
}

func TestSetPassword_UpdatesExistingAuthRow(t *testing.T) {
	store := &fakeStore{}
	auth := newFakeAuth()
	auth.hashes["ACT.1"] = "old-hash"
	svc := NewService(store, auth, nil)

	if err := svc.SetPassword(context.Background(), "ACT.1", "brandnewpass99"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(auth.hashes["ACT.1"]), []byte("brandnewpass99")) != nil {
		t.Error("existing auth row not updated to the new password")
	}
}

func TestSetPassword_RejectsShortPassword(t *testing.T) {
	svc := NewService(&fakeStore{}, newFakeAuth(), nil)
	if err := svc.SetPassword(context.Background(), "ACT.1", "short"); err == nil {
		t.Fatal("expected error for a password under 12 chars")
	}
}
