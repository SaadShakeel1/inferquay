package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SaadShakeel1/inferquay/internal/inference"
)

type fakeGenerator func(context.Context, string) (inference.Result, error)

func (f fakeGenerator) Generate(ctx context.Context, prompt string) (inference.Result, error) {
	return f(ctx, prompt)
}

func newTestRouter() http.Handler {
	return NewRouter(fakeGenerator(func(context.Context, string) (inference.Result, error) {
		return inference.Result{}, nil
	}))
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	newTestRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status code = %d, want %d",
			recorder.Code,
			http.StatusOK,
		)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf(
			"Content-Type = %q, want %q",
			got,
			"application/json",
		)
	}

	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf(
			"Cache-Control = %q, want %q",
			got,
			"no-store",
		)
	}

	var response struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Fatalf(
			"response status = %q, want %q",
			response.Status,
			"ok",
		)
	}
}

func TestRouterRejectsUnsupportedRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{
			name:       "wrong method",
			method:     http.MethodPost,
			target:     "/healthz",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown route",
			method:     http.MethodGet,
			target:     "/unknown",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			recorder := httptest.NewRecorder()

			newTestRouter().ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf(
					"status code = %d, want %d",
					recorder.Code,
					test.wantStatus,
				)
			}
		})
	}
}
