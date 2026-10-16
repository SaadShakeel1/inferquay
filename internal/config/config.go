package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr               = ":8080"
	defaultOllamaBaseURL          = "http://localhost:11434"
	defaultOllamaModel            = "qwen2.5:1.5b"
	defaultInferenceTimeout       = 60 * time.Second
	defaultMaxConcurrentInference = 1
	defaultShutdownTimeout        = 10 * time.Second
)

type Config struct {
	HTTPAddr               string
	OllamaBaseURL          string
	OllamaModel            string
	InferenceTimeout       time.Duration
	MaxConcurrentInference int
	ShutdownTimeout        time.Duration
}

func Load() (Config, error) {
	inferenceTimeout, err := time.ParseDuration(envOrDefault("INFERENCE_TIMEOUT", defaultInferenceTimeout.String()))
	if err != nil {
		return Config{}, fmt.Errorf("parse INFERENCE_TIMEOUT: %w", err)
	}

	maxConcurrentInference, err := strconv.Atoi(envOrDefault("MAX_CONCURRENT_INFERENCE", strconv.Itoa(defaultMaxConcurrentInference)))
	if err != nil {
		return Config{}, fmt.Errorf("parse MAX_CONCURRENT_INFERENCE: %w", err)
	}

	shutdownTimeout, err := time.ParseDuration(envOrDefault("SHUTDOWN_TIMEOUT", defaultShutdownTimeout.String()))

	if err != nil {
		return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg := Config{
		HTTPAddr:               envOrDefault("HTTP_ADDR", defaultHTTPAddr),
		OllamaBaseURL:          strings.TrimRight(envOrDefault("OLLAMA_BASE_URL", defaultOllamaBaseURL), "/"),
		OllamaModel:            envOrDefault("OLLAMA_MODEL", defaultOllamaModel),
		InferenceTimeout:       inferenceTimeout,
		MaxConcurrentInference: maxConcurrentInference,
		ShutdownTimeout:        shutdownTimeout,
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) validate() error {
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}

	if strings.TrimSpace(cfg.OllamaModel) == "" {
		return fmt.Errorf("OLLAMA_MODEL must not be empty")
	}

	if cfg.InferenceTimeout <= 0 {
		return fmt.Errorf("INFERENCE_TIMEOUT must be greater than zero")
	}

	if cfg.MaxConcurrentInference <= 0 {
		return fmt.Errorf("MAX_CONCURRENT_INFERENCE must be greater than zero")
	}

	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be greater than zero")
	}
	parsedURL, err := url.Parse(cfg.OllamaBaseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("OLLAMA_BASE_URL must be a valid absolute URL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("OLLAMA_BASE_URL scheme must be http or https")
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
