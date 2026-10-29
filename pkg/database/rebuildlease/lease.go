// Package rebuildlease coordinates explicit release maintenance with runtimes.
package rebuildlease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Runtime holds a shared lease until the returned cleanup is called. A durable
// unfinished operation keeps runtimes offline even after the tool process exits.
func Runtime(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	return acquire(ctx, pool, false)
}

// Rebuild excludes running servers and materializers for the entire operation.
func Rebuild(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	return acquire(ctx, pool, true)
}

func acquire(ctx context.Context, pool *pgxpool.Pool, exclusive bool) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	q := sqlcgen.New(conn)
	var ok bool
	if exclusive {
		ok, err = q.WeaveTryRebuildLease(ctx)
	} else {
		ok, err = q.WeaveTryRuntimeLease(ctx)
	}
	if err != nil || !ok {
		conn.Release()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("release maintenance lease unavailable; stop runtimes/materializers before rebuilding")
	}
	closeLease := func() {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		if exclusive {
			err = q.WeaveReleaseRebuildLease(bg)
		} else {
			err = q.WeaveReleaseRuntimeLease(bg)
		}
		if err != nil {
			_ = conn.Conn().Close(bg)
		}
		conn.Release()
	}
	if !exclusive {
		pending, err := q.WeaveRebuildPending(ctx)
		if err != nil || pending {
			closeLease()
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("release rebuild is unfinished; resume the recorded operation before starting the runtime")
		}
	}
	return closeLease, nil
}
