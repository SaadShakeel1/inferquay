package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SaadShakeel1/inferquay/internal/inference"
)

func TestInferenceSuccess(t *testing.T) {
	router := NewRouter(fakeGenerator(func(_ context.Context, prompt string) (inference.Result, error) {
		if prompt != "Hello" {
			t.Errorf("prompt = %q, want Hello", prompt)
		}
		return inference.Result{
			Model:         "test-model",
			Response:      "Hi!",
			PromptTokens:  2,
			OutputTokens:  3,
			TotalDuration: 1500 * time.Millisecond,
		}, nil
	}))

	request := httptest.NewRequest(http.MethodPost, "/v1/inference", strings.NewReader(`{"prompt":"Hello"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	var response struct {
		Model           string `json:"model"`
		Response        string `json:"response"`
		PromptTokens    int    `json:"prompt_tokens"`
		OutputTokens    int    `json:"output_tokens"`
		TotalDurationMS int64  `json:"total_duration_ms"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Model != "test-model" || response.Response != "Hi!" ||
		response.PromptTokens != 2 || response.OutputTokens != 3 ||
		response.TotalDurationMS != 1500 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestInferenceRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{"wrong content type", "text/plain", `{"prompt":"Hi"}`, http.StatusUnsupportedMediaType},
		{"malformed JSON", "application/json", `{"prompt":`, http.StatusBadRequest},
		{"unknown field", "application/json", `{"prompt":"Hi","model":"other"}`, http.StatusBadRequest},
		{"multiple objects", "application/json", `{"prompt":"Hi"}{"prompt":"Again"}`, http.StatusBadRequest},
		{"empty prompt", "application/json", `{"prompt":" "}`, http.StatusBadRequest},
		{"oversized body", "application/json", `{"prompt":"` + strings.Repeat("a", maxInferenceRequestBytes) + `"}`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			router := NewRouter(fakeGenerator(func(_ context.Context, prompt string) (inference.Result, error) {
				called = true
				if strings.TrimSpace(prompt) == "" {
					return inference.Result{}, inference.ErrInvalidPrompt
				}
				return inference.Result{}, nil
			}))
			request := httptest.NewRequest(http.MethodPost, "/v1/inference", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.name != "empty prompt" && called {
				t.Error("generator called for invalid request")
			}
		})
	}
}

func TestInferenceMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"invalid prompt", inference.ErrInvalidPrompt, http.StatusBadRequest},
		{"busy", inference.ErrBusy, http.StatusServiceUnavailable},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"upstream failure", errors.New("Ollama unavailable"), http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(fakeGenerator(func(context.Context, string) (inference.Result, error) {
				return inference.Result{}, tt.err
			}))
			request := httptest.NewRequest(http.MethodPost, "/v1/inference", strings.NewReader(`{"prompt":"Hi"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.name == "busy" && recorder.Header().Get("Retry-After") != "1" {
				t.Errorf("Retry-After = %q, want 1", recorder.Header().Get("Retry-After"))
			}
			var response struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error == "" {
				t.Error("error response is empty")
			}
		})
	}
}
