// Package browserread owns the lifetime of bounded browser reads.
package browserread

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

// Source uses one configured session for content and any inverse aggregates.
// Callers authorize the current root; the factory authorizes source projects.
type Source struct {
	sessions  *readsession.Factory
	assembler *projectread.Assembler
}

// New binds the assembler and the session authorization policy.
func New(sessions *readsession.Factory, options projectread.Options) (*Source, error) {
	if sessions == nil {
		return nil, fmt.Errorf("browser source requires sessions")
	}
	a, err := projectread.New(options)
	if err != nil {
		return nil, err
	}
	return &Source{sessions: sessions, assembler: a}, nil
}

// Detail returns detached input only after closing its read session.
func (s *Source) Detail(ctx context.Context, scope auth.ReadScope, project, kind, id string) (browserinput.Detail, error) {
	var out browserinput.Detail
	if s == nil || s.sessions == nil || s.assembler == nil {
		return out, fmt.Errorf("browser source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(session *readsession.Session) error {
		var err error
		out.Fragment, err = s.assembler.Entity(ctx, session, kind, id)
		if err != nil {
			return err
		}
		if kind == "field" {
			counts, err := session.FieldUsage(ctx, id)
			if err != nil {
				return err
			}
			out.FieldUsage = &counts
		}
		return nil
	})
	if err != nil {
		return browserinput.Detail{}, err
	}
	return out, nil
}
