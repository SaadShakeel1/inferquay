package inference

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBusy          = errors.New("inference capacity exhausted")
	ErrInvalidPrompt = errors.New("prompt must not be empty")
)

type Result struct {
	Model        string
	Response     string
	PromptTokens int
}

type GenerateFunc func(ctx context.Context, model string, prompt string) (Result, error)

type Service struct {
	model    string
	timeout  time.Duration
	slots    chan struct{}
	generate GenerateFunc
}

func NewService(model string, timeout time.Duration, maxConcurrent int, generate GenerateFunc) (*Service, error) {
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("inference model must not be empty")
	}
	if timeout <= 0 {
		return nil, errors.New("inference timeout must be greater than zero")
	}
	if maxConcurrent <= 0 {
		return nil, errors.New("maximum concurrent inference must be greater than zero")
	}
	if generate == nil {
		return nil, errors.New("inference generator must not be nil")
	}

	return &Service{
		model:    model,
		timeout:  timeout,
		slots:    make(chan struct{}, maxConcurrent),
		generate: generate,
	}, nil
}

func (s *Service) Generate(ctx context.Context, prompt string) (Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return Result{}, ErrInvalidPrompt
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Result{}, ErrBusy
	}

	inferenceCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.generate(inferenceCtx, s.model, prompt)
	if err != nil {
		return Result{}, fmt.Errorf("generate inference: %w", err)
	}

	return result, nil
}
