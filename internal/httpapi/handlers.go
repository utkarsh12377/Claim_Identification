package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
	"github.com/utkarsh/claim-identification/internal/workflow"
)

const maxRequestBody = 64 << 10

type identifyRequest struct {
	ProductID string `json:"productId"`
}

type identifyResponse struct {
	WorkflowID string               `json:"workflowId"`
	Status     model.WorkflowStatus `json:"status"`
	ProductID  string               `json:"productId"`
}

type statusResponse struct {
	WorkflowID string               `json:"workflowId"`
	Status     model.WorkflowStatus `json:"status"`
	ProductID  string               `json:"productId"`
	Steps      []model.WorkflowStep `json:"steps,omitempty"`
	Error      string               `json:"error,omitempty"`
	Product    *model.Product       `json:"product,omitempty"`
}

func (a *API) handleIdentify(w http.ResponseWriter, r *http.Request) {
	var req identifyRequest

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		message := "request body must be JSON of the form {\"productId\": \"...\"}"
		if errors.Is(err, io.EOF) {
			message = "request body is empty"
		}
		writeError(w, a.log, http.StatusBadRequest, codeInvalidRequest, message)
		return
	}

	req.ProductID = strings.TrimSpace(req.ProductID)
	if req.ProductID == "" {
		writeError(w, a.log, http.StatusBadRequest, codeInvalidRequest, "productId is required")
		return
	}

	wf, err := a.engine.Trigger(r.Context(), req.ProductID)
	switch {
	case errors.Is(err, store.ErrProductNotFound):
		writeError(w, a.log, http.StatusNotFound, codeProductNotFound, "no product with id "+req.ProductID)
		return
	case errors.Is(err, workflow.ErrQueueFull), errors.Is(err, workflow.ErrShuttingDown):
		writeError(w, a.log, http.StatusServiceUnavailable, codeUnavailable, "service is busy, please retry")
		return
	case err != nil:
		a.log.Error("trigger workflow", "productId", req.ProductID, "error", err)
		writeError(w, a.log, http.StatusInternalServerError, codeInternal, "failed to start claim identification")
		return
	}

	writeJSON(w, a.log, http.StatusAccepted, identifyResponse{
		WorkflowID: wf.ID,
		Status:     wf.Status,
		ProductID:  wf.ProductID,
	})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	workflowID := strings.TrimSpace(r.PathValue("workflowId"))
	if workflowID == "" {
		writeError(w, a.log, http.StatusBadRequest, codeInvalidRequest, "workflowId is required")
		return
	}

	wf, err := a.engine.Status(r.Context(), workflowID)
	switch {
	case errors.Is(err, store.ErrWorkflowNotFound):
		writeError(w, a.log, http.StatusNotFound, codeWorkflowNotFound, "no workflow with id "+workflowID)
		return
	case err != nil:
		a.log.Error("read workflow", "workflowId", workflowID, "error", err)
		writeError(w, a.log, http.StatusInternalServerError, codeInternal, "failed to read workflow status")
		return
	}

	writeJSON(w, a.log, http.StatusOK, statusResponse{
		WorkflowID: wf.ID,
		Status:     wf.Status,
		ProductID:  wf.ProductID,
		Steps:      wf.Steps,
		Error:      wf.Error,
		Product:    wf.Product,
	})
}

func (a *API) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	productID := strings.TrimSpace(r.PathValue("productId"))

	product, err := a.store.GetProduct(r.Context(), productID)
	switch {
	case errors.Is(err, store.ErrProductNotFound):
		writeError(w, a.log, http.StatusNotFound, codeProductNotFound, "no product with id "+productID)
		return
	case err != nil:
		a.log.Error("read product", "productId", productID, "error", err)
		writeError(w, a.log, http.StatusInternalServerError, codeInternal, "failed to read product")
		return
	}

	writeJSON(w, a.log, http.StatusOK, product)
}

func (a *API) handleCategories(w http.ResponseWriter, _ *http.Request) {
	type category struct {
		Name       model.Category `json:"name"`
		Restricted bool           `json:"restricted"`
	}

	out := make([]category, 0, len(model.Categories()))
	for _, c := range model.Categories() {
		out = append(out, category{Name: c, Restricted: c.Restricted()})
	}
	writeJSON(w, a.log, http.StatusOK, map[string]any{"categories": out})
}

func (a *API) handleRules(w http.ResponseWriter, _ *http.Request) {
	rules := claims.Rules()
	writeJSON(w, a.log, http.StatusOK, map[string]any{"count": len(rules), "rules": rules})
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		a.log.Error("store health check", "error", err)
		writeJSON(w, a.log, http.StatusServiceUnavailable, map[string]string{
			"status": "DEGRADED",
			"store":  "unreachable",
		})
		return
	}
	writeJSON(w, a.log, http.StatusOK, map[string]string{"status": "OK", "store": "reachable"})
}
