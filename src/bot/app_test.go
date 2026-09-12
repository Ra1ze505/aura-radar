package bot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ra1ze505/aura-radar/src/aura"
	"github.com/Ra1ze505/aura-radar/src/config"
	"github.com/Ra1ze505/aura-radar/src/judge"
	"github.com/Ra1ze505/aura-radar/src/store"
)

type recPoster struct {
	replies   []string
	reacts    []string
	admins    map[int64]bool
	failReact bool
}

func (p *recPoster) Reply(context.Context, int64, int, int, string) error {
	return nil
}

func (p *recPoster) React(_ context.Context, _ int64, _ int, emoji string, _ bool) error {
	if p.failReact {
		return context.DeadlineExceeded
	}
	p.reacts = append(p.reacts, emoji)
	return nil
}

func (p *recPoster) IsAdmin(_ context.Context, _, userID int64) (bool, error) {
	return p.admins[userID], nil
}

type recPosterText struct {
	recPoster
	texts []string
}

func (p *recPosterText) Reply(_ context.Context, _ int64, _, _ int, text string) error {
	p.texts = append(p.texts, text)
	return nil
}

func setupApp(t *testing.T, j judge.Judge, poster Poster) (*App, store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{
		OpenAIAPIKey:       "x",
		OpenAIModel:        "m",
		ContextMessages:    30,
		MinTextLen:         8,
		LLMTimeout:         time.Second,
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
		AllowedChatTitle:   "Winter Arc 🪦",
	}
	app := New(cfg, st, j, poster, nil, "Aura_radar_bot")
	app.now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) }
	return app, st
}

func winter(id int, user store.User, text string) Incoming {
	return Incoming{
		ChatID:    -100,
		ChatType:  "supergroup",
		ChatTitle: "Winter Arc 🪦",
		MessageID: id,
		User:      user,
		Text:      text,
		Time:      time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC),
	}
}

func TestAuraEvaluateThenExplainNoSecondDelta(t *testing.T) {
	emoji := "🗿"
	j := judge.Static{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 350, Confidence: 0.9,
		Comment: "закрыл тему.", Reaction: &emoji,
	}}
	p := &recPosterText{recPoster: recPoster{admins: map[int64]bool{}}}
	app, st := setupApp(t, j, p)
	ctx := context.Background()
	alice := store.User{ID: 1, FirstName: "Alice"}
	target := winter(1, alice, "коротко закрыл тему одной строкой")
	cmd := winter(2, store.User{ID: 2, FirstName: "Bob"}, "/aura")
	cmd.Reply = &target
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	sc, ok, err := st.GetScore(ctx, -100, 1)
	if err != nil || !ok || sc.Points != 350 {
		t.Fatalf("score %+v ok=%v err=%v", sc, ok, err)
	}
	if len(p.texts) != 1 || !strings.Contains(p.texts[0], "сильная аура") || !strings.Contains(p.texts[0], "+350") {
		t.Fatalf("first reply %q", p.texts)
	}
	j.Result.Delta = 999
	app.judge = j
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	sc, _, err = st.GetScore(ctx, -100, 1)
	if err != nil || sc.Points != 350 {
		t.Fatalf("second delta applied: %+v", sc)
	}
	if len(p.texts) != 2 || !strings.Contains(p.texts[1], "+350") {
		t.Fatalf("explain %q", p.texts)
	}
}

func TestRadarNonAdminCannotToggle(t *testing.T) {
	p := &recPosterText{recPoster: recPoster{admins: map[int64]bool{99: true}}}
	app, st := setupApp(t, judge.Static{}, p)
	ctx := context.Background()
	in := winter(3, store.User{ID: 1, FirstName: "User"}, "/radar off")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	chat, err := st.Chat(ctx, -100)
	if err != nil || !chat.RadarEnabled {
		t.Fatalf("radar should stay on: %+v err=%v", chat, err)
	}
	if len(p.texts) != 1 || p.texts[0] != MsgNotAdmin {
		t.Fatalf("replies %q", p.texts)
	}
	in.User.ID = 99
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	chat, err = st.Chat(ctx, -100)
	if err != nil || chat.RadarEnabled {
		t.Fatalf("admin should disable: %+v", chat)
	}
}

func TestAutoNeutralSilentAndNoEvent(t *testing.T) {
	j := judge.Static{Result: judge.Result{Verdict: aura.VerdictNeutral, Confidence: 0.99, Comment: "быт"}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	ctx := context.Background()
	in := winter(4, store.User{ID: 3, FirstName: "C"}, "ща приду на работу потом")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	_, ok, err := st.GetEvent(ctx, -100, 4)
	if err != nil || ok {
		t.Fatal("neutral auto must not store event")
	}
	if len(p.reacts) != 0 || len(p.texts) != 0 {
		t.Fatalf("must be silent reacts=%v texts=%v", p.reacts, p.texts)
	}
}

func TestAutoWeakReactOnlyLegendTextAndReactCooldownBlocksScore(t *testing.T) {
	emoji := "🤡"
	j := judge.Static{Result: judge.Result{
		Verdict: aura.VerdictWeak, Delta: -120, Confidence: 0.7,
		Comment: "объяснил шутку.", Reaction: &emoji,
	}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	ctx := context.Background()
	in := winter(5, store.User{ID: 4, FirstName: "D"}, "ща объясню этот мем подробно всем")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	if len(p.reacts) != 1 || len(p.texts) != 0 {
		t.Fatalf("weak → react only, got reacts=%v texts=%v", p.reacts, p.texts)
	}
	sc, ok, _ := st.GetScore(ctx, -100, 4)
	if !ok || sc.Points != -120 {
		t.Fatalf("score %+v", sc)
	}

	fire := "🔥"
	app.judge = judge.Static{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 900, Confidence: 0.95,
		Comment: "легенда.", Reaction: &fire,
	}}
	in2 := winter(6, store.User{ID: 5, FirstName: "E"}, "одна строка и тема закрыта навсегда")
	if err := app.Handle(ctx, in2); err != nil {
		t.Fatal(err)
	}
	if len(p.reacts) != 1 {
		t.Fatalf("react cooldown should block second auto, reacts=%v", p.reacts)
	}
	_, ok, _ = st.GetEvent(ctx, -100, 6)
	if ok {
		t.Fatal("no event when react cooldown blocks")
	}

	app.now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 20, 0, time.UTC) }
	if err := app.Handle(ctx, in2); err != nil {
		t.Fatal(err)
	}
	if len(p.reacts) != 2 || len(p.texts) != 1 {
		t.Fatalf("legend after cooldown reacts=%v texts=%v", p.reacts, p.texts)
	}
}

func TestAutoReactFailureDoesNotCommit(t *testing.T) {
	emoji := "🗿"
	j := judge.Static{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 200, Confidence: 0.8, Comment: "ok", Reaction: &emoji,
	}}
	p := &recPosterText{recPoster: recPoster{failReact: true}}
	app, st := setupApp(t, j, p)
	ctx := context.Background()
	in := winter(7, store.User{ID: 8, FirstName: "F"}, "короткий уверенный ход в чат")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := st.GetEvent(ctx, -100, 7)
	if ok {
		t.Fatal("failed react must not commit")
	}
}

func TestIsolationCommandsAndAllowedChat(t *testing.T) {
	p := &recPosterText{}
	app, _ := setupApp(t, judge.Static{}, p)
	ctx := context.Background()
	other := Incoming{ChatID: 1, ChatType: "supergroup", ChatTitle: "other", MessageID: 1, User: store.User{ID: 1}, Text: "/top"}
	if err := app.Handle(ctx, other); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 0 {
		t.Fatalf("other chat should be ignored: %v", p.texts)
	}
	priv := Incoming{ChatID: 9, ChatType: "private", MessageID: 1, User: store.User{ID: 1}, Text: "/aura"}
	if err := app.Handle(ctx, priv); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 1 || p.texts[0] != MsgGroupOnly {
		t.Fatalf("private aura %v", p.texts)
	}
}

func TestBlindCommand(t *testing.T) {
	p := &recPosterText{}
	app, _ := setupApp(t, nil, p)
	app.cfg.OpenAIAPIKey = ""
	ctx := context.Background()
	target := winter(1, store.User{ID: 1, FirstName: "A"}, "есть текст достаточно длинный")
	cmd := winter(2, store.User{ID: 2}, "/aura")
	cmd.Reply = &target
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 1 || p.texts[0] != MsgBlind {
		t.Fatalf("%v", p.texts)
	}
}

func TestTopAndAuraUsername(t *testing.T) {
	emoji := "🗿"
	j := judge.Static{Result: judge.Result{Verdict: aura.VerdictStrong, Delta: 200, Confidence: 0.9, Comment: "x", Reaction: &emoji}}
	p := &recPosterText{}
	app, _ := setupApp(t, j, p)
	ctx := context.Background()
	alice := store.User{ID: 1, Username: "alice", FirstName: "Alice"}
	msg := winter(1, alice, "закрыл тему коротко и ясно да")
	cmd := winter(2, store.User{ID: 2, FirstName: "Bob"}, "/aura")
	cmd.Reply = &msg
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if err := app.Handle(ctx, winter(3, alice, "/top")); err != nil {
		t.Fatal(err)
	}
	if err := app.Handle(ctx, winter(4, store.User{ID: 2}, "/aura @alice")); err != nil {
		t.Fatal(err)
	}
	if err := app.Handle(ctx, winter(5, store.User{ID: 2}, "/aura @nobody")); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(p.texts, "\n---\n")
	if !strings.Contains(joined, "топ ауры") || !strings.Contains(joined, "Alice") {
		t.Fatalf("top missing: %s", joined)
	}
	if !strings.Contains(joined, MsgUnknownUser) {
		t.Fatalf("missing nobody: %s", joined)
	}
}

func TestFormatCard(t *testing.T) {
	s := FormatCard(aura.VerdictWeak, -800, "🤡", "объяснил шутку. шутка не выжила.")
	if !strings.Contains(s, "слабая аура") || !strings.Contains(s, "−800") {
		t.Fatal(s)
	}
}
