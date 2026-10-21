package inference

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceGenerate(t *testing.T) {
	service, err := NewService(
		"test-model",
		time.Second,
		1,
		func(_ context.Context, model, prompt string) (Result, error) {
			if model != "test-model" {
				t.Errorf("model = %q, want test-model", model)
			}
			if prompt != "Hello" {
				t.Errorf("prompt = %q, want Hello", prompt)
			}

			return Result{Model: model, Response: "Hi!"}, nil
		},
	)
	if err != nil {
		t.Fatalf("NewService() error: %v", err)
	}

	got, err := service.Generate(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got.Response != "Hi!" {
		t.Fatalf("response = %q, want Hi!", got.Response)
	}
}

func TestServiceRejectsEmptyPrompt(t *testing.T) {
	service, err := NewService(
		"test-model",
		time.Second,
		1,
		func(context.Context, string, string) (Result, error) {
			t.Fatal("generator called for empty prompt")
			return Result{}, nil
		},
	)
	if err != nil {
		t.Fatalf("NewService() error: %v", err)
	}

	_, err = service.Generate(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("error = %v, want ErrInvalidPrompt", err)
	}
}

func TestServiceRejectsExcessWork(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	service, err := NewService(
		"test-model",
		time.Second,
		1,
		func(ctx context.Context, model, prompt string) (Result, error) {
			close(started)

			select {
			case <-release:
				return Result{Model: model, Response: "done"}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		},
	)
	if err != nil {
		t.Fatalf("NewService() error: %v", err)
	}

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.Generate(context.Background(), "first")
		firstResult <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first inference did not start")
	}

	_, err = service.Generate(context.Background(), "second")
	if !errors.Is(err, ErrBusy) {
		t.Errorf("second request error = %v, want ErrBusy", err)
	}

	close(release)

	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first request error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first inference did not finish")
	}
}

func TestServicePropagatesCancellation(t *testing.T) {
	started := make(chan struct{})

	service, err := NewService(
		"test-model",
		time.Second,
		1,
		func(ctx context.Context, _, _ string) (Result, error) {
			close(started)
			<-ctx.Done()
			return Result{}, ctx.Err()
		},
	)
	if err != nil {
		t.Fatalf("NewService() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := service.Generate(ctx, "Hello")
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("inference did not start")
	}

	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inference did not stop after cancellation")
	}
}

func TestServiceAppliesTimeout(t *testing.T) {
	service, err := NewService(
		"test-model",
		20*time.Millisecond,
		1,
		func(ctx context.Context, _, _ string) (Result, error) {
			<-ctx.Done()
			return Result{}, ctx.Err()
		},
	)
	if err != nil {
		t.Fatalf("NewService() error: %v", err)
	}

	_, err = service.Generate(context.Background(), "Hello")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}
