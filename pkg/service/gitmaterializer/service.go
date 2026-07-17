package gitmaterializer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/go-chi/chi/v5"
)

type Service struct {
	mat      *Materializer
	events   domain.EventBus
	logger   *slog.Logger
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
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

	s.logger.Info("gitmaterializer service started", "poll_interval", s.interval)
	return nil
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
