package judge

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Ra1ze505/aura-radar/prompts"
	"github.com/Ra1ze505/aura-radar/src/aura"
	"github.com/Ra1ze505/aura-radar/src/store"
)

type PromptInput struct {
	ChatTitle string
	Target    store.BufferMessage
	Context   []store.BufferMessage
}

func SystemPrompt() string {
	return strings.TrimSpace(prompts.Judge)
}

func BuildUserPrompt(in PromptInput) string {
	var b strings.Builder
	title := strings.TrimSpace(in.ChatTitle)
	if title == "" {
		title = "группа"
	}
	fmt.Fprintf(&b, "Чат: %s\n", title)
	b.WriteString("Контекст (старые → новые):\n")
	if len(in.Context) == 0 {
		b.WriteString("(пусто)\n")
	}
	for i, msg := range in.Context {
		same := ""
		if msg.UserID != nil && in.Target.UserID != nil && *msg.UserID == *in.Target.UserID {
			same = " (автор)"
		}
		fmt.Fprintf(&b, "%d. %s%s: %s\n", i+1, displayOf(msg), same, aura.ClipMessage(msg.Text))
	}
	b.WriteString("\nЦелевое:\n")
	fmt.Fprintf(&b, "Автор: %s\n", displayOf(in.Target))
	fmt.Fprintf(&b, "Текст: %s\n", aura.ClipMessage(in.Target.Text))
	out := b.String()
	if utf8.RuneCountInString(out)+utf8.RuneCountInString(SystemPrompt()) > aura.MaxPromptChars {
		// drop oldest context lines until it fits
		for len(in.Context) > 0 && utf8.RuneCountInString(out)+utf8.RuneCountInString(SystemPrompt()) > aura.MaxPromptChars {
			in.Context = in.Context[1:]
			out = BuildUserPrompt(in)
		}
	}
	return out
}

func displayOf(msg store.BufferMessage) string {
	if strings.TrimSpace(msg.Display) != "" {
		return strings.TrimSpace(msg.Display)
	}
	return "участник"
}
