package judge

import (
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	raw := `{"verdict":"strong","delta":350,"confidence":0.9,"comment":"коротко закрыл тему.","reaction":"🗿"}`
	r, err := ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "strong" || r.Delta != 350 || r.ReactionEmoji() != "🗿" {
		t.Fatalf("%+v", r)
	}
}

func TestParseBroken(t *testing.T) {
	if _, err := ParseAndNormalize("not json"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseAndNormalize(`{"delta":1}`); err == nil {
		t.Fatal("missing verdict")
	}
}

func TestSignConflictAndClip(t *testing.T) {
	raw := `{"verdict":"strong","delta":-120,"confidence":1.5,"comment":"x","reaction":"💀"}`
	r, err := ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delta != 120 {
		t.Fatalf("sign normalize: %d", r.Delta)
	}
	if r.Confidence != 1 {
		t.Fatalf("conf clip: %v", r.Confidence)
	}
	if r.ReactionEmoji() != "🗿" {
		t.Fatalf("reaction fallback %q", r.ReactionEmoji())
	}

	raw = `{"verdict":"weak","delta":800,"confidence":0.4,"comment":"y","reaction":null}`
	r, err = ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delta != -800 {
		t.Fatalf("weak sign %d", r.Delta)
	}
	if r.ReactionEmoji() != "🤡" {
		t.Fatalf("weak reaction %q", r.ReactionEmoji())
	}

	raw = `{"verdict":"strong","delta":9000,"confidence":0.8,"comment":"z","reaction":"🔥"}`
	r, err = ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delta != 5000 {
		t.Fatalf("clip %d", r.Delta)
	}
}

func TestLongCommentWordBoundary(t *testing.T) {
	comment := strings.Repeat("слово ", 80)
	raw := `{"verdict":"weak","delta":-100,"confidence":0.7,"comment":"` + comment + `","reaction":"🤡"}`
	r, err := ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(r.Comment)) > 240 {
		t.Fatalf("len %d", len([]rune(r.Comment)))
	}
	if strings.Contains(r.Comment, "...") {
		t.Fatal("must not append ellipsis essay")
	}
}

func TestExtractJSONFence(t *testing.T) {
	raw := "```json\n{\"verdict\":\"neutral\",\"delta\":10,\"confidence\":0.2,\"comment\":\"быт\",\"reaction\":null}\n```"
	r, err := ParseAndNormalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "neutral" || r.Delta != 0 {
		t.Fatalf("%+v", r)
	}
	if r.Reaction != nil {
		t.Fatalf("neutral reaction %+v", r.Reaction)
	}
}
