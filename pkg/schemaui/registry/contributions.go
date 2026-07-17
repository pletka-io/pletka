package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/schemaui/scope"
)

type Surface string

const (
	SurfacePublic   Surface = "public"
	SurfaceAdmin    Surface = "admin"
	SurfaceSettings Surface = "settings"
)

type MountSpec struct {
	ID           string
	Surface      Surface
	Pattern      string
	Capabilities []string
	Mount        func(chi.Router)
}

type AdminSection struct {
	ID           string
	Label        domain.Localizable
	URL          func(scope.Scope) string
	Capabilities []string
	Order        int
}

type SettingsPanel struct {
	ID           string
	Label        domain.Localizable
	URL          func(scope.Scope) string
	Capabilities []string
	Order        int
}

type WidgetContribution struct {
	Kind      string
	Component string
}

// SchemaContribution declares schema providers contributed by a slice.
// EntityType is the stable schema entity key, such as "category" or "model".
type SchemaContribution struct {
	EntityType string
	List       EntityListProvider
	Form       FormProvider
}

// GeneratorContribution declares a generator or derivative renderer a slice
// knows how to provide. The Descriptor is intentionally untyped so generator
// packages can define richer contracts without creating package cycles.
type GeneratorContribution struct {
	ID           string
	Label        domain.Localizable
	Capabilities []string
	Descriptor   any
}

// Contributions is the set of app surfaces a slice contributes. Registration
// means the binary knows these contributions exist; activation is a separate
// host decision.
type Contributions struct {
	Mounts        []MountSpec
	AdminSections []AdminSection
	Settings      []SettingsPanel
	Widgets       []WidgetContribution
	Schemas       []SchemaContribution
	Generators    []GeneratorContribution
}

// Empty reports whether no contributions are present.
func (c Contributions) Empty() bool {
	return len(c.Mounts) == 0 &&
		len(c.AdminSections) == 0 &&
		len(c.Settings) == 0 &&
		len(c.Widgets) == 0 &&
		len(c.Schemas) == 0 &&
		len(c.Generators) == 0
}

// Append returns a new Contributions value containing c followed by other.
func (c Contributions) Append(other Contributions) Contributions {
	c.Mounts = append(c.Mounts, other.Mounts...)
	c.AdminSections = append(c.AdminSections, other.AdminSections...)
	c.Settings = append(c.Settings, other.Settings...)
	c.Widgets = append(c.Widgets, other.Widgets...)
	c.Schemas = append(c.Schemas, other.Schemas...)
	c.Generators = append(c.Generators, other.Generators...)
	return c
}

// SliceDescriptor describes one reusable slice. It declares available
// contributions only; hosts decide whether the descriptor is active for a
// deployment, org, project, or user.
type SliceDescriptor struct {
	ID            string
	Label         domain.Localizable
	Description   domain.Localizable
	Version       string
	Capabilities  []string
	Contributions Contributions
}

// DescriptorProvider is implemented by configured slices that can build and
// validate their descriptor from explicit host dependencies.
type DescriptorProvider interface {
	Descriptor() (SliceDescriptor, error)
}

// Validate checks the minimum descriptor fields required for registration.
func (d SliceDescriptor) Validate() error {
	if d.ID == "" {
		return errors.New("slice descriptor id is required")
	}
	return nil
}

// ActivationContext describes where and for whom a contribution is being
// resolved. Hosts can use this to enforce deployment config, project/org
// settings, user rights, entitlements, or paid feature gates.
type ActivationContext struct {
	Surface Surface
	Scope   scope.Scope

	ActorID   string
	OrgID     string
	ProjectID string

	Features     map[string]bool
	Entitlements map[string]bool
}

// ActivationDecision is the result of a host activation check.
type ActivationDecision struct {
	Active bool
	Reason string
}

// ActivationPolicy decides whether a registered slice is active in a context.
type ActivationPolicy interface {
	IsActive(context.Context, SliceDescriptor, ActivationContext) (ActivationDecision, error)
}

// ActivationPolicyFunc adapts a function into an ActivationPolicy.
type ActivationPolicyFunc func(context.Context, SliceDescriptor, ActivationContext) (ActivationDecision, error)

// IsActive implements ActivationPolicy.
func (f ActivationPolicyFunc) IsActive(ctx context.Context, d SliceDescriptor, req ActivationContext) (ActivationDecision, error) {
	return f(ctx, d, req)
}

// AlwaysActive returns an activation policy that enables every descriptor.
func AlwaysActive() ActivationPolicy {
	return ActivationPolicyFunc(func(context.Context, SliceDescriptor, ActivationContext) (ActivationDecision, error) {
		return ActivationDecision{Active: true}, nil
	})
}

// SliceRegistry stores descriptors known to the binary. It does not decide
// activation; callers pass an ActivationPolicy when resolving contributions.
type SliceRegistry struct {
	descriptors map[string]SliceDescriptor
	order       []string
}

// NewSliceRegistry constructs a registry and registers descriptors in order.
func NewSliceRegistry(descriptors ...SliceDescriptor) (*SliceRegistry, error) {
	r := &SliceRegistry{descriptors: map[string]SliceDescriptor{}}
	for _, descriptor := range descriptors {
		if err := r.Register(descriptor); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// NewSliceRegistryFromProviders constructs a registry from configured slice
// providers. Provider validation errors fail registration.
func NewSliceRegistryFromProviders(providers ...DescriptorProvider) (*SliceRegistry, error) {
	r := &SliceRegistry{descriptors: map[string]SliceDescriptor{}}
	for _, provider := range providers {
		if err := r.RegisterProvider(provider); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds one descriptor to the registry.
func (r *SliceRegistry) Register(descriptor SliceDescriptor) error {
	if r == nil {
		return errors.New("slice registry is nil")
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if r.descriptors == nil {
		r.descriptors = map[string]SliceDescriptor{}
	}
	if _, exists := r.descriptors[descriptor.ID]; exists {
		return fmt.Errorf("slice descriptor %q already registered", descriptor.ID)
	}
	r.descriptors[descriptor.ID] = descriptor
	r.order = append(r.order, descriptor.ID)
	return nil
}

// RegisterProvider builds and registers a descriptor from a configured slice.
func (r *SliceRegistry) RegisterProvider(provider DescriptorProvider) error {
	if provider == nil {
		return errors.New("slice descriptor provider is nil")
	}
	descriptor, err := provider.Descriptor()
	if err != nil {
		return err
	}
	return r.Register(descriptor)
}

// Descriptor returns a registered descriptor by ID.
func (r *SliceRegistry) Descriptor(id string) (SliceDescriptor, bool) {
	if r == nil {
		return SliceDescriptor{}, false
	}
	descriptor, ok := r.descriptors[id]
	return descriptor, ok
}

// Descriptors returns registered descriptors in registration order.
func (r *SliceRegistry) Descriptors() []SliceDescriptor {
	if r == nil {
		return nil
	}
	out := make([]SliceDescriptor, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.descriptors[id])
	}
	return out
}

// ActiveDescriptors returns descriptors enabled by policy in registration
// order. nil policy means AlwaysActive.
func (r *SliceRegistry) ActiveDescriptors(ctx context.Context, req ActivationContext, policy ActivationPolicy) ([]SliceDescriptor, error) {
	if r == nil {
		return nil, nil
	}
	if policy == nil {
		policy = AlwaysActive()
	}
	active := make([]SliceDescriptor, 0, len(r.order))
	for _, descriptor := range r.Descriptors() {
		decision, err := policy.IsActive(ctx, descriptor, req)
		if err != nil {
			return nil, err
		}
		if decision.Active {
			active = append(active, descriptor)
		}
	}
	return active, nil
}

// ActiveContributions aggregates contributions from active descriptors.
func (r *SliceRegistry) ActiveContributions(ctx context.Context, req ActivationContext, policy ActivationPolicy) (Contributions, error) {
	descriptors, err := r.ActiveDescriptors(ctx, req, policy)
	if err != nil {
		return Contributions{}, err
	}
	var out Contributions
	for _, descriptor := range descriptors {
		out = out.Append(descriptor.Contributions)
	}
	sort.SliceStable(out.AdminSections, func(i, j int) bool {
		return out.AdminSections[i].Order < out.AdminSections[j].Order
	})
	sort.SliceStable(out.Settings, func(i, j int) bool { return out.Settings[i].Order < out.Settings[j].Order })
	return out, nil
}
