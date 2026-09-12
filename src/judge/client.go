package judge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

type Judge interface {
	Evaluate(ctx context.Context, in PromptInput) (Result, error)
}

type OpenAI struct {
	client openai.Client
	model  string
}

func NewOpenAI(apiKey, baseURL, model string, timeout time.Duration) *OpenAI {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
		option.WithHeader("HTTP-Referer", "https://github.com/Ra1ze505/aura-radar"),
		option.WithHeader("X-Title", "Aura Radar"),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &OpenAI{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

func (j *OpenAI) Evaluate(ctx context.Context, in PromptInput) (Result, error) {
	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(j.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(SystemPrompt()),
			openai.UserMessage(BuildUserPrompt(in)),
		},
		Temperature:     param.NewOpt(0.4),
		MaxTokens:       param.NewOpt(int64(800)),
		ReasoningEffort: shared.ReasoningEffortLow,
	}
	jsonFmt := shared.NewResponseFormatJSONObjectParam()
	params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONObject: &jsonFmt,
	}

	completion, err := j.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("judge: %w", err)
	}
	if len(completion.Choices) == 0 {
		return Result{}, fmt.Errorf("judge: empty choices")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return Result{}, fmt.Errorf("judge: empty content")
	}
	return ParseAndNormalize(content)
}

type Static struct {
	Result Result
	Err    error
}

func (s Static) Evaluate(context.Context, PromptInput) (Result, error) {
	return s.Result, s.Err
}
