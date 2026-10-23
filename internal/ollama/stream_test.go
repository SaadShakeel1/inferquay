package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStreamDeliversChunksAndFinalMetadata(t *testing.T) {
	firstChunkDelivered := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/generate" {
			t.Errorf(
				"request = %s %s, want POST /api/generate",
				r.Method,
				r.URL.Path,
			)
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

		if request.Model != "test-model" ||
			request.Prompt != "Say hello" ||
			request.Stream == nil ||
			!*request.Stream {
			t.Errorf("unexpected stream request: %+v", request)
		}

		w.Header().Set("Content-Type", "application/x-ndjson")

		fmt.Fprintln(w, `{"model":"test-model","response":"Hel","done":false}`)
		w.(http.Flusher).Flush()

		select {
		case <-firstChunkDelivered:
		case <-time.After(2 * time.Second):
			t.Error("first chunk was not delivered before stream completed")
			return
		}

		fmt.Fprintln(w, `{"model":"test-model","response":"lo","done":false}`)
		fmt.Fprintln(w, `{"model":"test-model","response":"!","done":true,"prompt_eval_count":3,"eval_count":2,"total_duration":1000000000}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	var chunks []string

	result, err := client.Stream(
		context.Background(),
		"test-model",
		"Say hello",
		func(chunk string) error {
			chunks = append(chunks, chunk)

			if len(chunks) == 1 {
				close(firstChunkDelivered)
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	if want := []string{"Hel", "lo", "!"}; !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %q, want %q", chunks, want)
	}

	if result.Model != "test-model" ||
		result.PromptTokens != 3 ||
		result.OutputTokens != 2 ||
		result.TotalDuration != time.Second ||
		result.Response != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStreamRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed JSON",
			body: "{\n",
		},
		{
			name: "missing completion",
			body: `{"response":"partial","done":false}` + "\n",
		},
		{
			name: "upstream stream error",
			body: `{"error":"model unavailable"}` + "\n",
		},
		{
			name: "oversized record",
			body: `{"response":"` + strings.Repeat("a", maxStreamRecordBytes) + `"}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}

			_, err = client.Stream(
				context.Background(),
				"test-model",
				"Hello",
				func(string) error {
					return nil
				},
			)
			if err == nil {
				t.Fatal("Stream() returned nil error for invalid response")
			}
		})
	}
}

func TestStreamReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Stream(
		context.Background(),
		"missing-model",
		"Hello",
		func(string) error {
			return nil
		},
	)

	var statusErr *StatusError

	if !errors.As(err, &statusErr) ||
		statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf(
			"error = %v, want StatusError with HTTP 404",
			err,
		)
	}
}

func TestStreamStopsWhenCallbackFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"response":"first","done":false}`)
		fmt.Fprintln(w, `{"response":"second","done":true}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	callbackErr := errors.New("client write failed")
	calls := 0

	_, err = client.Stream(
		context.Background(),
		"test-model",
		"Hello",
		func(string) error {
			calls++
			return callbackErr
		},
	)

	if !errors.Is(err, callbackErr) || calls != 1 {
		t.Fatalf(
			"error = %v, callback calls = %d; want callback error after one call",
			err,
			calls,
		)
	}
}

func TestStreamPropagatesCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"response":"first","done":false}`)
		w.(http.Flusher).Flush()

		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = client.Stream(
		ctx,
		"test-model",
		"Hello",
		func(string) error {
			cancel()
			return nil
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"error = %v, want context.Canceled",
			err,
		)
	}
}
