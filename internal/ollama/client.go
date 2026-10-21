package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type Client struct {
	endpoint   string
	httpClient *http.Client
}

type Result struct {
	Model         string
	Response      string
	PromptTokens  int
	OutputTokens  int
	TotalDuration time.Duration
}

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("ollama returned HTTP %d", e.StatusCode)
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("ollama HTTP client must not be nil")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid Ollama base URL")
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/api/generate"
	u.RawPath = ""

	return &Client{
		endpoint:   u.String(),
		httpClient: httpClient,
	}, nil
}

func (c *Client) Generate(ctx context.Context, model string, prompt string) (Result, error) {
	requestBody, err := json.Marshal(struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	})

	if err != nil {
		return Result{}, fmt.Errorf("encode ollama request:%w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(requestBody))

	if err != nil {
		return Result{}, fmt.Errorf("create Ollama request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("send Ollama request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, &StatusError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("read Ollama response: %w", err)
	}

	if len(body) > maxResponseBytes {
		return Result{}, errors.New("Ollama response exceeds size limit")
	}

	var payload struct {
		Model           string `json:"model"`
		Response        string `json:"response"`
		Done            bool   `json:"done"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		TotalDuration   int64  `json:"total_duration"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("decode Ollama response: %w", err)
	}

	if !payload.Done {
		return Result{}, errors.New("Ollama response is not marked complete")
	}

	return Result{
		Model:         payload.Model,
		Response:      payload.Response,
		PromptTokens:  payload.PromptEvalCount,
		OutputTokens:  payload.EvalCount,
		TotalDuration: time.Duration(payload.TotalDuration),
	}, nil
}
