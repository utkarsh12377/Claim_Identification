package memory

import (
	"context"
	"sync"
	"time"

	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
)

type Store struct {
	mu        sync.RWMutex
	products  map[string]*model.Product
	workflows map[string]*model.Workflow
}

func New() *Store {
	return &Store{
		products:  make(map[string]*model.Product),
		workflows: make(map[string]*model.Workflow),
	}
}

var _ store.Store = (*Store)(nil)

func (s *Store) UpsertProduct(_ context.Context, p *model.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.products[p.ID] = p.Clone()
	return nil
}

func (s *Store) GetProduct(_ context.Context, id string) (*model.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.products[id]
	if !ok {
		return nil, store.ErrProductNotFound
	}
	return p.Clone(), nil
}

func (s *Store) CreateWorkflow(_ context.Context, w *model.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workflows[w.ID] = w.Clone()
	return nil
}

func (s *Store) GetWorkflow(_ context.Context, id string) (*model.Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	w, ok := s.workflows[id]
	if !ok {
		return nil, store.ErrWorkflowNotFound
	}
	return w.Clone(), nil
}

func (s *Store) CompleteWorkflow(_ context.Context, w *model.Workflow, enriched *model.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.workflows[w.ID]
	if !ok {
		return store.ErrWorkflowNotFound
	}
	if !current.Status.CanTransitionTo(model.WorkflowStatusCompleted) {
		return store.ErrInvalidTransition
	}

	s.products[enriched.ID] = enriched.Clone()

	stored := w.Clone()
	stored.Status = model.WorkflowStatusCompleted
	stored.Product = enriched.Clone()
	stored.UpdatedAt = time.Now().UTC()
	completedAt := stored.UpdatedAt
	stored.CompletedAt = &completedAt
	s.workflows[w.ID] = stored

	return nil
}

func (s *Store) FailWorkflow(_ context.Context, w *model.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.workflows[w.ID]
	if !ok {
		return store.ErrWorkflowNotFound
	}
	if !current.Status.CanTransitionTo(model.WorkflowStatusFailed) {
		return store.ErrInvalidTransition
	}

	stored := w.Clone()
	stored.Status = model.WorkflowStatusFailed
	stored.UpdatedAt = time.Now().UTC()
	completedAt := stored.UpdatedAt
	stored.CompletedAt = &completedAt
	s.workflows[w.ID] = stored

	return nil
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Close() error { return nil }
