package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// whisperHint biases STT toward gym/pain vocabulary without locking the language.
// Keep it short: Whisper uses this as a prefix of the same language as the audio.
const whisperHint = "Pain is the only path to change. Love the pain. Discipline, gym, winter arc. Боль, дисциплина, зал. Полюбить боль."

// Client talks to an OpenAI-compatible /audio/transcriptions endpoint.
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
	if strings.TrimSpace(model) == "" {
		model = "openai/whisper-large-v3"
	}
	return &Client{
		api:   openai.NewClient(opts...),
		model: model,
	}
}

func (c *Client) Transcribe(ctx context.Context, r io.Reader, filename, mime string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("transcribe: nil client")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("transcribe read: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("transcribe: empty file")
	}
	if filename == "" {
		filename = "audio.ogg"
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	res, err := c.api.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  openai.File(bytes.NewReader(data), filename, mime),
		Model: c.model,
		// Do not force ru: gym clips are often English. Forced ru turns "pain" into «пах».
		Prompt: openai.String(whisperHint),
	})
	if err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}
	if res == nil {
		return "", fmt.Errorf("transcribe: empty response")
	}
	return strings.TrimSpace(res.Text), nil
}
