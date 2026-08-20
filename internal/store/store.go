package store

import (
	"context"
	"errors"

	"github.com/utkarsh/claim-identification/internal/model"
)

var (
	ErrProductNotFound = errors.New("product not found")

	ErrWorkflowNotFound = errors.New("workflow not found")

	ErrInvalidTransition = errors.New("invalid workflow state transition")
)

type Store interface {
	UpsertProduct(ctx context.Context, p *model.Product) error

	GetProduct(ctx context.Context, id string) (*model.Product, error)

	CreateWorkflow(ctx context.Context, w *model.Workflow) error

	GetWorkflow(ctx context.Context, id string) (*model.Workflow, error)

	CompleteWorkflow(ctx context.Context, w *model.Workflow, enriched *model.Product) error

	FailWorkflow(ctx context.Context, w *model.Workflow) error

	Ping(ctx context.Context) error

	Close() error
}
