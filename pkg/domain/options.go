package domain

// QueryOption is a functional option for composing queries.
// Used by all weave sub-store interfaces (WeaveCategoryStore, FieldStore, etc.).
type QueryOption func(*QueryConfig)

// QueryConfig holds query parameters.
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

// ApplyOptions applies all query options to a new QueryConfig with sensible defaults.
func ApplyOptions(opts []QueryOption) *QueryConfig {
	cfg := &QueryConfig{Limit: 100}
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithFilter adds a key-value filter to the query.
func WithFilter(key string, value any) QueryOption {
	return func(c *QueryConfig) {
		if c.Filters == nil {
			c.Filters = make(map[string]any)
		}

		c.Filters[key] = value
	}
}

// WithSearch sets the search term for full-text queries.
func WithSearch(search string) QueryOption {
	return func(c *QueryConfig) { c.Search = search }
}

// WithSearchColumns specifies which columns the search term applies to.
// Stores may use this to select between text and JSONB search strategies.
// If unset, stores default to searching their primary name column (typically ui_name).
// Calling with zero arguments is a no-op (leaves SearchColumns nil).
func WithSearchColumns(columns ...string) QueryOption {
	return func(c *QueryConfig) {
		if len(columns) > 0 {
			c.SearchColumns = columns
		}
	}
}

// WithLimit sets the maximum number of results.
func WithLimit(limit int) QueryOption {
	return func(c *QueryConfig) { c.Limit = limit }
}

// WithOffset sets the result offset for pagination.
func WithOffset(offset int) QueryOption {
	return func(c *QueryConfig) { c.Offset = offset }
}

// WithOrderBy sets the ordering field and direction.
func WithOrderBy(field string, desc bool) QueryOption {
	return func(c *QueryConfig) {
		c.OrderBy = field
		c.OrderDesc = desc
	}
}

// WithProjectID scopes the query to a specific project.
func WithProjectID(id string) QueryOption {
	return func(c *QueryConfig) { c.ProjectID = id }
}

// WithVersion scopes the query to a released version snapshot when non-empty.
func WithVersion(version string) QueryOption {
	return func(c *QueryConfig) { c.Version = version }
}

// IncludeDrafts controls whether draft rows should be included in list results.
func IncludeDrafts(include bool) QueryOption {
	return func(c *QueryConfig) { c.IncludeDrafts = include }
}
