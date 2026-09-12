package aura

import (
	"math"
	"strings"
	"time"
	"unicode"
)

const (
	VerdictStrong  = "strong"
	VerdictWeak    = "weak"
	VerdictNeutral = "neutral"

	TriggerCommand = "command"
	TriggerAuto    = "auto"

	MaxDeltaAbs      = 5000
	MaxCommentRunes  = 240
	MaxMessageRunes  = 500
	MaxPromptChars   = 12000
	BigReactionDelta = 800
)

var AllowedReactions = map[string]struct{}{
	"🔥": {},
	"🗿": {},
	"🤡": {},
	"🤣": {},
	"🌚": {},
	"👀": {},
}

type Thresholds struct {
	MinTextLen         int
	ReactMinConfidence float64
	ReactMinAbsDelta   int
	TextMinConfidence  float64
	TextMinAbsDelta    int
}

type Cooldowns struct {
	ReactOK bool
	TextOK  bool
}

type Decision int

const (
	DecisionNone Decision = iota
	DecisionReact
	DecisionReactAndText
)

type Verdict struct {
	Verdict    string
	Delta      int
	Confidence float64
	Comment    string
	Reaction   string
}

type FilterInput struct {
	ChatType     string
	FromBot      bool
	ViaBot       bool
	Text         string
	IsCommand    bool
	HasEvent     bool
	RadarEnabled bool
	InFlight     bool
	MinTextLen   int
}

func CheapFilters(in FilterInput) (ok bool, reason string) {
	if in.ChatType != "group" && in.ChatType != "supergroup" {
		return false, "not_group"
	}
	if !in.RadarEnabled {
		return false, "radar_off"
	}
	if in.FromBot || in.ViaBot {
		return false, "bot_author"
	}
	text := strings.TrimSpace(in.Text)
	if in.IsCommand || strings.HasPrefix(text, "/") {
		return false, "command"
	}
	if runeLen(text) < in.MinTextLen {
		return false, "short"
	}
	if in.HasEvent {
		return false, "has_event"
	}
	if in.InFlight {
		return false, "in_flight"
	}
	return true, ""
}

func DecideAuto(v Verdict, t Thresholds, cd Cooldowns) Decision {
	if v.Verdict == VerdictNeutral {
		return DecisionNone
	}
	if v.Confidence < t.ReactMinConfidence {
		return DecisionNone
	}
	if absInt(v.Delta) < t.ReactMinAbsDelta {
		return DecisionNone
	}
	if !cd.ReactOK {
		return DecisionNone
	}
	text := v.Confidence >= t.TextMinConfidence &&
		absInt(v.Delta) >= t.TextMinAbsDelta &&
		cd.TextOK
	if text {
		return DecisionReactAndText
	}
	return DecisionReact
}

func AbsInt(n int) int { return absInt(n) }

func BigReaction(delta int) bool {
	return absInt(delta) >= BigReactionDelta
}

func DefaultReaction(verdict, emoji string) string {
	if verdict == VerdictNeutral {
		return ""
	}
	if _, ok := AllowedReactions[emoji]; ok {
		return emoji
	}
	if verdict == VerdictStrong {
		return "🗿"
	}
	if verdict == VerdictWeak {
		return "🤡"
	}
	return ""
}

func ClipComment(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= MaxCommentRunes {
		return s
	}
	cut := runes[:MaxCommentRunes]
	i := len(cut) - 1
	for i > 0 && !unicode.IsSpace(cut[i]) {
		i--
	}
	if i > MaxCommentRunes/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(string(cut))
}

func ClipMessage(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= MaxMessageRunes {
		return s
	}
	return string(runes[:MaxMessageRunes])
}

func DayStartUTC(now time.Time) time.Time {
	t := now.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func runeLen(s string) int { return len([]rune(s)) }

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func ClipConfidence(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
