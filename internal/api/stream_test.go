package api

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SaadShakeel1/inferquay/internal/inference"
)

type fakeStreamer func(context.Context, string, func(string) error) (inference.Result, error)

func (f fakeStreamer) Stream(ctx context.Context, prompt string, onChunk func(string) error) (inference.Result, error) {
	return f(ctx, prompt, onChunk)
}

func (fakeStreamer) Generate(context.Context, string) (inference.Result, error) {
	return inference.Result{}, errors.New("unexpected generate call")
}

func streamRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/v1/inference/stream", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestInferenceStreamWritesSSEEvents(t *testing.T) {
	router := NewRouter(fakeStreamer(func(_ context.Context, prompt string, onChunk func(string) error) (inference.Result, error) {
		if prompt != "Hello" {
			t.Errorf("prompt = %q, want Hello", prompt)
		}
		for _, chunk := range []string{"Hel", "lo"} {
			if err := onChunk(chunk); err != nil {
				return inference.Result{}, err
			}
		}
		return inference.Result{Model: "test-model", PromptTokens: 3, OutputTokens: 2, TotalDuration: time.Second}, nil
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, streamRequest(`{"prompt":"Hello"}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if !recorder.Flushed {
		t.Error("SSE response was not flushed")
	}
	want := "event: token\ndata: {\"text\":\"Hel\"}\n\n" +
		"event: token\ndata: {\"text\":\"lo\"}\n\n" +
		"event: done\ndata: {\"model\":\"test-model\",\"prompt_tokens\":3,\"output_tokens\":2,\"total_duration_ms\":1000}\n\n"
	if got := recorder.Body.String(); got != want {
		t.Fatalf("SSE body = %q, want %q", got, want)
	}
}

func TestInferenceStreamRejectsInvalidRequestBeforeStarting(t *testing.T) {
	called := false
	router := NewRouter(fakeStreamer(func(context.Context, string, func(string) error) (inference.Result, error) {
		called = true
		return inference.Result{}, nil
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, streamRequest(`{"prompt":"Hi","model":"other"}`))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if called {
		t.Error("streamer called for invalid request")
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestInferenceStreamMapsErrorsBeforeFirstEvent(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"invalid prompt", inference.ErrInvalidPrompt, http.StatusBadRequest},
		{"busy", inference.ErrBusy, http.StatusServiceUnavailable},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"upstream failure", errors.New("secret upstream detail"), http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(fakeStreamer(func(context.Context, string, func(string) error) (inference.Result, error) {
				return inference.Result{}, tt.err
			}))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, streamRequest(`{"prompt":"Hello"}`))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.name == "busy" && recorder.Header().Get("Retry-After") != "1" {
				t.Errorf("Retry-After = %q, want 1", recorder.Header().Get("Retry-After"))
			}
			if strings.Contains(recorder.Body.String(), "secret upstream detail") {
				t.Error("upstream error detail leaked to client")
			}
		})
	}
}

func TestInferenceStreamWritesErrorAfterFirstEvent(t *testing.T) {
	router := NewRouter(fakeStreamer(func(_ context.Context, _ string, onChunk func(string) error) (inference.Result, error) {
		if err := onChunk("partial"); err != nil {
			return inference.Result{}, err
		}
		return inference.Result{}, errors.New("secret upstream detail")
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, streamRequest(`{"prompt":"Hello"}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Body.String(); !strings.Contains(got, "event: token\n") || !strings.Contains(got, "event: error\n") {
		t.Fatalf("missing token or error event: %q", got)
	}
	if strings.Contains(recorder.Body.String(), "secret upstream detail") {
		t.Error("upstream error detail leaked to client")
	}
}

func TestInferenceStreamFlushesBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	router := NewRouter(fakeStreamer(func(ctx context.Context, _ string, onChunk func(string) error) (inference.Result, error) {
		if err := onChunk("first"); err != nil {
			return inference.Result{}, err
		}
		select {
		case <-release:
			return inference.Result{Model: "test-model"}, nil
		case <-ctx.Done():
			return inference.Result{}, ctx.Err()
		}
	}))
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(server.URL+"/v1/inference/stream", "application/json", strings.NewReader(`{"prompt":"Hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "event: token\n" {
		t.Fatalf("first line = %q, want token event", line)
	}
	close(release)
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("read remainder: %v", err)
	}
}

func TestInferenceStreamClientDisconnectCancelsUpstream(t *testing.T) {
	upstreamCancelled := make(chan struct{})
	router := NewRouter(fakeStreamer(func(ctx context.Context, _ string, onChunk func(string) error) (inference.Result, error) {
		if err := onChunk("first"); err != nil {
			return inference.Result{}, err
		}
		<-ctx.Done()
		close(upstreamCancelled)
		return inference.Result{}, ctx.Err()
	}))
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/inference/stream", strings.NewReader(`{"prompt":"Hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "event: token\n" {
		t.Fatalf("first event line = %q, error = %v", line, err)
	}
	cancel()

	select {
	case <-upstreamCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("client cancellation did not reach streamer")
	}
}
