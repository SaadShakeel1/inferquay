package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const maxStreamRecordBytes = 1 << 20

func (c *Client) Stream(
	ctx context.Context,
	model, prompt string,
	onChunk func(string) error,
) (Result, error) {
	if onChunk == nil {
		return Result{}, errors.New("stream callback must not be nil")
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	requestBody, err := json.Marshal(struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}{
		Model:  model,
		Prompt: prompt,
		Stream: true,
	})
	if err != nil {
		return Result{}, fmt.Errorf("encode Ollama stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return Result{}, fmt.Errorf("create Ollama stream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("send Ollama stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, &StatusError{
			StatusCode: resp.StatusCode,
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), maxStreamRecordBytes)

	for scanner.Scan() {
		var record struct {
			Model           string `json:"model"`
			Response        string `json:"response"`
			Done            bool   `json:"done"`
			Error           string `json:"error"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
			TotalDuration   int64  `json:"total_duration"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return Result{}, fmt.Errorf(
				"decode Ollama stream record: %w",
				err,
			)
		}

		if record.Error != "" {
			return Result{}, fmt.Errorf(
				"Ollama stream error: %s",
				record.Error,
			)
		}

		if record.Response != "" {
			if err := onChunk(record.Response); err != nil {
				return Result{}, fmt.Errorf(
					"deliver Ollama stream chunk: %w",
					err,
				)
			}
		}

		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		if record.Done {
			return Result{
				Model:         record.Model,
				PromptTokens:  record.PromptEvalCount,
				OutputTokens:  record.EvalCount,
				TotalDuration: time.Duration(record.TotalDuration),
			}, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	if err := scanner.Err(); err != nil {
		return Result{}, fmt.Errorf(
			"read Ollama stream: %w",
			err,
		)
	}

	return Result{}, errors.New(
		"Ollama stream ended without completion record",
	)
}
