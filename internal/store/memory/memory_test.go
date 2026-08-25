package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
)

func testProduct() *model.Product {
	return &model.Product{
		ID:               "prod-1",
		SKU:              "SKU-1",
		Title:            "Cold pressed olive oil",
		Brand:            "Acme",
		ShortDescription: "Clinically proven to lower cholesterol.",
		AboutItems:       []string{"100% organic", "Made in Italy"},
		Attributes:       map[string]any{"organic": true},
	}
}

func inProgressWorkflow() *model.Workflow {
	return &model.Workflow{
		ID:        "wf-1",
		ProductID: "prod-1",
		Status:    model.WorkflowStatusInProgress,
	}
}

func TestUpsertAndGetProduct(t *testing.T) {
	ctx := context.Background()
	s := New()

	want := testProduct()
	if err := s.UpsertProduct(ctx, want); err != nil {
		t.Fatalf("UpsertProduct() returned error: %v", err)
	}

	got, err := s.GetProduct(ctx, "prod-1")
	if err != nil {
		t.Fatalf("GetProduct() returned error: %v", err)
	}
	if got.Title != want.Title || got.SKU != want.SKU {
		t.Errorf("GetProduct() = %+v, want title %q and sku %q", got, want.Title, want.SKU)
	}
}

func TestUpsertProductReplacesPrevious(t *testing.T) {
	ctx := context.Background()
	s := New()

	if err := s.UpsertProduct(ctx, testProduct()); err != nil {
		t.Fatalf("UpsertProduct() returned error: %v", err)
	}

	updated := testProduct()
	updated.Title = "Extra virgin olive oil"
	if err := s.UpsertProduct(ctx, updated); err != nil {
		t.Fatalf("UpsertProduct() returned error: %v", err)
	}

	got, err := s.GetProduct(ctx, "prod-1")
	if err != nil {
		t.Fatalf("GetProduct() returned error: %v", err)
	}
	if got.Title != "Extra virgin olive oil" {
		t.Errorf("Title = %q, want the updated title", got.Title)
	}
}

// The store hands out copies, so a caller mutating a result must not be able
// to corrupt what is held internally.
func TestStoreReturnsIndependentCopies(t *testing.T) {
	ctx := context.Background()
	s := New()

	if err := s.UpsertProduct(ctx, testProduct()); err != nil {
		t.Fatalf("UpsertProduct() returned error: %v", err)
	}

	first, err := s.GetProduct(ctx, "prod-1")
	if err != nil {
		t.Fatalf("GetProduct() returned error: %v", err)
	}
	first.Title = "mutated"
	first.AboutItems[0] = "mutated"

	second, err := s.GetProduct(ctx, "prod-1")
	if err != nil {
		t.Fatalf("GetProduct() returned error: %v", err)
	}
	if second.Title == "mutated" {
		t.Error("mutating a returned product changed the stored copy")
	}
	if second.AboutItems[0] == "mutated" {
		t.Error("mutating a returned about item changed the stored copy")
	}
}

func TestGetProductNotFound(t *testing.T) {
	_, err := New().GetProduct(context.Background(), "missing")
	if !errors.Is(err, store.ErrProductNotFound) {
		t.Fatalf("GetProduct() error = %v, want ErrProductNotFound", err)
	}
}

func TestCreateAndGetWorkflow(t *testing.T) {
	ctx := context.Background()
	s := New()

	if err := s.CreateWorkflow(ctx, inProgressWorkflow()); err != nil {
		t.Fatalf("CreateWorkflow() returned error: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-1")
	if err != nil {
		t.Fatalf("GetWorkflow() returned error: %v", err)
	}
	if got.Status != model.WorkflowStatusInProgress {
		t.Errorf("Status = %q, want IN_PROGRESS", got.Status)
	}
	if got.ProductID != "prod-1" {
		t.Errorf("ProductID = %q, want \"prod-1\"", got.ProductID)
	}
}

func TestGetWorkflowNotFound(t *testing.T) {
	_, err := New().GetWorkflow(context.Background(), "missing")
	if !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflow() error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestCompleteWorkflowStoresEnrichedProduct(t *testing.T) {
	ctx := context.Background()
	s := New()

	wf := inProgressWorkflow()
	if err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() returned error: %v", err)
	}

	enriched := testProduct()
	enriched.Claims = []model.Claim{{
		ID:         "claim-1",
		ClaimType:  model.CategoryMedical,
		ClaimValue: "Clinically proven to lower cholesterol",
		Status:     model.ClaimStatusIdentified,
	}}

	if err := s.CompleteWorkflow(ctx, wf, enriched); err != nil {
		t.Fatalf("CompleteWorkflow() returned error: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-1")
	if err != nil {
		t.Fatalf("GetWorkflow() returned error: %v", err)
	}
	if got.Status != model.WorkflowStatusCompleted {
		t.Errorf("Status = %q, want COMPLETED", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt is nil, want a timestamp")
	}
	if got.Product == nil || len(got.Product.Claims) != 1 {
		t.Fatalf("Product claims = %+v, want exactly one claim", got.Product)
	}

	stored, err := s.GetProduct(ctx, "prod-1")
	if err != nil {
		t.Fatalf("GetProduct() returned error: %v", err)
	}
	if len(stored.Claims) != 1 {
		t.Errorf("stored product has %d claims, want 1", len(stored.Claims))
	}
}

func TestFailWorkflow(t *testing.T) {
	ctx := context.Background()
	s := New()

	wf := inProgressWorkflow()
	if err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() returned error: %v", err)
	}

	wf.Error = "product not found"
	if err := s.FailWorkflow(ctx, wf); err != nil {
		t.Fatalf("FailWorkflow() returned error: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-1")
	if err != nil {
		t.Fatalf("GetWorkflow() returned error: %v", err)
	}
	if got.Status != model.WorkflowStatusFailed {
		t.Errorf("Status = %q, want FAILED", got.Status)
	}
	if got.Error != "product not found" {
		t.Errorf("Error = %q, want the recorded failure reason", got.Error)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt is nil, want a timestamp")
	}
}

// COMPLETED and FAILED are terminal, so a second transition must be refused.
func TestTerminalWorkflowsRejectFurtherTransitions(t *testing.T) {
	ctx := context.Background()

	t.Run("complete twice", func(t *testing.T) {
		s := New()
		wf := inProgressWorkflow()
		if err := s.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow() returned error: %v", err)
		}
		if err := s.CompleteWorkflow(ctx, wf, testProduct()); err != nil {
			t.Fatalf("first CompleteWorkflow() returned error: %v", err)
		}
		err := s.CompleteWorkflow(ctx, wf, testProduct())
		if !errors.Is(err, store.ErrInvalidTransition) {
			t.Fatalf("second CompleteWorkflow() error = %v, want ErrInvalidTransition", err)
		}
	})

	t.Run("fail after complete", func(t *testing.T) {
		s := New()
		wf := inProgressWorkflow()
		if err := s.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow() returned error: %v", err)
		}
		if err := s.CompleteWorkflow(ctx, wf, testProduct()); err != nil {
			t.Fatalf("CompleteWorkflow() returned error: %v", err)
		}
		if err := s.FailWorkflow(ctx, wf); !errors.Is(err, store.ErrInvalidTransition) {
			t.Fatalf("FailWorkflow() error = %v, want ErrInvalidTransition", err)
		}
	})
}

func TestTransitionsOnUnknownWorkflow(t *testing.T) {
	ctx := context.Background()
	s := New()
	unknown := inProgressWorkflow()

	if err := s.CompleteWorkflow(ctx, unknown, testProduct()); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Errorf("CompleteWorkflow() error = %v, want ErrWorkflowNotFound", err)
	}
	if err := s.FailWorkflow(ctx, unknown); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Errorf("FailWorkflow() error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestPingAndClose(t *testing.T) {
	s := New()
	if err := s.Ping(context.Background()); err != nil {
		t.Errorf("Ping() returned error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}
