package hub

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
)

// ErrIntegrationNotRegistered is returned when a stored row references
// an integration ID that no longer exists in the registry (e.g. the
// integration was removed from a private build between deploys).
var ErrIntegrationNotRegistered = errors.New("integration not registered")

// ErrConfigNotFound is returned when an operation targets a configID
// the project has not configured. Maps to HTTP 404.
var ErrConfigNotFound = errors.New("integration config not found")

// Service is the hub's orchestration layer. Handlers depend on this
// type; the implementation wires Store + Registry + Cipher.
type Service struct {
	store           Store
	registry        *registry.Registry
	cipher          *integrations.Cipher
	logger          *slog.Logger
	managedResolver ManagedConfigResolver
	managedConflict ManagedConflictChecker
	managedLabel    ManagedLabelResolver
}

// ManagedConflictChecker reports whether the given fleet instance is
// already linked to a project_integration row. Used by SaveConfig to
// surface a clean field error before the partial-unique index trips
// at INSERT. Returns nil when no link exists.
type ManagedConflictChecker func(ctx context.Context, instanceID string) (link *ManagedConflictLink, err error)

// ManagedConflictLink describes the existing binding.
type ManagedConflictLink struct {
	ProjectID     string
	IntegrationID string
	ConfigID      string
	Label         string
}

// SetManagedConflictChecker installs the conflict check. Idempotent.
func (s *Service) SetManagedConflictChecker(c ManagedConflictChecker) {
	s.managedConflict = c
}

// ManagedLabelResolver returns a human-readable label for the
// supplied fleet instance ID. Used by list-schema so the project
// settings page can render "managed · ptest" instead of
// "managed · 01KTHPRC…".
type ManagedLabelResolver func(ctx context.Context, instanceID string) (string, error)

// SetManagedLabelResolver installs the label resolver. Idempotent.
func (s *Service) SetManagedLabelResolver(r ManagedLabelResolver) {
	s.managedLabel = r
}

// ResolveManagedLabel is exposed for handlers that build the list
// payload. Returns the resolver's output (or the raw ID prefix as a
// safe fallback when the resolver is unset or errors).
func (s *Service) ResolveManagedLabel(ctx context.Context, instanceID string) string {
	if instanceID == "" {
		return ""
	}
	if s.managedLabel == nil {
		return ""
	}
	label, err := s.managedLabel(ctx, instanceID)
	if err != nil || label == "" {
		return ""
	}
	return label
}

// NewService constructs the hub service. The cipher may be nil; in that
// case any integration that declares secret fields will produce a clear
// configuration error on save instead of silently storing plaintext.
func NewService(store Store, reg *registry.Registry, cipher *integrations.Cipher, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		registry: reg,
		cipher:   cipher,
		logger:   logger.With("service", "integrations.hub"),
	}
}

// EnumeratedIntegration pairs registry metadata with every persisted
// config a project has for the integration. Returned by List for the
// settings page; Configs is empty when the project has never touched
// this integration.
type EnumeratedIntegration struct {
	Integration registry.Integration
	Configs     []*ProjectIntegration
}

// List returns every registered integration paired with its stored
// configs (zero or more) for the given project.
func (s *Service) List(ctx context.Context, projectID string) ([]EnumeratedIntegration, error) {
	rows, err := s.store.ListForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byIntegration := make(map[string][]*ProjectIntegration, len(rows))
	for _, row := range rows {
		byIntegration[row.IntegrationID] = append(byIntegration[row.IntegrationID], row)
	}
	all := s.registry.All()
	out := make([]EnumeratedIntegration, 0, len(all))
	for _, integ := range all {
		out = append(out, EnumeratedIntegration{
			Integration: integ,
			Configs:     byIntegration[integ.ID()],
		})
	}
	return out, nil
}

// ListConfigs returns every persisted config for a single
// (project, integration) pair.
func (s *Service) ListConfigs(ctx context.Context, projectID, integrationID string) (registry.Integration, []*ProjectIntegration, error) {
	integ, ok := s.registry.Integration(integrationID)
	if !ok {
		return nil, nil, ErrIntegrationNotRegistered
	}
	rows, err := s.store.ListConfigsForIntegration(ctx, projectID, integrationID)
	if err != nil {
		return integ, nil, err
	}
	return integ, rows, nil
}

// AddConfigInput is what the hub passes to AddConfig — used by the
// settings UI's "Add config" button to seed a fresh row before the
// operator fills in the config form.
type AddConfigInput struct {
	ProjectID     string
	IntegrationID string
	Label         string
}

// AddConfig creates a new (project, integration, configID) row with a
// fresh ULID and the supplied label. Enabled defaults to false; the
// operator opens the config form, fills it in, and SaveConfig flips
// enabled to true (preserving prior state on subsequent edits).
func (s *Service) AddConfig(ctx context.Context, in AddConfigInput) (*ProjectIntegration, error) {
	if _, ok := s.registry.Integration(in.IntegrationID); !ok {
		return nil, ErrIntegrationNotRegistered
	}
	configID := generateConfigID()
	return s.store.Upsert(ctx, &ProjectIntegration{
		ProjectID:     in.ProjectID,
		IntegrationID: in.IntegrationID,
		ConfigID:      configID,
		Label:         in.Label,
		Enabled:       false,
		Config:        map[string]any{},
	})
}

// GetForConfigForm returns the persisted config with secret fields
// redacted, suitable for echoing back into the config form.
//
// Returns (nil, ErrConfigNotFound) when configID has no row.
func (s *Service) GetForConfigForm(ctx context.Context, projectID, integrationID, configID string) (registry.Integration, *ProjectIntegration, map[string]any, error) {
	integ, ok := s.registry.Integration(integrationID)
	if !ok {
		return nil, nil, nil, ErrIntegrationNotRegistered
	}
	row, err := s.store.Get(ctx, projectID, integrationID, configID)
	if err != nil {
		return integ, nil, nil, err
	}
	if row == nil {
		return integ, nil, nil, ErrConfigNotFound
	}
	cfg := make(map[string]any, len(row.Config))
	for k, v := range row.Config {
		cfg[k] = v
	}
	integrations.RedactFields(cfg, integ.SecretFields())
	// Surface the top-level columns the integration's ConfigSchema
	// reads to derive UI state (source selector, managed dropdown).
	if row.ManagedInstanceID != "" {
		cfg["managed_instance_id"] = row.ManagedInstanceID
		cfg["source"] = "managed"
	}
	return integ, row, cfg, nil
}

// SaveConfigInput is the validated input for SaveConfig.
type SaveConfigInput struct {
	ProjectID     string
	IntegrationID string
	ConfigID      string
	Label         string
	RawConfig     map[string]any
}

// SaveConfig validates raw config through the integration, encrypts
// secret fields, and upserts the row. Preserves the row's existing
// enabled state (or defaults to true on first save). Returns the
// field-error map from ValidateConfig when validation fails — the row
// is not written in that case.
func (s *Service) SaveConfig(ctx context.Context, in SaveConfigInput) (*ProjectIntegration, map[string][]string, error) {
	integ, ok := s.registry.Integration(in.IntegrationID)
	if !ok {
		return nil, nil, ErrIntegrationNotRegistered
	}

	rawCfg := in.RawConfig
	if rawCfg == nil {
		rawCfg = map[string]any{}
	}
	// managed_instance_id and the source toggle are top-level row
	// columns / UI state, not part of the integration's validated
	// config map. Pull them out before ValidateConfig so the
	// integration's allowed-field set doesn't reject them.
	managedFromRaw := ""
	if v, ok := rawCfg["managed_instance_id"].(string); ok {
		managedFromRaw = strings.TrimSpace(v)
	}
	sourceFromRaw, _ := rawCfg["source"].(string)
	delete(rawCfg, "managed_instance_id")
	// Keep `source` in rawCfg until after ValidateConfig so the
	// integration can branch its required-field rules on the source
	// mode (managed-mode skips manual base_url/OAuth checks).
	// We delete it from the normalised output before writing to
	// store — `source` is a UI toggle, not a config field.

	prior, err := s.store.Get(ctx, in.ProjectID, in.IntegrationID, in.ConfigID)
	if err != nil {
		return nil, nil, err
	}
	if prior == nil {
		return nil, nil, ErrConfigNotFound
	}

	// Preserve existing secret values: if a secret field arrives empty
	// (redacted by GetForConfigForm), pull the prior ciphertext from the
	// stored row so the user does not have to retype unchanged secrets.
	secretKeys := integ.SecretFields()
	for _, k := range secretKeys {
		v, _ := rawCfg[k].(string)
		if v != "" {
			continue
		}
		if existing, ok := prior.Config[k]; ok {
			rawCfg[k] = existing
		}
	}

	normalised, fieldErrors := integ.ValidateConfig(rawCfg)
	if len(fieldErrors) > 0 {
		return nil, fieldErrors, nil
	}

	toStore := make(map[string]any, len(normalised))
	for k, v := range normalised {
		toStore[k] = v
	}
	if len(secretKeys) > 0 {
		if s.cipher == nil {
			return nil, nil, fmt.Errorf("%w: integration %q declares secret fields", integrations.ErrCipherUnavailable, in.IntegrationID)
		}
		for _, k := range secretKeys {
			v, _ := toStore[k].(string)
			if v == "" {
				continue
			}
			if _, err := s.cipher.Decrypt(v); err == nil {
				continue // already ciphertext
			}
			enc, err := s.cipher.Encrypt([]byte(v))
			if err != nil {
				return nil, nil, fmt.Errorf("encrypt %q: %w", k, err)
			}
			toStore[k] = enc
		}
	}

	label := in.Label
	if label == "" {
		label = prior.Label
	}

	// managed_instance_id is a top-level row column, not part of the
	// JSONB config. When source == "manual" the column is cleared so
	// the runtime overlay stops kicking in even if a prior managed
	// link was stored.
	managed := prior.ManagedInstanceID
	if sourceFromRaw == "managed" || managedFromRaw != "" {
		managed = managedFromRaw
	}
	if sourceFromRaw == "manual" {
		managed = ""
	}
	// Conflict guard: if we're newly binding to a fleet instance
	// (managed changed from "" or from a different ID), reject
	// when another project already holds the link. The partial-
	// unique index in migration 060 is the authoritative check;
	// this just surfaces a clean field error first.
	if managed != "" && managed != prior.ManagedInstanceID && s.managedConflict != nil {
		existing, lookupErr := s.managedConflict(ctx, managed)
		if lookupErr != nil {
			return nil, nil, fmt.Errorf("managed conflict check: %w", lookupErr)
		}
		if existing != nil &&
			(existing.ProjectID != in.ProjectID || existing.ConfigID != in.ConfigID) {
			return nil, map[string][]string{
				"managed_instance_id": {
					fmt.Sprintf("Instance already linked to project %q (config %q, label %q). Unbind there first.",
						existing.ProjectID, existing.ConfigID, existing.Label),
				},
			}, nil
		}
	}

	saved, err := s.store.Upsert(ctx, &ProjectIntegration{
		ProjectID:         in.ProjectID,
		IntegrationID:     in.IntegrationID,
		ConfigID:          in.ConfigID,
		Label:             label,
		Enabled:           prior.Enabled,
		Config:            toStore,
		ManagedInstanceID: managed,
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, nil, nil
}

// SetEnabled flips a config's enabled flag without touching config.
func (s *Service) SetEnabled(ctx context.Context, projectID, integrationID, configID string, enabled bool) (*ProjectIntegration, error) {
	if _, ok := s.registry.Integration(integrationID); !ok {
		return nil, ErrIntegrationNotRegistered
	}
	row, err := s.store.Get(ctx, projectID, integrationID, configID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConfigNotFound
	}
	row.Enabled = enabled
	return s.store.Upsert(ctx, row)
}

// Remove drops a single (projectID, integrationID, configID) row. Idempotent.
func (s *Service) Remove(ctx context.Context, projectID, integrationID, configID string) error {
	if _, ok := s.registry.Integration(integrationID); !ok {
		return ErrIntegrationNotRegistered
	}
	return s.store.Delete(ctx, projectID, integrationID, configID)
}

// LoadForAction returns the integration + decrypted config for an
// action invocation, scoped to a single config_id.
func (s *Service) LoadForAction(ctx context.Context, projectID, integrationID, configID string) (registry.Integration, *ProjectIntegration, map[string]any, error) {
	integ, ok := s.registry.Integration(integrationID)
	if !ok {
		return nil, nil, nil, ErrIntegrationNotRegistered
	}
	row, err := s.store.Get(ctx, projectID, integrationID, configID)
	if err != nil {
		return nil, nil, nil, err
	}
	if row == nil {
		return integ, nil, nil, ErrConfigNotFound
	}
	if !row.Enabled {
		return integ, row, nil, fmt.Errorf("integration %q config %q not enabled", integrationID, configID)
	}
	cfg := make(map[string]any, len(row.Config))
	for k, v := range row.Config {
		cfg[k] = v
	}
	if err := integrations.DecryptFields(s.cipher, cfg, integ.SecretFields()); err != nil {
		return integ, row, nil, fmt.Errorf("decrypt config: %w", err)
	}
	// Managed binding: when row.ManagedInstanceID is set, overlay
	// the connection fields from the managed instance store (wired by
	// the hosting binary via SetManagedConfigResolver) so cred rotation
	// on the fleet side propagates without touching project rows.
	if row.ManagedInstanceID != "" && s.managedResolver != nil {
		over, err := s.managedResolver(ctx, row.ManagedInstanceID)
		if err != nil {
			return integ, row, nil, fmt.Errorf("managed resolver: %w", err)
		}
		for k, v := range over {
			cfg[k] = v
		}
	}
	return integ, row, cfg, nil
}

// ManagedConfigResolver returns overrides for a managed integration
// config given the linked instance ID. Wired by the hosting binary via
// SetManagedConfigResolver once the managed instance store is
// constructed; nil = no overlay (manual-only deployments unaffected).
type ManagedConfigResolver func(ctx context.Context, instanceID string) (map[string]any, error)

// SetManagedConfigResolver installs the overlay function. Idempotent.
func (s *Service) SetManagedConfigResolver(r ManagedConfigResolver) {
	s.managedResolver = r
}

// generateConfigID returns a fresh ULID. Crockford-base32, monotonic
// within the process. Internal to the service so callers can't supply
// their own (avoids collision risk + keeps URLs stable).
var ulidEntropy = ulid.Monotonic(rand.Reader, 0)

func generateConfigID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
