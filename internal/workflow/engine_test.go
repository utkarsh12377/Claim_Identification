package workflow_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
	"github.com/utkarsh/claim-identification/internal/store/memory"
	"github.com/utkarsh/claim-identification/internal/workflow"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sampleProduct(t *testing.T) *model.Product {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "seed", "product.json"))
	if err != nil {
		t.Fatalf("read seed product: %v", err)
	}
	p, err := model.ParseProductDocument(raw)
	if err != nil {
		t.Fatalf("parse seed product: %v", err)
	}
	return p
}

func newEngine(t *testing.T, st store.Store, cfg workflow.Config) *workflow.Engine {
	t.Helper()

	engine := workflow.New(st, claims.New(), cfg, discardLogger())
	engine.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engine.Shutdown(ctx); err != nil {
			t.Errorf("shutdown engine: %v", err)
		}
	})
	return engine
}

func seededStore(t *testing.T) *memory.Store {
	t.Helper()

	st := memory.New()
	if err := st.UpsertProduct(context.Background(), sampleProduct(t)); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return st
}

func waitFor(t *testing.T, engine *workflow.Engine, workflowID string) *model.Workflow {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		wf, err := engine.Status(context.Background(), workflowID)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if wf.Status.Terminal() {
			return wf
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("workflow %s did not finish in time", workflowID)
	return nil
}

func TestEngineCompletesRun(t *testing.T) {
	st := seededStore(t)
	engine := newEngine(t, st, workflow.Config{Workers: 2})

	product := sampleProduct(t)
	wf, err := engine.Trigger(context.Background(), product.ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if wf.Status != model.WorkflowStatusInProgress {
		t.Errorf("trigger status = %s, want IN_PROGRESS", wf.Status)
	}

	done := waitFor(t, engine, wf.ID)
	if done.Status != model.WorkflowStatusCompleted {
		t.Fatalf("final status = %s (error %q), want COMPLETED", done.Status, done.Error)
	}
	if done.Product == nil {
		t.Fatal("completed workflow has no product snapshot")
	}
	if len(done.Product.Claims) != 5 {
		t.Errorf("claims = %d, want 5", len(done.Product.Claims))
	}
	if done.CompletedAt == nil {
		t.Error("completedAt not set on a completed run")
	}

	wantSteps := []string{model.StepFetchProduct, model.StepDetectClaims, model.StepPersistResult}
	if len(done.Steps) != len(wantSteps) {
		t.Fatalf("steps = %d, want %d", len(done.Steps), len(wantSteps))
	}
	for i, name := range wantSteps {
		if done.Steps[i].Name != name || done.Steps[i].Status != model.StepStatusSucceeded {
			t.Errorf("step[%d] = (%s, %s), want (%s, SUCCEEDED)",
				i, done.Steps[i].Name, done.Steps[i].Status, name)
		}
	}

	stored, err := st.GetProduct(context.Background(), product.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if len(stored.Claims) != 5 {
		t.Errorf("stored product claims = %d, want 5", len(stored.Claims))
	}
	if stored.Title != product.Title || len(stored.Media) != len(product.Media) {
		t.Error("enrichment altered the original product document")
	}
}

func TestTriggerUnknownProduct(t *testing.T) {
	engine := newEngine(t, seededStore(t), workflow.Config{Workers: 1})

	_, err := engine.Trigger(context.Background(), "missing-product")
	if !errors.Is(err, store.ErrProductNotFound) {
		t.Errorf("error = %v, want ErrProductNotFound", err)
	}
}

type failingStore struct {
	*memory.Store
	reads int
}

func (s *failingStore) GetProduct(ctx context.Context, id string) (*model.Product, error) {
	s.reads++
	if s.reads > 1 {
		return nil, errors.New("database is on fire")
	}
	return s.Store.GetProduct(ctx, id)
}

func TestEngineRecordsFailure(t *testing.T) {
	st := &failingStore{Store: seededStore(t)}
	engine := newEngine(t, st, workflow.Config{Workers: 1})

	wf, err := engine.Trigger(context.Background(), sampleProduct(t).ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}

	done := waitFor(t, engine, wf.ID)
	if done.Status != model.WorkflowStatusFailed {
		t.Fatalf("final status = %s, want FAILED", done.Status)
	}
	if done.Error == "" {
		t.Error("failed workflow has no error message")
	}
	if done.Product != nil {
		t.Error("failed workflow should not carry a product snapshot")
	}
	if len(done.Steps) == 0 || done.Steps[0].Status != model.StepStatusFailed {
		t.Errorf("expected a failed FETCH_PRODUCT step, got %+v", done.Steps)
	}
}

func TestTriggerRejectedWhenQueueIsFull(t *testing.T) {
	st := seededStore(t)

	engine := workflow.New(st, claims.New(), workflow.Config{Workers: 1, QueueSize: 1}, discardLogger())

	productID := sampleProduct(t).ID
	if _, err := engine.Trigger(context.Background(), productID); err != nil {
		t.Fatalf("first trigger: %v", err)
	}

	wf, err := engine.Trigger(context.Background(), productID)
	if !errors.Is(err, workflow.ErrQueueFull) {
		t.Fatalf("second trigger error = %v, want ErrQueueFull", err)
	}
	if wf != nil {
		t.Error("no workflow should be returned when the queue is full")
	}
}

func TestTriggerAfterShutdown(t *testing.T) {
	st := seededStore(t)
	engine := workflow.New(st, claims.New(), workflow.Config{Workers: 1}, discardLogger())
	engine.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := engine.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if err := engine.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}

	if _, err := engine.Trigger(context.Background(), sampleProduct(t).ID); !errors.Is(err, workflow.ErrShuttingDown) {
		t.Errorf("trigger after shutdown = %v, want ErrShuttingDown", err)
	}
}

func TestStatusUnknownWorkflow(t *testing.T) {
	engine := newEngine(t, seededStore(t), workflow.Config{Workers: 1})

	if _, err := engine.Status(context.Background(), "wf-nope"); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Errorf("error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestCompleteWorkflowRejectsSecondClose(t *testing.T) {
	ctx := context.Background()
	st := seededStore(t)
	product := sampleProduct(t)

	wf := &model.Workflow{
		ID:        "wf-test",
		ProductID: product.ID,
		Status:    model.WorkflowStatusInProgress,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := st.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := st.CompleteWorkflow(ctx, wf, product); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if err := st.CompleteWorkflow(ctx, wf, product); !errors.Is(err, store.ErrInvalidTransition) {
		t.Errorf("second completion = %v, want ErrInvalidTransition", err)
	}
}

func TestWorkflowStateMachine(t *testing.T) {
	cases := []struct {
		from, to model.WorkflowStatus
		want     bool
	}{
		{model.WorkflowStatusInProgress, model.WorkflowStatusCompleted, true},
		{model.WorkflowStatusInProgress, model.WorkflowStatusFailed, true},
		{model.WorkflowStatusInProgress, model.WorkflowStatusInProgress, false},
		{model.WorkflowStatusCompleted, model.WorkflowStatusFailed, false},
		{model.WorkflowStatusFailed, model.WorkflowStatusCompleted, false},
	}

	for _, tc := range cases {
		if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
			t.Errorf("%s -> %s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
