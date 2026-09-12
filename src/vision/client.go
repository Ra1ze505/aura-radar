package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

const describePrompt = "Опиши изображение для судьи ауры в русском Telegram-чате. " +
	"По-русски, 1–3 коротких предложения: что на картинке, текст если читается, мем/шутка если есть. " +
	"Не ставь оценку ауры, не здоровайся."

// Client talks to an OpenAI-compatible chat.completions vision endpoint.
type Client struct {
	api   openai.Client
	model string
}

func New(apiKey, baseURL, model string, timeout time.Duration) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
		option.WithHeader("HTTP-Referer", "https://github.com/Ra1ze505/aura-radar"),
		option.WithHeader("X-Title", "Aura Radar"),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{
		api:   openai.NewClient(opts...),
		model: model,
	}
}

func (c *Client) Describe(ctx context.Context, r io.Reader, filename, mime string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("vision: nil client")
	}
	if strings.TrimSpace(c.model) == "" {
		return "", fmt.Errorf("vision: empty model")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("vision read: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("vision: empty file")
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	_ = filename
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(c.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				openai.TextContentPart(describePrompt),
				openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL:    dataURL,
					Detail: "auto",
				}),
			}),
		},
		Temperature:     param.NewOpt(0.2),
		MaxTokens:       param.NewOpt(int64(300)),
		ReasoningEffort: shared.ReasoningEffortLow,
	}
	completion, err := c.api.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("vision: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("vision: empty choices")
	}
	return strings.TrimSpace(completion.Choices[0].Message.Content), nil
}
