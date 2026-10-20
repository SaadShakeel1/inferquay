package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SaadShakeel1/inferquay/internal/api"
	"github.com/SaadShakeel1/inferquay/internal/config"
	"github.com/SaadShakeel1/inferquay/internal/inference"
	"github.com/SaadShakeel1/inferquay/internal/ollama"
)

const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 1 << 20
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", slog.Any("error", err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("application stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	ollamaClient, err := ollama.NewClient(cfg.OllamaBaseURL, &http.Client{})
	if err != nil {
		return fmt.Errorf("create Ollama client: %w", err)
	}

	service, err := inference.NewService(
		cfg.OllamaModel,
		cfg.InferenceTimeout,
		cfg.MaxConcurrentInference,
		func(ctx context.Context, model, prompt string) (inference.Result, error) {
			result, err := ollamaClient.Generate(ctx, model, prompt)
			if err != nil {
				return inference.Result{}, err
			}

			return inference.Result{
				Model:         result.Model,
				Response:      result.Response,
				PromptTokens:  result.PromptTokens,
				OutputTokens:  result.OutputTokens,
				TotalDuration: result.TotalDuration,
			}, nil
		},
	)
	if err != nil {
		return fmt.Errorf("create inference service: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(service),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErrors := make(chan error, 1)

	logger.Info("starting HTTP server", slog.String("address", cfg.HTTPAddr))

	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)

	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		closeErr := server.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("gracefully shut down HTTP server: %w", err),
				fmt.Errorf("force close HTTP server: %w", closeErr),
			)
		}
		return fmt.Errorf("gracefully shut down HTTP server: %w", err)
	}

	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}

	logger.Info("HTTP server stopped")
	return nil
}
