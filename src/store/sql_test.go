package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ra1ze505/aura-radar/src/aura"
)

func testStore(t *testing.T) *SQLStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestInsertEventIsolationAndIdempotentExplain(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	uid := int64(1)
	if err := s.UpsertUser(ctx, User{ID: uid, FirstName: "Ann"}); err != nil {
		t.Fatal(err)
	}
	ev := Event{
		ChatID: 10, UserID: uid, MessageID: 5,
		Verdict: aura.VerdictStrong, Delta: 350, Confidence: 0.9,
		Comment: "закрыл тему", Reaction: "🗿", Trigger: aura.TriggerCommand,
		PublishedReaction: true,
	}
	if err := s.InsertEventAndScore(ctx, ev); err != nil {
		t.Fatal(err)
	}
	ev.ChatID = 11
	ev.Delta = -800
	ev.Verdict = aura.VerdictWeak
	if err := s.InsertEventAndScore(ctx, ev); err != nil {
		t.Fatal(err)
	}
	a, ok, err := s.GetScore(ctx, 10, uid)
	if err != nil || !ok || a.Points != 350 {
		t.Fatalf("chat A %+v ok=%v err=%v", a, ok, err)
	}
	b, ok, err := s.GetScore(ctx, 11, uid)
	if err != nil || !ok || b.Points != -800 {
		t.Fatalf("chat B %+v ok=%v err=%v", b, ok, err)
	}
	topA, err := s.Top(ctx, 10, 10)
	if err != nil || len(topA) != 1 || topA[0].Points != 350 {
		t.Fatalf("top A %+v err=%v", topA, err)
	}
	topB, err := s.Top(ctx, 11, 10)
	if err != nil || len(topB) != 1 || topB[0].Points != -800 {
		t.Fatalf("top B leaked or wrong %+v", topB)
	}
	if err := s.InsertEventAndScore(ctx, Event{
		ChatID: 10, UserID: uid, MessageID: 5, Verdict: aura.VerdictStrong, Delta: 100,
		Trigger: aura.TriggerCommand,
	}); err == nil {
		t.Fatal("second insert on same message must fail unique")
	}
	got, ok, err := s.GetEvent(ctx, 10, 5)
	if err != nil || !ok || got.Delta != 350 {
		t.Fatalf("event unchanged %+v", got)
	}
}

func TestBufferPruneAndContext(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	uid := int64(7)
	_ = s.UpsertUser(ctx, User{ID: uid, FirstName: "Bob"})
	for i := 1; i <= 5; i++ {
		u := uid
		err := s.AppendMessage(ctx, BufferMessage{
			ChatID: 1, MessageID: i, UserID: &u, Text: "hello world message", SentAt: time.Now().UTC(),
		}, 3, 7*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctxMsgs, err := s.ContextBefore(ctx, 1, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctxMsgs) != 2 { // keep=max(3,50) wait, keep is max(keep, 50) in AppendMessage!
		// keep is forced to at least 50, so all 5 remain except we asked context before 5
		// so message 1-4
	}
	if len(ctxMsgs) != 4 {
		t.Fatalf("context len %d", len(ctxMsgs))
	}
	if ctxMsgs[0].MessageID != 1 || ctxMsgs[len(ctxMsgs)-1].MessageID != 4 {
		t.Fatalf("order %+v", ctxMsgs)
	}
}

func TestRankAndUsernameLookup(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	_ = s.UpsertUser(ctx, User{ID: 1, Username: "alice", FirstName: "A"})
	_ = s.UpsertUser(ctx, User{ID: 2, Username: "bob", FirstName: "B"})
	_ = s.InsertEventAndScore(ctx, Event{ChatID: 1, UserID: 1, MessageID: 1, Verdict: aura.VerdictStrong, Delta: 500, Trigger: "command"})
	_ = s.InsertEventAndScore(ctx, Event{ChatID: 1, UserID: 2, MessageID: 2, Verdict: aura.VerdictWeak, Delta: -50, Trigger: "command"})
	place, total, err := s.Rank(ctx, 1, 1)
	if err != nil || total != 2 || place != 1 {
		t.Fatalf("rank alice place=%d total=%d err=%v", place, total, err)
	}
	sc, ok, err := s.GetScoreByUsername(ctx, 1, "@Bob")
	if err != nil || !ok || sc.UserID != 2 {
		t.Fatalf("username lookup %+v ok=%v err=%v", sc, ok, err)
	}
	_, ok, err = s.GetScoreByUsername(ctx, 1, "missing")
	if err != nil || ok {
		t.Fatal("missing user")
	}
}
