package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/SaadShakeel1/inferquay/internal/inference"
)

const (
	healthyPayload           = `{"status":"ok"}` + "\n"
	maxInferenceRequestBytes = 64 << 10
)

type Generator interface {
	Generate(context.Context, string) (inference.Result, error)
	Stream(context.Context, string, func(string) error) (inference.Result, error)
}

func NewRouter(generator Generator) http.Handler {
	if generator == nil {
		panic("api: nil inference generator")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /v1/inference", handleInference(generator))
	mux.HandleFunc("POST /v1/inference/stream", handleInferenceStream(generator))
	return mux
}

func handleInference(generator Generator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		prompt, ok := decodeInferencePrompt(w, r)
		if !ok {
			return
		}

		result, err := generator.Generate(r.Context(), prompt)
		if err != nil {
			switch {
			case errors.Is(err, inference.ErrInvalidPrompt):
				writeError(w, http.StatusBadRequest, "prompt must not be empty")

			case errors.Is(err, inference.ErrBusy):
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "inference capacity exhausted")

			case errors.Is(err, context.DeadlineExceeded):
				writeError(w, http.StatusGatewayTimeout, "inference timed out")

			case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
				// The client disconnected; there is no useful response to send.
				return

			default:
				writeError(w, http.StatusBadGateway, "model inference failed")
			}
			return
		}

		writeJSON(w, http.StatusOK, struct {
			Model           string `json:"model"`
			Response        string `json:"response"`
			PromptTokens    int    `json:"prompt_tokens"`
			OutputTokens    int    `json:"output_tokens"`
			TotalDurationMS int64  `json:"total_duration_ms"`
		}{
			Model:           result.Model,
			Response:        result.Response,
			PromptTokens:    result.PromptTokens,
			OutputTokens:    result.OutputTokens,
			TotalDurationMS: result.TotalDuration.Milliseconds(),
		})
	}
}

func decodeInferencePrompt(w http.ResponseWriter, r *http.Request) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", false
	}

	var request struct {
		Prompt string `json:"prompt"`
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInferenceRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return "", false
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			writeError(w, http.StatusBadRequest, "request must contain one JSON object")
		} else {
			writeDecodeError(w, err)
		}
		return "", false
	}

	return request.Prompt, true
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(w, healthyPayload); err != nil {
		slog.Error("write health response", slog.Any("error", err))
	}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var sizeError *http.MaxBytesError
	if errors.As(err, &sizeError) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid JSON request")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// Headers are already sent, so only logging is possible here.
		slog.Error("write JSON response", slog.Any("error", err))
	}
}
