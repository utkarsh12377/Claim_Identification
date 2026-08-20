package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
	"github.com/utkarsh/claim-identification/internal/uid"
)

var (
	ErrQueueFull = errors.New("workflow queue is full")

	ErrShuttingDown = errors.New("workflow engine is shutting down")
)

type Config struct {
	Workers   int
	QueueSize int
	Timeout   time.Duration
}

type Engine struct {
	store    store.Store
	detector *claims.Detector
	log      *slog.Logger
	jobs     chan string
	workers  int
	timeout  time.Duration
	wg       sync.WaitGroup
	mu       sync.RWMutex
	closed   bool
}

func New(st store.Store, detector *claims.Detector, cfg Config, log *slog.Logger) *Engine {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 64
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}

	return &Engine{
		store:    st,
		detector: detector,
		log:      log,
		jobs:     make(chan string, cfg.QueueSize),
		workers:  cfg.Workers,
		timeout:  cfg.Timeout,
	}
}

func (e *Engine) Start() {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			for workflowID := range e.jobs {
				e.process(workflowID)
			}
		}()
	}
	e.log.Info("workflow engine started", "workers", e.workers, "timeout", e.timeout)
}

func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	close(e.jobs)
	e.mu.Unlock()

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.log.Info("workflow engine stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("workflow engine shutdown: %w", ctx.Err())
	}
}

func (e *Engine) Trigger(ctx context.Context, productID string) (*model.Workflow, error) {
	if _, err := e.store.GetProduct(ctx, productID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	wf := &model.Workflow{
		ID:        uid.NewWorkflowID(),
		ProductID: productID,
		Status:    model.WorkflowStatusInProgress,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := e.store.CreateWorkflow(ctx, wf); err != nil {
		return nil, err
	}

	if err := e.enqueue(ctx, wf); err != nil {
		return nil, err
	}

	e.log.Info("workflow triggered", "workflowId", wf.ID, "productId", productID)
	return wf, nil
}

func (e *Engine) enqueue(ctx context.Context, wf *model.Workflow) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return ErrShuttingDown
	}

	select {
	case e.jobs <- wf.ID:
		return nil
	default:

		wf.Error = ErrQueueFull.Error()
		if err := e.store.FailWorkflow(ctx, wf); err != nil {
			e.log.Error("mark queue-rejected workflow failed", "workflowId", wf.ID, "error", err)
		}
		return ErrQueueFull
	}
}

func (e *Engine) Status(ctx context.Context, workflowID string) (*model.Workflow, error) {
	return e.store.GetWorkflow(ctx, workflowID)
}

func (e *Engine) process(workflowID string) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			e.log.Error("workflow panicked", "workflowId", workflowID, "panic", r)
			if wf, err := e.store.GetWorkflow(ctx, workflowID); err == nil {
				e.fail(ctx, wf, fmt.Errorf("internal error: %v", r))
			}
		}
	}()

	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		e.log.Error("load workflow", "workflowId", workflowID, "error", err)
		return
	}
	if wf.Status.Terminal() {
		e.log.Warn("skipping workflow in terminal state", "workflowId", workflowID, "status", wf.Status)
		return
	}

	started := time.Now()
	product, err := e.store.GetProduct(ctx, wf.ProductID)
	wf.RecordStep(model.StepFetchProduct, started, time.Now(), "", err)
	if err != nil {
		e.fail(ctx, wf, fmt.Errorf("fetch product %s: %w", wf.ProductID, err))
		return
	}

	started = time.Now()
	detected := e.detector.Detect(product)
	wf.RecordStep(model.StepDetectClaims, started, time.Now(),
		fmt.Sprintf("%d claims identified", len(detected)), nil)
	e.logRestricted(wf, detected)

	started = time.Now()
	product.Claims = detected
	wf.RecordStep(model.StepPersistResult, started, time.Now(),
		fmt.Sprintf("%d claims persisted", len(detected)), nil)

	if err := e.store.CompleteWorkflow(ctx, wf, product); err != nil {
		wf.Steps = wf.Steps[:len(wf.Steps)-1]
		wf.RecordStep(model.StepPersistResult, started, time.Now(), "", err)

		if errors.Is(err, store.ErrInvalidTransition) {
			e.log.Warn("workflow already closed", "workflowId", wf.ID)
			return
		}
		e.fail(ctx, wf, err)
		return
	}

	e.log.Info("workflow completed",
		"workflowId", wf.ID, "productId", wf.ProductID, "claims", len(detected))
}

func (e *Engine) logRestricted(wf *model.Workflow, detected []model.Claim) {
	for _, c := range detected {
		if c.ClaimType.Restricted() {
			e.log.Warn("restricted claim detected",
				"workflowId", wf.ID,
				"productId", wf.ProductID,
				"claimType", string(c.ClaimType),
				"claimValue", c.ClaimValue)
		}
	}
}

// detached context, so a run aborted by its own timeout still records the failure
func (e *Engine) fail(ctx context.Context, wf *model.Workflow, cause error) {
	e.log.Error("workflow failed", "workflowId", wf.ID, "productId", wf.ProductID, "error", cause)

	wf.Error = cause.Error()

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := e.store.FailWorkflow(writeCtx, wf); err != nil {
		e.log.Error("persist workflow failure", "workflowId", wf.ID, "error", err)
	}
}
