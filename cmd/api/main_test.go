package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/SaadShakeel1/inferquay/internal/config"
)

func TestRunReturnsServerStartupError(t *testing.T) {
	logger := discardLogger()

	cfg := config.Config{
		HTTPAddr:               "invalid-address",
		OllamaBaseURL:          "http://localhost:11434",
		OllamaModel:            "test-model",
		InferenceTimeout:       time.Second,
		MaxConcurrentInference: 1,
		ShutdownTimeout:        time.Second,
	}

	err := run(context.Background(), cfg, logger)
	if err == nil {
		t.Fatal("run() returned nil error, want a server startup error")
	}

	if !strings.Contains(err.Error(), "serve HTTP") {
		t.Fatalf("run() error = %q, want it to contain %q", err.Error(), "serve HTTP")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	logger := discardLogger()

	cfg := config.Config{
		HTTPAddr:               "127.0.0.1:0",
		OllamaBaseURL:          "http://localhost:11434",
		OllamaModel:            "test-model",
		InferenceTimeout:       time.Second,
		MaxConcurrentInference: 1,
		ShutdownTimeout:        time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := run(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
