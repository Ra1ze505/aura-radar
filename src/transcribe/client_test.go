package transcribe

import (
	"strings"
	"testing"
)

func TestWhisperHintHasPainVocab(t *testing.T) {
	if !strings.Contains(whisperHint, "Pain") || !strings.Contains(whisperHint, "боль") {
		t.Fatalf("hint %q", whisperHint)
	}
}
