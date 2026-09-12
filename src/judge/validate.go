package judge

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Ra1ze505/aura-radar/src/aura"
)

type Result struct {
	Verdict    string  `json:"verdict"`
	Delta      int     `json:"delta"`
	Confidence float64 `json:"confidence"`
	Comment    string  `json:"comment"`
	Reaction   *string `json:"reaction"`
}

func ParseAndNormalize(raw string) (Result, error) {
	payload, err := extractJSON(raw)
	if err != nil {
		return Result{}, err
	}
	var r Result
	if err := json.Unmarshal(payload, &r); err != nil {
		return Result{}, fmt.Errorf("invalid judge json: %w", err)
	}
	r.Verdict = strings.ToLower(strings.TrimSpace(r.Verdict))
	switch r.Verdict {
	case aura.VerdictStrong, aura.VerdictWeak, aura.VerdictNeutral:
	default:
		return Result{}, fmt.Errorf("invalid verdict %q", r.Verdict)
	}
	if math.IsNaN(r.Confidence) || math.IsInf(r.Confidence, 0) {
		r.Confidence = 0
	}
	r.Confidence = aura.ClipConfidence(r.Confidence)
	r.Comment = aura.ClipComment(r.Comment)

	switch r.Verdict {
	case aura.VerdictNeutral:
		r.Delta = 0
	case aura.VerdictStrong:
		if r.Delta == 0 {
			r.Delta = 50
		}
		if r.Delta < 0 {
			r.Delta = -r.Delta
		}
	case aura.VerdictWeak:
		if r.Delta == 0 {
			r.Delta = -50
		}
		if r.Delta > 0 {
			r.Delta = -r.Delta
		}
	}
	if r.Delta > aura.MaxDeltaAbs {
		r.Delta = aura.MaxDeltaAbs
	}
	if r.Delta < -aura.MaxDeltaAbs {
		r.Delta = -aura.MaxDeltaAbs
	}

	emoji := ""
	if r.Reaction != nil {
		emoji = strings.TrimSpace(*r.Reaction)
	}
	emoji = aura.DefaultReaction(r.Verdict, emoji)
	if emoji == "" {
		r.Reaction = nil
	} else {
		r.Reaction = &emoji
	}
	if r.Verdict != aura.VerdictNeutral && strings.TrimSpace(r.Comment) == "" {
		r.Comment = fallbackComment(r.Verdict)
	}
	return r, nil
}

func (r Result) ReactionEmoji() string {
	if r.Reaction == nil {
		return ""
	}
	return *r.Reaction
}

func (r Result) ToVerdict() aura.Verdict {
	return aura.Verdict{
		Verdict:    r.Verdict,
		Delta:      r.Delta,
		Confidence: r.Confidence,
		Comment:    r.Comment,
		Reaction:   r.ReactionEmoji(),
	}
}

func extractJSON(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no json object in judge response")
	}
	return []byte(s[start : end+1]), nil
}

func fallbackComment(verdict string) string {
	if verdict == aura.VerdictStrong {
		return "ход зашёл."
	}
	return "ход не зашёл."
}
