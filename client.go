package sense

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type claudeClient struct {
	client anthropic.Client
}

func newClaudeClient(apiKey string) *claudeClient {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	client := anthropic.NewClient(opts...)
	return &claudeClient{client: client}
}

// evalToolSchema is the JSON schema for the submit_evaluation tool.
var evalToolSchema = anthropic.ToolInputSchemaParam{
	Properties: map[string]any{
		"pass": map[string]any{
			"type":        "boolean",
			"description": "True only if ALL expectations pass",
		},
		"score": map[string]any{
			"type":        "number",
			"minimum":     0,
			"maximum":     1,
			"description": "Fraction of expectations that passed (0.0 to 1.0)",
		},
		"checks": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expect": map[string]any{
						"type":        "string",
						"description": "The expectation text",
					},
					"pass": map[string]any{
						"type":        "boolean",
						"description": "Whether the output satisfies this expectation",
					},
					"confidence": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Confidence in the judgment (0.0 to 1.0)",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Why it passes or fails",
					},
					"evidence": map[string]any{
						"type":        "string",
						"description": "Specific quotes or references from the output",
					},
				},
				"required": []string{"expect", "pass", "confidence", "reason"},
			},
		},
	},
	Required: []string{"pass", "score", "checks"},
}

// compareToolSchema is the JSON schema for the submit_comparison tool.
var compareToolSchema = anthropic.ToolInputSchemaParam{
	Properties: map[string]any{
		"winner": map[string]any{
			"type":        "string",
			"enum":        []string{"A", "B", "tie"},
			"description": "Which output is better overall",
		},
		"score_a": map[string]any{
			"type":    "number",
			"minimum": 0,
			"maximum": 1,
		},
		"score_b": map[string]any{
			"type":    "number",
			"minimum": 0,
			"maximum": 1,
		},
		"criteria": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"score_a": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"score_b": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"winner":  map[string]any{"type": "string", "enum": []string{"A", "B", "tie"}},
					"reason":  map[string]any{"type": "string"},
				},
				"required": []string{"name", "score_a", "score_b", "winner", "reason"},
			},
		},
		"reasoning": map[string]any{
			"type":        "string",
			"description": "Overall reasoning for the winner",
		},
	},
	Required: []string{"winner", "score_a", "score_b", "criteria", "reasoning"},
}

type callRequest struct {
	systemPrompt string
	userMessage  string
	toolName     string
	toolSchema   anthropic.ToolInputSchemaParam
	model        string
}

func (c *claudeClient) call(ctx context.Context, req callRequest) (json.RawMessage, *Usage, error) {
	model := req.model
	if model == "" {
		model = getModel()
	}

	tools := []anthropic.ToolUnionParam{{
		OfTool: &anthropic.ToolParam{
			Name:        req.toolName,
			Description: anthropic.String("Submit the structured result"),
			InputSchema: req.toolSchema,
		},
	}}

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: req.systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.userMessage)),
		},
		Tools:      tools,
		ToolChoice: anthropic.ToolChoiceParamOfTool(req.toolName),
	}

	maxRetries := globalConfig.MaxRetries
	var lastErr error

	for attempt := range maxRetries {
		message, err := c.client.Messages.New(ctx, params)
		if err != nil {
			if isRetryable(err) && attempt < maxRetries-1 {
				lastErr = err
				backoff(attempt)
				continue
			}
			return nil, nil, fmt.Errorf("api call failed: %w", err)
		}

		usage := &Usage{
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
		}

		// Extract tool call result
		for _, block := range message.Content {
			if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				raw := json.RawMessage(variant.JSON.Input.Raw())
				return raw, usage, nil
			}
		}

		return nil, usage, ErrNoToolCall
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrRateLimit, lastErr)
	}
	return nil, nil, ErrNoToolCall
}

// Usage tracks token consumption for a single call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

func isRetryable(err error) bool {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		code := apiErr.StatusCode
		return code == http.StatusTooManyRequests || code >= 500
	}
	return false
}

func backoff(attempt int) {
	delay := min(time.Duration(math.Pow(2, float64(attempt)))*time.Second, 30*time.Second)
	time.Sleep(delay)
}
