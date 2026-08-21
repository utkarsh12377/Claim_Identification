package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorCode string

const (
	codeInvalidRequest   errorCode = "INVALID_REQUEST"
	codeProductNotFound  errorCode = "PRODUCT_NOT_FOUND"
	codeWorkflowNotFound errorCode = "WORKFLOW_NOT_FOUND"
	codeUnavailable      errorCode = "SERVICE_UNAVAILABLE"
	codeInternal         errorCode = "INTERNAL_ERROR"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    errorCode `json:"code"`
	Message string    `json:"message"`
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Error("encode response", "error", err)
		writeError(w, log, http.StatusInternalServerError, codeInternal, "failed to encode response")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Warn("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, log *slog.Logger, status int, code errorCode, message string) {
	body, err := json.Marshal(errorBody{Error: errorDetail{Code: code, Message: message}})
	if err != nil {
		http.Error(w, message, status)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Warn("write error response", "error", err)
	}
}
