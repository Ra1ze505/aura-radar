package bot

import (
	"bytes"
	"context"
	"io"
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
		STTTimeout:         40 * time.Second,
		MediaMaxSec:        90,
		MediaMaxBytes:      20_000_000,
		TranscribeModel:    "whisper-test",
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

type stubOpen struct {
	calls int
	err   error
	data  []byte
}

func (o *stubOpen) OpenMedia(context.Context, string) (io.ReadCloser, error) {
	o.calls++
	if o.err != nil {
		return nil, o.err
	}
	if o.data == nil {
		o.data = []byte("ogg")
	}
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

type stubSTT struct {
	calls    int
	text     string
	err      error
	filename string
}

func (s *stubSTT) Transcribe(_ context.Context, _ io.Reader, filename, _ string) (string, error) {
	s.calls++
	s.filename = filename
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

type recJudge struct {
	last   judge.PromptInput
	Result judge.Result
	Err    error
}

func (j *recJudge) Evaluate(_ context.Context, in judge.PromptInput) (judge.Result, error) {
	j.last = in
	return j.Result, j.Err
}

func mediaWinter(id int, user store.User, kind, fileID string) Incoming {
	in := winter(id, user, "")
	in.MediaKind = kind
	in.FileID = fileID
	in.Duration = 4
	in.FileSize = 1200
	return in
}

func TestVoiceAutoTranscribesAndJudges(t *testing.T) {
	emoji := "🗿"
	j := &recJudge{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 200, Confidence: 0.85,
		Comment: "зашло голосом.", Reaction: &emoji,
	}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	open := &stubOpen{}
	stt := &stubSTT{text: "закрыл тему одной фразой в голосовом"}
	app.SetSpeech(open, stt)
	ctx := context.Background()
	in := mediaWinter(20, store.User{ID: 11, FirstName: "V"}, MediaVoice, "voice-1")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	if stt.calls != 1 || open.calls != 1 {
		t.Fatalf("stt=%d open=%d", stt.calls, open.calls)
	}
	if !strings.HasPrefix(j.last.Target.Text, "[голос] ") {
		t.Fatalf("judge text %q", j.last.Target.Text)
	}
	if len(p.reacts) != 1 {
		t.Fatalf("reacts %v", p.reacts)
	}
	_, ok, _ := st.GetEvent(ctx, -100, 20)
	if !ok {
		t.Fatal("expected auto event")
	}
	buf, err := st.ContextBefore(ctx, -100, 21, 10)
	if err != nil || len(buf) != 1 || !strings.Contains(buf[0].Text, "[голос]") {
		t.Fatalf("buffer %+v err=%v", buf, err)
	}
}

func TestCircleAndVideoPrefixes(t *testing.T) {
	emoji := "🤡"
	j := &recJudge{Result: judge.Result{
		Verdict: aura.VerdictWeak, Delta: -120, Confidence: 0.8,
		Comment: "кружок мимо.", Reaction: &emoji,
	}}
	p := &recPosterText{}
	app, _ := setupApp(t, j, p)
	stt := &stubSTT{text: "ща объясню этот мем всем в кружке"}
	app.SetSpeech(&stubOpen{}, stt)
	ctx := context.Background()
	circle := mediaWinter(21, store.User{ID: 12, FirstName: "C"}, MediaCircle, "vn-1")
	if err := app.Handle(ctx, circle); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(j.last.Target.Text, "[кружок] ") {
		t.Fatalf("circle %q", j.last.Target.Text)
	}
	app.now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 20, 0, time.UTC) }
	stt.text = "коротко закрыл тему на видео да"
	video := mediaWinter(22, store.User{ID: 13, FirstName: "D"}, MediaVideo, "vid-1")
	if err := app.Handle(ctx, video); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(j.last.Target.Text, "[видео] ") {
		t.Fatalf("video %q", j.last.Target.Text)
	}
}

func TestAuraEmptyTranscriptNoScore(t *testing.T) {
	j := &recJudge{Result: judge.Result{Verdict: aura.VerdictStrong, Delta: 400, Confidence: 0.9, Comment: "x"}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	stt := &stubSTT{text: "   "}
	app.SetSpeech(&stubOpen{}, stt)
	ctx := context.Background()
	target := mediaWinter(30, store.User{ID: 1, FirstName: "A"}, MediaVoice, "v")
	cmd := winter(31, store.User{ID: 2, FirstName: "B"}, "/aura")
	cmd.Reply = &target
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 1 || p.texts[0] != MsgNoVoice {
		t.Fatalf("replies %q", p.texts)
	}
	if _, ok, _ := st.GetEvent(ctx, -100, 30); ok {
		t.Fatal("no event on empty stt")
	}
	if _, ok, _ := st.GetScore(ctx, -100, 1); ok {
		t.Fatal("no score on empty stt")
	}
}

func TestAutoEmptySTTSilent(t *testing.T) {
	j := &recJudge{Result: judge.Result{Verdict: aura.VerdictStrong, Delta: 200, Confidence: 0.9, Comment: "x"}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	stt := &stubSTT{text: ""}
	app.SetSpeech(&stubOpen{}, stt)
	ctx := context.Background()
	in := mediaWinter(32, store.User{ID: 9, FirstName: "E"}, MediaVoice, "v2")
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	if stt.calls != 1 {
		t.Fatalf("stt calls %d", stt.calls)
	}
	if len(p.reacts) != 0 || len(p.texts) != 0 {
		t.Fatalf("must be silent reacts=%v texts=%v", p.reacts, p.texts)
	}
	if _, ok, _ := st.GetEvent(ctx, -100, 32); ok {
		t.Fatal("no auto event")
	}
}

func TestVideoCaptionSkipsSTT(t *testing.T) {
	emoji := "🗿"
	j := &recJudge{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 150, Confidence: 0.8, Comment: "подпись.", Reaction: &emoji,
	}}
	p := &recPosterText{}
	app, _ := setupApp(t, j, p)
	stt := &stubSTT{text: "это не должно вызваться"}
	app.SetSpeech(&stubOpen{}, stt)
	ctx := context.Background()
	in := mediaWinter(33, store.User{ID: 4, FirstName: "F"}, MediaVideo, "vid")
	in.Text = "достаточно длинная подпись к ролику"
	if err := app.Handle(ctx, in); err != nil {
		t.Fatal(err)
	}
	if stt.calls != 0 {
		t.Fatalf("stt should skip caption, calls=%d", stt.calls)
	}
	if j.last.Target.Text != in.Text {
		t.Fatalf("caption lost: %q", j.last.Target.Text)
	}
}

func TestMediaTooLongAuraNoDownload(t *testing.T) {
	p := &recPosterText{}
	app, _ := setupApp(t, &recJudge{}, p)
	open := &stubOpen{}
	app.SetSpeech(open, &stubSTT{text: "длинный монолог который не должны качать"})
	ctx := context.Background()
	target := mediaWinter(34, store.User{ID: 1, FirstName: "A"}, MediaVoice, "long")
	target.Duration = 120
	cmd := winter(35, store.User{ID: 2}, "/aura")
	cmd.Reply = &target
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if open.calls != 0 {
		t.Fatalf("must not download, open=%d", open.calls)
	}
	if len(p.texts) != 1 || p.texts[0] != MsgNoVoice {
		t.Fatalf("%q", p.texts)
	}
}

func TestAuraVoiceEvaluatesTranscript(t *testing.T) {
	emoji := "🔥"
	j := &recJudge{Result: judge.Result{
		Verdict: aura.VerdictStrong, Delta: 350, Confidence: 0.92,
		Comment: "голос в точку.", Reaction: &emoji,
	}}
	p := &recPosterText{}
	app, st := setupApp(t, j, p)
	app.SetSpeech(&stubOpen{}, &stubSTT{text: "закрыл тему коротко и ясно голосом"})
	ctx := context.Background()
	target := mediaWinter(36, store.User{ID: 1, FirstName: "A"}, MediaVoice, "v")
	cmd := winter(37, store.User{ID: 2, FirstName: "B"}, "/aura")
	cmd.Reply = &target
	if err := app.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.last.Target.Text, "[голос]") {
		t.Fatalf("prompt %q", j.last.Target.Text)
	}
	sc, ok, _ := st.GetScore(ctx, -100, 1)
	if !ok || sc.Points != 350 {
		t.Fatalf("score %+v ok=%v", sc, ok)
	}
	if len(p.texts) != 1 || !strings.Contains(p.texts[0], "+350") {
		t.Fatalf("reply %q", p.texts)
	}
}
