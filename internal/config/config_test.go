package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	clearEnvironment(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	want := Config{
		HTTPAddr:               ":8080",
		OllamaBaseURL:          "http://localhost:11434",
		OllamaModel:            "qwen2.5:1.5b",
		InferenceTimeout:       60 * time.Second,
		MaxConcurrentInference: 1,
		ShutdownTimeout:        10 * time.Second,
	}

	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadUsesEnvironmentValues(t *testing.T) {
	clearEnvironment(t)

	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("OLLAMA_BASE_URL", "http://ollama.example:11434/")
	t.Setenv("OLLAMA_MODEL", "custom-model")
	t.Setenv("INFERENCE_TIMEOUT", "30s")
	t.Setenv("MAX_CONCURRENT_INFERENCE", "2")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	want := Config{
		HTTPAddr:               ":9090",
		OllamaBaseURL:          "http://ollama.example:11434",
		OllamaModel:            "custom-model",
		InferenceTimeout:       30 * time.Second,
		MaxConcurrentInference: 2,
		ShutdownTimeout:        5 * time.Second,
	}

	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		wantMessage string
	}{
		{
			name:        "invalid timeout",
			key:         "INFERENCE_TIMEOUT",
			value:       "not-a-duration",
			wantMessage: "parse INFERENCE_TIMEOUT",
		},
		{
			name:        "non-positive timeout",
			key:         "INFERENCE_TIMEOUT",
			value:       "0s",
			wantMessage: "must be greater than zero",
		},
		{
			name:        "invalid shutdown timeout",
			key:         "SHUTDOWN_TIMEOUT",
			value:       "not-a-duration",
			wantMessage: "parse SHUTDOWN_TIMEOUT",
		},
		{
			name:        "non-positive shutdown timeout",
			key:         "SHUTDOWN_TIMEOUT",
			value:       "0s",
			wantMessage: "SHUTDOWN_TIMEOUT must be greater than zero",
		},
		{
			name:        "invalid concurrency",
			key:         "MAX_CONCURRENT_INFERENCE",
			value:       "not-a-number",
			wantMessage: "parse MAX_CONCURRENT_INFERENCE",
		},
		{
			name:        "non-positive concurrency",
			key:         "MAX_CONCURRENT_INFERENCE",
			value:       "0",
			wantMessage: "must be greater than zero",
		},
		{
			name:        "invalid Ollama URL",
			key:         "OLLAMA_BASE_URL",
			value:       "not-a-url",
			wantMessage: "valid absolute URL",
		},
		{
			name:        "unsupported Ollama URL scheme",
			key:         "OLLAMA_BASE_URL",
			value:       "ftp://localhost:11434",
			wantMessage: "scheme must be http or https",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearEnvironment(t)
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() returned nil error, want an error")
			}

			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("Load() error = %q, want it to contain %q", err.Error(), test.wantMessage)
			}
		})
	}
}

func clearEnvironment(t *testing.T) {
	t.Helper()

	keys := []string{
		"HTTP_ADDR",
		"OLLAMA_BASE_URL",
		"OLLAMA_MODEL",
		"INFERENCE_TIMEOUT",
		"MAX_CONCURRENT_INFERENCE",
		"SHUTDOWN_TIMEOUT",
	}

	for _, key := range keys {
		t.Setenv(key, "")
	}
}
