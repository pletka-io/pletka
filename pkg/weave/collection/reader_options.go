package collection

import (
	"fmt"
	"math"

	"github.com/pletka-io/pletka/pkg/domain"
)

type boundCollectionOptions struct {
	*domain.QueryConfig
	limit, offset int32
}

func boundListOptions(projectID, version string, opts []domain.QueryOption) (*boundCollectionOptions, error) {
	cfg := domain.ApplyOptions(opts)
	if (cfg.ProjectID != "" && cfg.ProjectID != projectID) || (cfg.Version != "" && cfg.Version != version) {
		return nil, fmt.Errorf("collection options cannot retarget a bound reader")
	}
	limit, offset := cfg.Limit, cfg.Offset
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if limit > math.MaxInt32 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("collection pagination exceeds the database range")
	}
	return &boundCollectionOptions{QueryConfig: cfg, limit: int32(limit), offset: int32(offset)}, nil
}
