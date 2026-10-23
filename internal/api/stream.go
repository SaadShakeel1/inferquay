package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/SaadShakeel1/inferquay/internal/inference"
)

func handleInferenceStream(generator Generator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		prompt, ok := decodeInferencePrompt(w, r)
		if !ok {
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming is not supported")
			return
		}

		started := false
		emit := func(event string, payload any) error {
			data, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("encode SSE event: %w", err)
			}

			if !started {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				started = true
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
				return fmt.Errorf("write SSE event: %w", err)
			}

			flusher.Flush()
			return nil
		}

		result, err := generator.Stream(r.Context(), prompt, func(chunk string) error {
			return emit("token", struct {
				Text string `json:"text"`
			}{Text: chunk})
		})
		if err != nil {
			if r.Context().Err() != nil {
				return // Client disconnected.
			}

			if !started {
				writeStreamStartError(w, err)
				return
			}

			message := "model inference failed"
			if errors.Is(err, context.DeadlineExceeded) {
				message = "inference timed out"
			}

			if writeErr := emit("error", struct {
				Error string `json:"error"`
			}{Error: message}); writeErr != nil {
				slog.Error("write SSE error event", slog.Any("error", writeErr))
			}
			return
		}

		if err := emit("done", struct {
			Model           string `json:"model"`
			PromptTokens    int    `json:"prompt_tokens"`
			OutputTokens    int    `json:"output_tokens"`
			TotalDurationMS int64  `json:"total_duration_ms"`
		}{
			Model:           result.Model,
			PromptTokens:    result.PromptTokens,
			OutputTokens:    result.OutputTokens,
			TotalDurationMS: result.TotalDuration.Milliseconds(),
		}); err != nil {
			slog.Error("write SSE completion event", slog.Any("error", err))
		}
	}
}

func writeStreamStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inference.ErrInvalidPrompt):
		writeError(w, http.StatusBadRequest, "prompt must not be empty")
	case errors.Is(err, inference.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "inference capacity exhausted")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "inference timed out")
	default:
		writeError(w, http.StatusBadGateway, "model inference failed")
	}
}
