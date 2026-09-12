package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	DefaultSQLitePath    = "data/aura_radar.db"
	DefaultLogLevel      = "INFO"

	DefaultContextMessages = 30
	DefaultMinTextLen      = 8
	DefaultLLMTimeoutSec   = 20

	DefaultReactMinConfidence = 0.62
	DefaultReactMinAbsDelta   = 80
	DefaultReactChatCooldown  = 12
	DefaultReactUserCooldown  = 90
	DefaultReactChatDailyCap  = 80

	DefaultTextMinConfidence = 0.78
	DefaultTextMinAbsDelta   = 500
	DefaultTextChatCooldown  = 720
	DefaultTextUserCooldown  = 10800
	DefaultTextChatDailyCap  = 4
)

// Config is process configuration from env. Secrets are never logged.
type Config struct {
	BotToken      string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string
	DatabaseURL   string
	LogLevel      string

	AllowedChatID    int64
	AllowedChatTitle string

	ContextMessages int
	MinTextLen      int
	LLMTimeout      time.Duration

	ReactMinConfidence float64
	ReactMinAbsDelta   int
	ReactChatCooldown  time.Duration
	ReactUserCooldown  time.Duration
	ReactChatDailyCap  int
	TextMinConfidence  float64
	TextMinAbsDelta    int
	TextChatCooldown   time.Duration
	TextUserCooldown   time.Duration
	TextChatDailyCap   int
}

func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{
		BotToken:      firstNonEmpty(os.Getenv("BOT_TOKEN"), os.Getenv("AURA_RADAR_BOT_TOKEN")),
		OpenAIAPIKey:  firstNonEmpty(os.Getenv("OPENAI_API_KEY"), os.Getenv("AURA_RADAR_OPENAI_API_KEY")),
		OpenAIModel:   strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		OpenAIBaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:      strings.TrimSpace(os.Getenv("LOG_LEVEL")),

		AllowedChatTitle: strings.TrimSpace(os.Getenv("AURA_ALLOWED_CHAT_TITLE")),
		ContextMessages:  envInt("AURA_CONTEXT_MESSAGES", DefaultContextMessages),
		MinTextLen:       envInt("AURA_MIN_TEXT_LEN", DefaultMinTextLen),
		LLMTimeout:       time.Duration(envInt("AURA_LLM_TIMEOUT_SEC", DefaultLLMTimeoutSec)) * time.Second,

		ReactMinConfidence: envFloat("AURA_REACT_MIN_CONFIDENCE", DefaultReactMinConfidence),
		ReactMinAbsDelta:   envInt("AURA_REACT_MIN_ABS_DELTA", DefaultReactMinAbsDelta),
		ReactChatCooldown:  time.Duration(envInt("AURA_REACT_CHAT_COOLDOWN_SEC", DefaultReactChatCooldown)) * time.Second,
		ReactUserCooldown:  time.Duration(envInt("AURA_REACT_USER_COOLDOWN_SEC", DefaultReactUserCooldown)) * time.Second,
		ReactChatDailyCap:  envInt("AURA_REACT_CHAT_DAILY_CAP", DefaultReactChatDailyCap),

		TextMinConfidence: envFloat("AURA_TEXT_MIN_CONFIDENCE", DefaultTextMinConfidence),
		TextMinAbsDelta:   envInt("AURA_TEXT_MIN_ABS_DELTA", DefaultTextMinAbsDelta),
		TextChatCooldown:  time.Duration(envInt("AURA_TEXT_CHAT_COOLDOWN_SEC", DefaultTextChatCooldown)) * time.Second,
		TextUserCooldown:  time.Duration(envInt("AURA_TEXT_USER_COOLDOWN_SEC", DefaultTextUserCooldown)) * time.Second,
		TextChatDailyCap:  envInt("AURA_TEXT_CHAT_DAILY_CAP", DefaultTextChatDailyCap),
	}
	if cfg.OpenAIBaseURL == "" {
		cfg.OpenAIBaseURL = DefaultOpenAIBaseURL
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = DefaultSQLitePath
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	if id := strings.TrimSpace(os.Getenv("AURA_ALLOWED_CHAT_ID")); id != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("AURA_ALLOWED_CHAT_ID: %w", err)
		}
		cfg.AllowedChatID = n
	}
	return cfg, nil
}

func (c Config) Blind() bool {
	return strings.TrimSpace(c.OpenAIAPIKey) == "" || strings.TrimSpace(c.OpenAIModel) == ""
}

func (c Config) AllowsChat(chatID int64, title, chatType string) bool {
	if chatType == "private" {
		return true
	}
	if c.AllowedChatID == 0 && c.AllowedChatTitle == "" {
		return true
	}
	if c.AllowedChatID != 0 && chatID == c.AllowedChatID {
		return true
	}
	if c.AllowedChatID == 0 && c.AllowedChatTitle != "" && title == c.AllowedChatTitle {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return n
}
