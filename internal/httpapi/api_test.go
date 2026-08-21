package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/httpapi"
	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store/memory"
	"github.com/utkarsh/claim-identification/internal/workflow"
)

const sampleProductID = "cfe6aa75-5da8-44f5-b587-56857841ad9f"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "seed", "product.json"))
	if err != nil {
		t.Fatalf("read seed product: %v", err)
	}
	product, err := model.ParseProductDocument(raw)
	if err != nil {
		t.Fatalf("parse seed product: %v", err)
	}

	st := memory.New()
	if err := st.UpsertProduct(context.Background(), product); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := workflow.New(st, claims.New(), workflow.Config{Workers: 2}, log)
	engine.Start()

	api := httpapi.New(httpapi.Config{
		Engine:   engine,
		Store:    st,
		Logger:   log,
		MediaDir: filepath.Join("..", "..", "assets", "images"),
	})

	srv := httptest.NewServer(api.Handler())
	t.Cleanup(func() {
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engine.Shutdown(ctx); err != nil {
			t.Errorf("shutdown engine: %v", err)
		}
	})
	return srv
}

func postJSON(t *testing.T, srv *httptest.Server, path, body string) *http.Response {
	t.Helper()

	resp, err := srv.Client().Post(srv.URL+path, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

type identifyResponse struct {
	WorkflowID string `json:"workflowId"`
	Status     string `json:"status"`
	ProductID  string `json:"productId"`
}

type statusResponse struct {
	WorkflowID string               `json:"workflowId"`
	Status     string               `json:"status"`
	ProductID  string               `json:"productId"`
	Steps      []model.WorkflowStep `json:"steps"`
	Error      string               `json:"error"`
	Product    *model.Product       `json:"product"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func waitForCompletion(t *testing.T, srv *httptest.Server, workflowID string) statusResponse {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := srv.Client().Get(srv.URL + "/claims/status/" + workflowID)
		if err != nil {
			t.Fatalf("GET status: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("status code = %d, want 200", resp.StatusCode)
		}

		got := decode[statusResponse](t, resp)
		if got.Status == "COMPLETED" || got.Status == "FAILED" {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("workflow %s did not finish in time", workflowID)
	return statusResponse{}
}

func TestIdentifyThenStatus(t *testing.T) {
	srv := newTestServer(t)

	resp := postJSON(t, srv, "/claims/identify", `{"productId":"`+sampleProductID+`"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("identify status = %d, want 202", resp.StatusCode)
	}
	triggered := decode[identifyResponse](t, resp)
	if triggered.Status != "IN_PROGRESS" {
		t.Errorf("status = %q, want IN_PROGRESS", triggered.Status)
	}
	if triggered.WorkflowID == "" {
		t.Fatal("no workflowId returned")
	}

	final := waitForCompletion(t, srv, triggered.WorkflowID)
	if final.Status != "COMPLETED" {
		t.Fatalf("final status = %q (error %q), want COMPLETED", final.Status, final.Error)
	}
	if final.Product == nil {
		t.Fatal("completed response has no product")
	}

	if final.Product.ID != sampleProductID {
		t.Errorf("product id = %q, want %q", final.Product.ID, sampleProductID)
	}
	if final.Product.Brand != "Maggi" || final.Product.CategoryID != 15 {
		t.Error("original product fields were not preserved")
	}
	if len(final.Product.AboutItems) != 5 || len(final.Product.Media) != 2 {
		t.Error("original product arrays were not preserved")
	}
	if final.Product.Attributes["pack_size"] != "72.5g" {
		t.Error("product attributes were not preserved")
	}

	want := map[string]string{
		"High Proteins":          "Nutritional Claims",
		"Source of Fibre & Iron": "Nutritional Claims",
		"Safe for children":      "Safety Claims",
		"Made in India":          "Manufacturing / Origin",
		"FSSAI certified":        "Certification Claims",
	}
	if len(final.Product.Claims) != len(want) {
		t.Fatalf("claims = %d, want %d: %+v", len(final.Product.Claims), len(want), final.Product.Claims)
	}
	for _, c := range final.Product.Claims {
		wantType, ok := want[c.ClaimValue]
		if !ok {
			t.Errorf("unexpected claim %q", c.ClaimValue)
			continue
		}
		if string(c.ClaimType) != wantType {
			t.Errorf("claim %q type = %q, want %q", c.ClaimValue, c.ClaimType, wantType)
		}
		if c.Status != model.ClaimStatusIdentified {
			t.Errorf("claim %q status = %q, want IDENTIFIED", c.ClaimValue, c.Status)
		}
		if c.ID == "" {
			t.Errorf("claim %q has no id", c.ClaimValue)
		}
	}
}

func TestIdentifyValidation(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"empty body", ``, http.StatusBadRequest, "INVALID_REQUEST"},
		{"malformed json", `{`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"missing productId", `{}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"blank productId", `{"productId":"   "}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"wrong type", `{"productId":42}`, http.StatusBadRequest, "INVALID_REQUEST"},
		{"unknown product", `{"productId":"nope"}`, http.StatusNotFound, "PRODUCT_NOT_FOUND"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv, "/claims/identify", tc.body)
			if resp.StatusCode != tc.wantCode {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantCode)
			}
			if got := decode[errorResponse](t, resp); got.Error.Code != tc.wantErr {
				t.Errorf("error code = %q, want %q", got.Error.Code, tc.wantErr)
			}
		})
	}
}

func TestStatusUnknownWorkflow(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/claims/status/wf-does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if got := decode[errorResponse](t, resp); got.Error.Code != "WORKFLOW_NOT_FOUND" {
		t.Errorf("error code = %q, want WORKFLOW_NOT_FOUND", got.Error.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/claims/identify")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "POST" {
		t.Errorf("Allow = %q, want POST", allow)
	}
}

func TestUnknownRoute(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if got := decode[errorResponse](t, resp); got.Error.Code == "" {
		t.Error("unknown route did not return a JSON error envelope")
	}
}

func TestGetProductReflectsClaims(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/products/" + sampleProductID)
	if err != nil {
		t.Fatalf("GET product: %v", err)
	}
	if before := decode[model.Product](t, resp); len(before.Claims) != 0 {
		t.Errorf("claims before run = %d, want 0", len(before.Claims))
	}

	triggered := decode[identifyResponse](t, postJSON(t, srv, "/claims/identify", `{"productId":"`+sampleProductID+`"}`))
	waitForCompletion(t, srv, triggered.WorkflowID)

	resp, err = srv.Client().Get(srv.URL + "/products/" + sampleProductID)
	if err != nil {
		t.Fatalf("GET product: %v", err)
	}
	if after := decode[model.Product](t, resp); len(after.Claims) != 5 {
		t.Errorf("claims after run = %d, want 5", len(after.Claims))
	}
}

func TestSupportingEndpoints(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/healthz", "/claims/categories", "/claims/rules"} {
		t.Run(path, func(t *testing.T) {
			resp, err := srv.Client().Get(srv.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("content-type = %q", ct)
			}
		})
	}
}

func TestConcurrentRunsAreIndependent(t *testing.T) {
	srv := newTestServer(t)

	const runs = 8
	ids := make([]string, 0, runs)
	for i := 0; i < runs; i++ {
		resp := postJSON(t, srv, "/claims/identify", `{"productId":"`+sampleProductID+`"}`)
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("identify status = %d, want 202", resp.StatusCode)
		}
		ids = append(ids, decode[identifyResponse](t, resp).WorkflowID)
	}

	seen := make(map[string]bool, runs)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate workflow id %s", id)
		}
		seen[id] = true

		final := waitForCompletion(t, srv, id)
		if final.Status != "COMPLETED" {
			t.Errorf("workflow %s status = %q, want COMPLETED", id, final.Status)
		}
		if final.Product == nil || len(final.Product.Claims) != 5 {
			t.Errorf("workflow %s did not produce 5 claims", id)
		}
	}
}
