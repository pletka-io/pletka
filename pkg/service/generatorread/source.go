// Package generatorread owns generator assembly lifetimes. Its output contains
// no lazy readers, so rendering cannot retain a database transaction.
package generatorread

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/projectread"
)

// Source combines the configured session policy and bounded assembler.
type Source struct {
	sessions  *readsession.Factory
	assembler *projectread.Assembler
}

// New binds configuration. Callers authorize the root, and the factory checks
// every source project while traversing its recorded release dependencies.
func New(sessions *readsession.Factory, options projectread.Options) (*Source, error) {
	if sessions == nil {
		return nil, fmt.Errorf("generator source requires sessions")
	}
	assembler, err := projectread.New(options)
	if err != nil {
		return nil, err
	}
	return &Source{sessions: sessions, assembler: assembler}, nil
}

// Entity returns a bounded detached selection after its session has closed.
func (s *Source) Entity(ctx context.Context, scope auth.ReadScope, project, kind, id string) (projectsnapshot.EntityFragment, error) {
	var out projectsnapshot.EntityFragment
	if s == nil || s.sessions == nil || s.assembler == nil {
		return out, fmt.Errorf("generator source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(session *readsession.Session) error {
		var err error
		out, err = s.assembler.Entity(ctx, session, kind, id)
		return err
	})
	if err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	return out, nil
}

// Project loads one complete value for a bulk operation; all outputs must reuse it.
func (s *Source) Project(ctx context.Context, scope auth.ReadScope, project string) (projectsnapshot.CompleteProject, error) {
	var out projectsnapshot.CompleteProject
	if s == nil || s.sessions == nil || s.assembler == nil {
		return out, fmt.Errorf("generator source is uninitialized")
	}
	err := s.sessions.Read(ctx, scope, project, func(session *readsession.Session) error {
		var err error
		out, err = s.assembler.Project(ctx, session)
		return err
	})
	if err != nil {
		return projectsnapshot.CompleteProject{}, err
	}
	return out, nil
}
