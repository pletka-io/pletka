package cliruntime

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestApplyLockConnFloor pins the exact bug the whole-branch review found:
// applyLockConnFloor must raise an unset (defaulted) pool size up to the
// floor, but leave an operator's own explicit pool_max_conns alone on
// either side of the floor — raising an explicit 8 to 16 would silently
// override a deliberate lower cap (e.g. an operator fitting several
// instances into one database's connection limit).
//
// The "no pool_max_conns set" case can't assert an exact want=lockConnFloor:
// pgx's own default is max(4, runtime.NumCPU()), which on a many-core test
// machine can already sit above the floor, and that default deserves the
// same "don't override it" treatment an explicit setting gets — the floor
// only needs to raise a default that would otherwise fall below it. So that
// case asserts the result is the greater of the floor and pgx's own
// (unfloored) default, computed by parsing the same string a second time.
func TestApplyLockConnFloor(t *testing.T) {
	noExplicitSetting := "host=db.example port=5432 user=u password=p dbname=d sslmode=disable"
	unflooredDefault, err := pgxpool.ParseConfig(noExplicitSetting)
	if err != nil {
		t.Fatalf("ParseConfig(%q): %v", noExplicitSetting, err)
	}
	wantDefault := unflooredDefault.MaxConns
	if wantDefault < lockConnFloor {
		wantDefault = lockConnFloor
	}

	tests := []struct {
		name       string
		connString string
		want       int32
	}{
		{
			name:       "no pool_max_conns set: raised to at least the floor",
			connString: noExplicitSetting,
			want:       wantDefault,
		},
		{
			name:       "explicit pool_max_conns below the floor: kept as-is",
			connString: "host=db.example port=5432 user=u password=p dbname=d sslmode=disable pool_max_conns=8",
			want:       8,
		},
		{
			name:       "explicit pool_max_conns above the floor: kept as-is",
			connString: "host=db.example port=5432 user=u password=p dbname=d sslmode=disable pool_max_conns=32",
			want:       32,
		},
		{
			name:       "explicit pool_max_conns as a URL query param below the floor: kept as-is",
			connString: "postgres://u:p@db.example:5432/d?sslmode=disable&pool_max_conns=8",
			want:       8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := pgxpool.ParseConfig(tt.connString)
			if err != nil {
				t.Fatalf("ParseConfig(%q): %v", tt.connString, err)
			}
			applyLockConnFloor(cfg, tt.connString)
			if cfg.MaxConns != tt.want {
				t.Fatalf("MaxConns = %d, want %d", cfg.MaxConns, tt.want)
			}
		})
	}
}
