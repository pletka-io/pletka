package gitmaterializer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pletka-io/pletka/pkg/domain"
)

// defaultSweepInterval is how often the nightly reconcile sweep runs when
// sweepInterval is left unset (zero value).
const defaultSweepInterval = 24 * time.Hour

type Service struct {
	mat           *Materializer
	events        domain.EventBus
	logger        *slog.Logger
	interval      time.Duration
	sweepInterval time.Duration
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewService(mat *Materializer, events domain.EventBus, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		mat:      mat,
		events:   events,
		logger:   logger.With("service", "gitmaterializer"),
		interval: 5 * time.Second,
	}
}

// SetSweepInterval overrides the nightly reconcile sweep's tick interval
// (default 24h). Intended for tests; call before Start.
func (s *Service) SetSweepInterval(d time.Duration) {
	s.sweepInterval = d
}

func (s *Service) Name() string { return "gitmaterializer" }

func (s *Service) Routes(r chi.Router) {}

func (s *Service) Start(ctx context.Context) error {
	workCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	runner := newGitRunner("", s.logger)
	if err := runner.Available(workCtx); err != nil {
		s.logger.Error("git not available, materializer will not start", "err", err)
		cancel()
		return nil
	}

	if s.events != nil {
		s.events.Subscribe("ChangeSetClosed", func(ctx context.Context, event domain.Event) {
			if _, err := s.mat.ProcessPending(ctx, 10); err != nil {
				s.logger.Error("failed to process pending after event", "err", err)
			}
		})
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				if _, err := s.mat.ProcessPending(workCtx, 10); err != nil {
					s.logger.Error("background poll failed", "err", err)
				}
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runSweep(workCtx)
	}()

	s.logger.Info("gitmaterializer service started", "poll_interval", s.interval, "sweep_interval", s.sweepIntervalOrDefault())
	return nil
}

// sweepIntervalOrDefault returns the configured sweep interval, falling back
// to defaultSweepInterval when unset.
func (s *Service) sweepIntervalOrDefault() time.Duration {
	if s.sweepInterval <= 0 {
		return defaultSweepInterval
	}
	return s.sweepInterval
}

// runSweep periodically reconciles every project's materialized tree
// against current DB state, self-healing any closure gap the scoped hot
// path missed. This is the backstop's backstop: Reconcile (Task 6) already
// rebuilds one project on demand; runSweep makes sure it runs for every
// project at least once per sweep interval (default 24h), not just when a
// human or CLI happens to invoke it.
func (s *Service) runSweep(ctx context.Context) {
	ticker := time.NewTicker(s.sweepIntervalOrDefault())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOnce(ctx)
		}
	}
}

// sweepOnce runs one reconcile pass over every project, logging per-project
// failures and drift without letting either abort the rest of the sweep.
func (s *Service) sweepOnce(ctx context.Context) {
	ids, err := s.mat.projectIDsWithRepos(ctx)
	if err != nil {
		s.logger.Error("reconcile sweep: list projects", "err", err)
		return
	}
	for _, id := range ids {
		changed, err := s.mat.Reconcile(ctx, id)
		if err != nil {
			s.logger.Error("reconcile sweep", "project_id", id, "err", err)
			continue
		}
		if changed > 0 {
			s.logger.Warn("reconcile sweep healed drift", "project_id", id, "changed", changed)
		}
	}
}

func (s *Service) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
