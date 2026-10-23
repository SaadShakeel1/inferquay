package inference

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func unusedGenerate(context.Context, string, string) (Result, error) {
	return Result{}, errors.New("unexpected generate call")
}

func TestServiceStreamDeliversChunks(t *testing.T) {
	service, err := NewService("test-model", time.Second, 1, unusedGenerate,
		func(_ context.Context, model, prompt string, onChunk func(string) error) (Result, error) {
			if model != "test-model" || prompt != "Hello" {
				t.Errorf("model = %q, prompt = %q", model, prompt)
			}
			for _, chunk := range []string{"Hel", "lo"} {
				if err := onChunk(chunk); err != nil {
					return Result{}, err
				}
			}
			return Result{Model: model, OutputTokens: 2}, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	var chunks []string
	result, err := service.Stream(context.Background(), "Hello", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "Hello" {
		t.Errorf("chunks = %q, want Hello", got)
	}
	if result.Model != "test-model" || result.OutputTokens != 2 {
		t.Errorf("result = %+v", result)
	}
}

func TestServiceStreamAndGenerateShareCapacity(t *testing.T) {
	generateStarted := make(chan struct{})
	releaseGenerate := make(chan struct{})
	streamStarted := make(chan struct{})
	releaseStream := make(chan struct{})

	service, err := NewService("test-model", 5*time.Second, 1,
		func(ctx context.Context, _, _ string) (Result, error) {
			close(generateStarted)
			select {
			case <-releaseGenerate:
				return Result{}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		},
		func(ctx context.Context, _, _ string, _ func(string) error) (Result, error) {
			close(streamStarted)
			select {
			case <-releaseStream:
				return Result{}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	generateDone := make(chan error, 1)
	go func() {
		_, err := service.Generate(ctx, "first")
		generateDone <- err
	}()

	select {
	case <-generateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Generate did not start")
	}

	_, err = service.Stream(ctx, "second", func(string) error { return nil })
	if !errors.Is(err, ErrBusy) {
		t.Errorf("Stream while Generate active: error = %v, want ErrBusy", err)
	}

	close(releaseGenerate)
	select {
	case err := <-generateDone:
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Generate did not finish")
	}

	streamDone := make(chan error, 1)
	go func() {
		_, err := service.Stream(ctx, "third", func(string) error { return nil })
		streamDone <- err
	}()

	select {
	case <-streamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not start")
	}

	_, err = service.Generate(ctx, "fourth")
	if !errors.Is(err, ErrBusy) {
		t.Errorf("Generate while Stream active: error = %v, want ErrBusy", err)
	}

	close(releaseStream)
	select {
	case err := <-streamDone:
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not finish")
	}
}

func TestServiceStreamReleasesCapacityAfterCallbackError(t *testing.T) {
	service, err := NewService("test-model", time.Second, 1, unusedGenerate,
		func(_ context.Context, _, _ string, onChunk func(string) error) (Result, error) {
			return Result{}, onChunk("text")
		})
	if err != nil {
		t.Fatal(err)
	}

	callbackErr := errors.New("client write failed")
	_, err = service.Stream(context.Background(), "first", func(string) error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("error = %v, want callback error", err)
	}

	_, err = service.Stream(context.Background(), "second", func(string) error { return nil })
	if err != nil {
		t.Fatalf("second Stream error = %v; slot was not released", err)
	}
}

func TestServiceStreamTimeoutAndCancellation(t *testing.T) {
	service, err := NewService("test-model", 20*time.Millisecond, 1, unusedGenerate,
		func(ctx context.Context, _, _ string, onChunk func(string) error) (Result, error) {
			if err := onChunk("first"); err != nil {
				return Result{}, err
			}
			<-ctx.Done()
			return Result{}, ctx.Err()
		})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Stream(context.Background(), "timeout", func(string) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context.DeadlineExceeded", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = service.Stream(ctx, "cancel", func(string) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
}

func TestServiceStreamRejectsInvalidInput(t *testing.T) {
	service, err := NewService("test-model", time.Second, 1, unusedGenerate,
		func(context.Context, string, string, func(string) error) (Result, error) {
			t.Fatal("streamer called for invalid input")
			return Result{}, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Stream(context.Background(), "  ", func(string) error { return nil })
	if !errors.Is(err, ErrInvalidPrompt) {
		t.Errorf("empty prompt error = %v, want ErrInvalidPrompt", err)
	}
	_, err = service.Stream(context.Background(), "Hello", nil)
	if err == nil {
		t.Error("nil callback returned no error")
	}
}
