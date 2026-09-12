package config

import (
	"testing"
	"time"
)

func TestLoadFallbacksAndDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("AURA_RADAR_BOT_TOKEN", "tg-secret")
	t.Setenv("AURA_RADAR_OPENAI_API_KEY", "llm-secret")
	t.Setenv("OPENAI_MODEL", "x-ai/grok-4.6")
	t.Setenv("AURA_ALLOWED_CHAT_ID", "-1004468217243")
	t.Setenv("AURA_ALLOWED_CHAT_TITLE", "Winter Arc 🪦")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BotToken != "tg-secret" {
		t.Fatalf("bot token fallback: %q", cfg.BotToken)
	}
	if cfg.OpenAIAPIKey != "llm-secret" {
		t.Fatalf("openai key fallback")
	}
	if cfg.ReactMinConfidence != DefaultReactMinConfidence {
		t.Fatalf("react conf default %v", cfg.ReactMinConfidence)
	}
	if cfg.TextChatCooldown != DefaultTextChatCooldown*time.Second {
		t.Fatalf("text cooldown %v", cfg.TextChatCooldown)
	}
	if cfg.AllowedChatID != -1004468217243 {
		t.Fatalf("allowed chat id %d", cfg.AllowedChatID)
	}
	if !cfg.AllowsChat(-1004468217243, "Winter Arc 🪦", "supergroup") {
		t.Fatal("expected allowed winter arc")
	}
	if cfg.AllowsChat(-1, "other", "supergroup") {
		t.Fatal("other group must be blocked when id is set")
	}
	if !cfg.AllowsChat(42, "dm", "private") {
		t.Fatal("private chats stay allowed for onboarding")
	}
}

func TestAllowsChatByTitleWhenIDUnset(t *testing.T) {
	cfg := Config{AllowedChatTitle: "Winter Arc 🪦"}
	if !cfg.AllowsChat(-5, "Winter Arc 🪦", "supergroup") {
		t.Fatal("title match")
	}
	if cfg.AllowsChat(-5, "Another", "supergroup") {
		t.Fatal("wrong title")
	}
}

func TestBlind(t *testing.T) {
	cfg := Config{OpenAIAPIKey: "k", OpenAIModel: "m"}
	if cfg.Blind() {
		t.Fatal("expected not blind")
	}
	cfg.OpenAIAPIKey = ""
	if !cfg.Blind() {
		t.Fatal("expected blind without key")
	}
}
