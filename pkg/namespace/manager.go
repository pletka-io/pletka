package namespace

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNamespaceNotFound = errors.New("namespace not found")
	ErrNamespaceNotValid = errors.New("namespace not valid: prefix and base URI are required")
)

// Namespace is one prefix-to-base-URI binding.
type Namespace struct {
	Prefix string
	URI    string
	Weight int
}

// Manager provides bidirectional prefix and namespace-base lookup.
type Manager interface {
	GetWithPrefix(prefix string) (*Namespace, error)
	GetWithBase(base string) (*Namespace, error)
	GetSearchLabel(uri string) (string, error)
	Expand(input string) (string, error)
	Put(prefix, base string, weight int) (*Namespace, error)
	Prefixes() map[string]string
}

// SplitURI splits a URI into base namespace and local name.
// It splits on last "#" first, then last "/".
func SplitURI(uri string) (base, localName string) {
	if uri == "" {
		return "", ""
	}
	if idx := strings.LastIndex(uri, "#"); idx >= 0 {
		return uri[:idx+1], uri[idx+1:]
	}
	if idx := strings.LastIndex(uri, "/"); idx >= 0 {
		return uri[:idx+1], uri[idx+1:]
	}
	return "", uri
}

type store struct {
	mu       sync.RWMutex
	byPrefix map[string][]*Namespace
	byBase   map[string][]*Namespace
}

func NewStore() Manager {
	return &store{
		byPrefix: make(map[string][]*Namespace),
		byBase:   make(map[string][]*Namespace),
	}
}

func (s *store) Put(prefix, base string, weight int) (*Namespace, error) {
	if prefix == "" || base == "" {
		return nil, ErrNamespaceNotValid
	}

	ns := &Namespace{
		Prefix: prefix,
		URI:    base,
		Weight: weight,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.byPrefix[prefix] {
		if existing.URI == base {
			if existing.Weight != weight {
				existing.Weight = weight
				s.sortLocked()
			}
			return existing, nil
		}
	}

	if ns.Weight == 0 {
		ns.Weight = len(s.byPrefix[prefix]) + len(s.byBase[base]) + 1
	}

	s.byPrefix[prefix] = append(s.byPrefix[prefix], ns)
	s.byBase[base] = append(s.byBase[base], ns)
	s.sortLocked()

	return ns, nil
}

func (s *store) GetWithPrefix(prefix string) (*Namespace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.byPrefix[prefix]
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: prefix %q", ErrNamespaceNotFound, prefix)
	}
	return entries[0], nil
}

func (s *store) GetWithBase(base string) (*Namespace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.byBase[base]
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: base %q", ErrNamespaceNotFound, base)
	}
	return entries[0], nil
}

func (s *store) GetSearchLabel(uri string) (string, error) {
	base, local := SplitURI(uri)
	if base == "" {
		return uri, nil
	}

	ns, err := s.GetWithBase(base)
	if err != nil {
		return "", fmt.Errorf("cannot find namespace for %q: %w", uri, err)
	}

	return ns.Prefix + "_" + local, nil
}

func (s *store) Expand(input string) (string, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, nil
	}

	prefix, local, ok := strings.Cut(input, ":")
	if !ok {
		return input, nil
	}

	ns, err := s.GetWithPrefix(prefix)
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %w", input, err)
	}

	return ns.URI + local, nil
}

func (s *store) Prefixes() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]string, len(s.byPrefix))
	for prefix, entries := range s.byPrefix {
		if len(entries) > 0 {
			result[prefix] = entries[0].URI
		}
	}
	return result
}

func (s *store) sortLocked() {
	for _, entries := range s.byPrefix {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Weight > entries[j].Weight
		})
	}
	for _, entries := range s.byBase {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Weight > entries[j].Weight
		})
	}
}
