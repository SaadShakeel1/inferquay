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
	Model         string
	Response      string
	PromptTokens  int
	OutputTokens  int
	TotalDuration time.Duration
}

type GenerateFunc func(context.Context, string, string) (Result, error)
type StreamFunc func(context.Context, string, string, func(string) error) (Result, error)

type Service struct {
	model    string
	timeout  time.Duration
	slots    chan struct{}
	generate GenerateFunc
	stream   StreamFunc
}

func NewService(model string, timeout time.Duration, maxConcurrent int, generate GenerateFunc, stream StreamFunc) (*Service, error) {
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
	if stream == nil {
		return nil, errors.New("inference streamer must not be nil")
	}

	return &Service{
		model:    model,
		timeout:  timeout,
		slots:    make(chan struct{}, maxConcurrent),
		generate: generate,
		stream:   stream,
	}, nil
}

func (s *Service) Generate(ctx context.Context, prompt string) (Result, error) {
	inferenceCtx, finish, err := s.start(ctx, prompt)
	if err != nil {
		return Result{}, err
	}
	defer finish()

	result, err := s.generate(inferenceCtx, s.model, prompt)
	if err != nil {
		return Result{}, fmt.Errorf("generate inference: %w", err)
	}
	return result, nil
}

func (s *Service) Stream(ctx context.Context, prompt string, onChunk func(string) error) (Result, error) {
	if onChunk == nil {
		return Result{}, errors.New("stream callback must not be nil")
	}

	inferenceCtx, finish, err := s.start(ctx, prompt)
	if err != nil {
		return Result{}, err
	}
	defer finish()

	result, err := s.stream(inferenceCtx, s.model, prompt, onChunk)
	if err != nil {
		return Result{}, fmt.Errorf("stream inference: %w", err)
	}
	return result, nil
}

func (s *Service) start(ctx context.Context, prompt string) (context.Context, func(), error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, nil, ErrInvalidPrompt
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
		return nil, nil, ErrBusy
	}

	if err := ctx.Err(); err != nil {
		<-s.slots
		return nil, nil, err
	}

	inferenceCtx, cancel := context.WithTimeout(ctx, s.timeout)
	finish := func() {
		cancel()
		<-s.slots
	}

	return inferenceCtx, finish, nil
}
