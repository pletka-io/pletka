package domain

// QueryOption is a functional option for composing store queries.
type QueryOption func(*QueryConfig)

// QueryConfig holds common query parameters used by store slices.
type QueryConfig struct {
	Filters       map[string]any
	Search        string
	SearchColumns []string
	Limit         int
	Offset        int
	OrderBy       string
	OrderDesc     bool
	ProjectID     string
	Version       string
	IncludeDrafts bool
}

// ApplyOptions applies all query options to a new QueryConfig.
func ApplyOptions(opts []QueryOption) *QueryConfig {
	cfg := &QueryConfig{Limit: 100}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithFilter adds a key-value filter to the query.
func WithFilter(key string, value any) QueryOption {
	return func(cfg *QueryConfig) {
		if cfg.Filters == nil {
			cfg.Filters = make(map[string]any)
		}
		cfg.Filters[key] = value
	}
}

// WithProjectID scopes the query to a project.
func WithProjectID(id string) QueryOption {
	return func(cfg *QueryConfig) {
		cfg.ProjectID = id
	}
}

// WithVersion scopes the query to a released version snapshot when non-empty.
func WithVersion(version string) QueryOption {
	return func(cfg *QueryConfig) {
		cfg.Version = version
	}
}
