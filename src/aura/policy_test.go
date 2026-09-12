package aura

import "testing"

func TestCheapFilters(t *testing.T) {
	base := FilterInput{
		ChatType:     "supergroup",
		Text:         "закрыл тему одной строкой",
		RadarEnabled: true,
		MinTextLen:   8,
	}
	if ok, _ := CheapFilters(base); !ok {
		t.Fatal("expected pass")
	}
	cases := []struct {
		name string
		mut  func(*FilterInput)
		want string
	}{
		{"private", func(in *FilterInput) { in.ChatType = "private" }, "not_group"},
		{"off", func(in *FilterInput) { in.RadarEnabled = false }, "radar_off"},
		{"bot", func(in *FilterInput) { in.FromBot = true }, "bot_author"},
		{"via", func(in *FilterInput) { in.ViaBot = true }, "via_bot_or_bot"},
		{"short", func(in *FilterInput) { in.Text = "ок" }, "short"},
		{"cmd", func(in *FilterInput) { in.Text = "/top" }, "command"},
		{"event", func(in *FilterInput) { in.HasEvent = true }, "has_event"},
		{"inflight", func(in *FilterInput) { in.InFlight = true }, "in_flight"},
	}
	for _, tc := range cases {
		in := base
		tc.mut(&in)
		ok, reason := CheapFilters(in)
		if ok {
			t.Fatalf("%s: expected reject", tc.name)
		}
		if tc.name == "via" && reason != "bot_author" {
			t.Fatalf("%s reason %s", tc.name, reason)
		}
		if tc.name != "via" && reason != tc.want {
			t.Fatalf("%s reason %s want %s", tc.name, reason, tc.want)
		}
	}
}

func TestDecideAuto(t *testing.T) {
	th := Thresholds{
		ReactMinConfidence: 0.62,
		ReactMinAbsDelta:   80,
		TextMinConfidence:  0.78,
		TextMinAbsDelta:    500,
	}
	cdOK := Cooldowns{ReactOK: true, TextOK: true}

	if d := DecideAuto(Verdict{Verdict: VerdictNeutral, Delta: 0, Confidence: 0.99}, th, cdOK); d != DecisionNone {
		t.Fatalf("neutral → none, got %d", d)
	}
	weakReact := Verdict{Verdict: VerdictWeak, Delta: -120, Confidence: 0.7}
	if d := DecideAuto(weakReact, th, cdOK); d != DecisionReact {
		t.Fatalf("weak signal → react, got %d", d)
	}
	legend := Verdict{Verdict: VerdictStrong, Delta: 900, Confidence: 0.9}
	if d := DecideAuto(legend, th, cdOK); d != DecisionReactAndText {
		t.Fatalf("legend → react+text, got %d", d)
	}
	if d := DecideAuto(legend, th, Cooldowns{ReactOK: true, TextOK: false}); d != DecisionReact {
		t.Fatalf("text cooldown → react only, got %d", d)
	}
	if d := DecideAuto(legend, th, Cooldowns{ReactOK: false, TextOK: true}); d != DecisionNone {
		t.Fatalf("react cooldown → none (no points), got %d", d)
	}
	lowConf := Verdict{Verdict: VerdictStrong, Delta: 400, Confidence: 0.5}
	if d := DecideAuto(lowConf, th, cdOK); d != DecisionNone {
		t.Fatalf("low conf → none, got %d", d)
	}
}

func TestClipAndReactionDefaults(t *testing.T) {
	if DefaultReaction(VerdictStrong, "💀") != "🗿" {
		t.Fatal("strong fallback")
	}
	if DefaultReaction(VerdictWeak, "😂") != "🤡" {
		t.Fatal("weak fallback")
	}
	if DefaultReaction(VerdictNeutral, "🔥") != "" {
		t.Fatal("neutral no reaction")
	}
	long := ""
	for i := 0; i < 300; i++ {
		long += "слово "
	}
	got := ClipComment(long)
	if len([]rune(got)) > MaxCommentRunes {
		t.Fatalf("comment too long: %d", len([]rune(got)))
	}
	if !BigReaction(800) || BigReaction(799) {
		t.Fatal("is_big threshold")
	}
}
