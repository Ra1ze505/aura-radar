//go:build live

package bot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ra1ze505/aura-radar/src/config"
	"github.com/Ra1ze505/aura-radar/src/judge"
	"github.com/Ra1ze505/aura-radar/src/store"
)

func TestLiveJudgeScenarios(t *testing.T) {
	key := os.Getenv("AURA_RADAR_OPENAI_API_KEY")
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	if key == "" {
		t.Skip("no LLM key")
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	j := judge.NewOpenAI(key, "https://openrouter.ai/api/v1", "x-ai/grok-4.6", 90*time.Second)
	p := &recPosterText{recPoster: recPoster{admins: map[int64]bool{99: true}}}
	cfg := config.Config{
		OpenAIAPIKey:       key,
		OpenAIModel:        "x-ai/grok-4.6",
		OpenAIBaseURL:      "https://openrouter.ai/api/v1",
		ContextMessages:    30,
		MinTextLen:         8,
		LLMTimeout:         90 * time.Second,
		ReactMinConfidence: 0.62,
		ReactMinAbsDelta:   80,
		ReactChatCooldown:  12 * time.Second,
		ReactUserCooldown:  90 * time.Second,
		ReactChatDailyCap:  80,
		TextMinConfidence:  0.78,
		TextMinAbsDelta:    500,
		TextChatCooldown:   720 * time.Second,
		TextUserCooldown:   10800 * time.Second,
		TextChatDailyCap:   4,
		AllowedChatID:      -100,
		AllowedChatTitle:   "Winter Arc 🪦",
	}
	app := New(cfg, st, j, p, nil, "Aura_radar_bot")
	ctx := context.Background()
	user := store.User{ID: 1, Username: "ilya", FirstName: "Илья"}

	logistics := Incoming{
		ChatID: -100, ChatType: "supergroup", ChatTitle: "Winter Arc 🪦",
		MessageID: 101, User: user, Text: "Так, я поставил агента варить бота, через час доеду до точки Б",
		Time: time.Now().UTC(),
	}
	if err := app.Handle(ctx, logistics); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.GetEvent(ctx, logistics.ChatID, 101); ok {
		t.Fatal("logistics should stay silent")
	}
	if len(p.reacts) != 0 || len(p.texts) != 0 {
		t.Fatalf("silent auto, got reacts=%v texts=%v", p.reacts, p.texts)
	}

	target := Incoming{
		ChatID: -100, ChatType: "supergroup", ChatTitle: "Winter Arc 🪦",
		MessageID: 102, User: user, Text: "зацените ауру, я сейчас очень сильно в образе",
		Time: time.Now().UTC(),
	}
	cmd := Incoming{
		ChatID: -100, ChatType: "supergroup", ChatTitle: "Winter Arc 🪦",
		MessageID: 103, User: store.User{ID: 2, FirstName: "Bob"}, Text: "/aura",
		Reply: &target, Time: time.Now().UTC(),
	}
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 1 || !strings.Contains(p.texts[0], "аура") {
		t.Fatalf("force evaluate reply %v", p.texts)
	}
	sc, ok, err := st.GetScore(ctx, target.ChatID, user.ID)
	if err != nil || !ok {
		t.Fatalf("score missing %+v %v", sc, err)
	}
	points := sc.Points
	nReplies := len(p.texts)
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	sc2, _, _ := st.GetScore(ctx, target.ChatID, user.ID)
	if sc2.Points != points {
		t.Fatalf("explain must not rescore %d -> %d", points, sc2.Points)
	}
	if len(p.texts) != nReplies+1 {
		t.Fatalf("explain should reply again, %v", p.texts)
	}

	if err := app.Handle(ctx, Incoming{
		ChatID: -100, ChatType: "supergroup", ChatTitle: "Winter Arc 🪦",
		MessageID: 104, User: user, Text: "/top", Time: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Handle(ctx, Incoming{
		ChatID: -100, ChatType: "supergroup", ChatTitle: "Winter Arc 🪦",
		MessageID: 105, User: store.User{ID: 3}, Text: "/radar off", Time: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	chat, _ := st.Chat(ctx, -100)
	if !chat.RadarEnabled {
		t.Fatal("non-admin toggled radar")
	}
	if err := app.Handle(ctx, Incoming{
		ChatID: 1, ChatType: "supergroup", ChatTitle: "other",
		MessageID: 1, User: user, Text: "/top", Time: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("live replies: %v score=%d", p.texts, points)
}
