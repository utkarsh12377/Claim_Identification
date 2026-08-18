package model

import "time"

type WorkflowStatus string

const (
	WorkflowStatusInProgress WorkflowStatus = "IN_PROGRESS"
	WorkflowStatusCompleted  WorkflowStatus = "COMPLETED"
	WorkflowStatusFailed     WorkflowStatus = "FAILED"
)

func (s WorkflowStatus) Terminal() bool {
	return s == WorkflowStatusCompleted || s == WorkflowStatusFailed
}

// IN_PROGRESS -> COMPLETED | FAILED. Terminal states are final.
func (s WorkflowStatus) CanTransitionTo(next WorkflowStatus) bool {
	if s == next {
		return false
	}
	if s == WorkflowStatusInProgress {
		return next == WorkflowStatusCompleted || next == WorkflowStatusFailed
	}
	return false
}

const (
	StepStatusSucceeded = "SUCCEEDED"
	StepStatusFailed    = "FAILED"
)

const (
	StepFetchProduct  = "FETCH_PRODUCT"
	StepDetectClaims  = "DETECT_CLAIMS"
	StepPersistResult = "PERSIST_RESULT"
)

type WorkflowStep struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt"`
	DurationMs int64     `json:"durationMs"`
	Detail     string    `json:"detail,omitempty"`
}

type Workflow struct {
	ID          string         `json:"workflowId"`
	ProductID   string         `json:"productId"`
	Status      WorkflowStatus `json:"status"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	CompletedAt *time.Time     `json:"completedAt,omitempty"`
	Product     *Product       `json:"product,omitempty"`
}

func (w *Workflow) RecordStep(name string, startedAt, endedAt time.Time, detail string, err error) {
	step := WorkflowStep{
		Name:       name,
		Status:     StepStatusSucceeded,
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		DurationMs: endedAt.Sub(startedAt).Milliseconds(),
		Detail:     detail,
	}
	if err != nil {
		step.Status = StepStatusFailed
		step.Detail = err.Error()
	}
	w.Steps = append(w.Steps, step)
}

func (w *Workflow) Clone() *Workflow {
	if w == nil {
		return nil
	}
	out := *w
	out.Steps = append([]WorkflowStep(nil), w.Steps...)
	out.Product = w.Product.Clone()
	if w.CompletedAt != nil {
		completedAt := *w.CompletedAt
		out.CompletedAt = &completedAt
	}
	return &out
}
