package hub_test

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/hub"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

type stubIntegration struct {
	id       string
	secrets  []string
	validate func(map[string]any) (map[string]any, map[string][]string)
	applyTo  []registry.Format
}

func (s *stubIntegration) ID() string                   { return s.id }
func (s *stubIntegration) Metadata() registry.Metadata  { return registry.Metadata{} }
func (s *stubIntegration) AppliesTo() []registry.Format { return s.applyTo }
func (s *stubIntegration) SecretFields() []string       { return s.secrets }
func (s *stubIntegration) ConfigSchema(registry.ConfigRequest, map[string]any) *formschema.FormSchema {
	return nil
}
func (s *stubIntegration) ValidateConfig(raw map[string]any) (map[string]any, map[string][]string) {
	if s.validate != nil {
		return s.validate(raw)
	}
	return raw, nil
}
func (s *stubIntegration) Actions() []registry.ActionSpec { return nil }
func (s *stubIntegration) RunAction(context.Context, string, registry.ActionInput) (registry.ActionResult, error) {
	return registry.ActionResult{}, nil
}

type fakeStore struct {
	rows map[string]*hub.ProjectIntegration
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]*hub.ProjectIntegration{}} }

func keyOf(p, i, c string) string { return p + "|" + i + "|" + c }

func (f *fakeStore) Get(_ context.Context, p, i, c string) (*hub.ProjectIntegration, error) {
	row, ok := f.rows[keyOf(p, i, c)]
	if !ok {
		return nil, nil
	}
	return clone(row), nil
}
func (f *fakeStore) ListForProject(_ context.Context, p string) ([]*hub.ProjectIntegration, error) {
	out := []*hub.ProjectIntegration{}
	for _, r := range f.rows {
		if r.ProjectID == p {
			out = append(out, clone(r))
		}
	}
	return out, nil
}
func (f *fakeStore) ListConfigsForIntegration(_ context.Context, p, i string) ([]*hub.ProjectIntegration, error) {
	out := []*hub.ProjectIntegration{}
	for _, r := range f.rows {
		if r.ProjectID == p && r.IntegrationID == i {
			out = append(out, clone(r))
		}
	}
	return out, nil
}
func (f *fakeStore) ListEnabledForProject(_ context.Context, p string) ([]*hub.ProjectIntegration, error) {
	out := []*hub.ProjectIntegration{}
	for _, r := range f.rows {
		if r.ProjectID == p && r.Enabled {
			out = append(out, clone(r))
		}
	}
	return out, nil
}
func (f *fakeStore) Upsert(_ context.Context, pi *hub.ProjectIntegration) (*hub.ProjectIntegration, error) {
	saved := clone(pi)
	f.rows[keyOf(pi.ProjectID, pi.IntegrationID, pi.ConfigID)] = saved
	return clone(saved), nil
}
func (f *fakeStore) Delete(_ context.Context, p, i, c string) error {
	delete(f.rows, keyOf(p, i, c))
	return nil
}

func clone(pi *hub.ProjectIntegration) *hub.ProjectIntegration {
	cp := *pi
	cfg := make(map[string]any, len(pi.Config))
	for k, v := range pi.Config {
		cfg[k] = v
	}
	cp.Config = cfg
	return &cp
}

func newTestCipher(t *testing.T) *integrations.Cipher {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	c, err := integrations.NewCipher(hex.EncodeToString(raw))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return c
}

func TestService_AddConfig_AssignsULID(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem", secrets: []string{"password"}})
	store := newFakeStore()
	svc := hub.NewService(store, reg, newTestCipher(t), nil)

	saved, err := svc.AddConfig(context.Background(), hub.AddConfigInput{
		ProjectID:     "P1",
		IntegrationID: "threem",
		Label:         "production",
	})
	if err != nil {
		t.Fatalf("AddConfig: %v", err)
	}
	if saved.ConfigID == "" {
		t.Fatalf("config_id not assigned")
	}
	if saved.Label != "production" {
		t.Fatalf("label: %q", saved.Label)
	}
	if saved.Enabled {
		t.Fatalf("new config should default to disabled until SaveConfig + SetEnabled")
	}
}

func TestService_AddConfig_TwoConfigsCoexist(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem"})
	store := newFakeStore()
	svc := hub.NewService(store, reg, nil, nil)

	a, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "prod"})
	b, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "staging"})
	if a.ConfigID == b.ConfigID {
		t.Fatalf("expected distinct config IDs")
	}
	_, rows, err := svc.ListConfigs(context.Background(), "P1", "threem")
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 configs, got %d", len(rows))
	}
}

func TestService_SaveConfig_RequiresExistingRow(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem"})
	svc := hub.NewService(newFakeStore(), reg, nil, nil)

	_, _, err := svc.SaveConfig(context.Background(), hub.SaveConfigInput{
		ProjectID:     "P1",
		IntegrationID: "threem",
		ConfigID:      "01HYNONEXISTENT",
		RawConfig:     map[string]any{},
	})
	if !errors.Is(err, hub.ErrConfigNotFound) {
		t.Fatalf("want ErrConfigNotFound, got %v", err)
	}
}

func TestService_SaveConfig_PreservesEnabledAndSecret(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem", secrets: []string{"password"}})
	store := newFakeStore()
	svc := hub.NewService(store, reg, newTestCipher(t), nil)

	added, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "prod"})
	first, _, err := svc.SaveConfig(context.Background(), hub.SaveConfigInput{
		ProjectID: "P1", IntegrationID: "threem", ConfigID: added.ConfigID,
		RawConfig: map[string]any{"username": "alice", "password": "secret"},
	})
	if err != nil {
		t.Fatalf("first SaveConfig: %v", err)
	}
	firstPW := first.Config["password"].(string)
	if firstPW == "secret" {
		t.Fatalf("password not encrypted")
	}
	// Flip enabled outside SaveConfig
	_, err = svc.SetEnabled(context.Background(), "P1", "threem", added.ConfigID, true)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	// Save again with blank password — prior ciphertext preserved, enabled preserved.
	second, _, err := svc.SaveConfig(context.Background(), hub.SaveConfigInput{
		ProjectID: "P1", IntegrationID: "threem", ConfigID: added.ConfigID,
		RawConfig: map[string]any{"username": "alice", "password": ""},
	})
	if err != nil {
		t.Fatalf("second SaveConfig: %v", err)
	}
	if second.Config["password"] != firstPW {
		t.Fatalf("prior ciphertext not preserved")
	}
	if !second.Enabled {
		t.Fatalf("enabled state not preserved across SaveConfig")
	}
}

func TestService_LoadForAction_DecryptsSecrets(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem", secrets: []string{"password"}})
	store := newFakeStore()
	svc := hub.NewService(store, reg, newTestCipher(t), nil)

	added, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "prod"})
	_, _, _ = svc.SaveConfig(context.Background(), hub.SaveConfigInput{
		ProjectID: "P1", IntegrationID: "threem", ConfigID: added.ConfigID,
		RawConfig: map[string]any{"username": "alice", "password": "secret"},
	})
	_, _ = svc.SetEnabled(context.Background(), "P1", "threem", added.ConfigID, true)

	_, _, cfg, err := svc.LoadForAction(context.Background(), "P1", "threem", added.ConfigID)
	if err != nil {
		t.Fatalf("LoadForAction: %v", err)
	}
	if cfg["password"] != "secret" {
		t.Fatalf("password not decrypted: %v", cfg["password"])
	}
}

func TestService_LoadForAction_RefusesDisabled(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem"})
	store := newFakeStore()
	svc := hub.NewService(store, reg, nil, nil)
	added, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "off"})
	if _, _, _, err := svc.LoadForAction(context.Background(), "P1", "threem", added.ConfigID); err == nil {
		t.Fatalf("want disabled error")
	}
}

func TestService_Remove_OnlyDropsTargetConfig(t *testing.T) {
	reg, _ := registry.NewRegistry(&stubIntegration{id: "threem"})
	store := newFakeStore()
	svc := hub.NewService(store, reg, nil, nil)

	a, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "prod"})
	b, _ := svc.AddConfig(context.Background(), hub.AddConfigInput{ProjectID: "P1", IntegrationID: "threem", Label: "staging"})
	if err := svc.Remove(context.Background(), "P1", "threem", a.ConfigID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, rows, _ := svc.ListConfigs(context.Background(), "P1", "threem")
	if len(rows) != 1 || rows[0].ConfigID != b.ConfigID {
		t.Fatalf("expected only %q to remain, got %+v", b.ConfigID, rows)
	}
}
