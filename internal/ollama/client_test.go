package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var request struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream *bool  `json:"stream"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if request.Model != "test-model" {
			t.Errorf("model = %q, want test-model", request.Model)
		}
		if request.Prompt != "Say hello" {
			t.Errorf("prompt = %q, want Say hello", request.Prompt)
		}
		if request.Stream == nil || *request.Stream {
			t.Errorf("stream = %v, want explicit false", request.Stream)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"model":"test-model",
			"response":"Hello!",
			"done":true,
			"prompt_eval_count":3,
			"eval_count":2,
			"total_duration":1000000000
		}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	got, err := client.Generate(
		context.Background(),
		"test-model",
		"Say hello",
	)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	want := Result{
		Model:         "test-model",
		Response:      "Hello!",
		PromptTokens:  3,
		OutputTokens:  2,
		TotalDuration: time.Second,
	}

	if got != want {
		t.Fatalf("Generate() = %+v, want %+v", got, want)
	}
}

func TestGenerateReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		http.Error(w, "model unavailable", http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	_, err = client.Generate(
		context.Background(),
		"missing-model",
		"Hello",
	)

	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want *StatusError", err)
	}

	if statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf(
			"status = %d, want %d",
			statusErr.StatusCode,
			http.StatusNotFound,
		)
	}
}

func TestGenerateRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed JSON",
			body: `{`,
		},
		{
			name: "incomplete generation",
			body: `{"model":"test-model","response":"partial","done":false}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, server.Client())
			if err != nil {
				t.Fatalf("NewClient() error: %v", err)
			}

			_, err = client.Generate(
				context.Background(),
				"test-model",
				"Hello",
			)
			if err == nil {
				t.Fatal("Generate() returned nil error for invalid response")
			}
		})
	}
}

func TestGeneratePropagatesCancellation(t *testing.T) {
	started := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)

	go func() {
		_, err := client.Generate(ctx, "test-model", "Hello")
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Ollama request did not start")
	}

	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Generate() did not stop after cancellation")
	}
}
