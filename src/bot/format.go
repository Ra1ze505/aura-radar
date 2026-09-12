package bot

import (
	"fmt"
	"strings"

	"github.com/Ra1ze505/aura-radar/src/aura"
	"github.com/Ra1ze505/aura-radar/src/store"
)

func formatDelta(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	if n < 0 {
		return fmt.Sprintf("−%d", -n)
	}
	return "+0"
}

func verdictLabel(verdict string) string {
	switch verdict {
	case aura.VerdictStrong:
		return "сильная аура"
	case aura.VerdictWeak:
		return "слабая аура"
	default:
		return "нейтрально"
	}
}

func cardEmoji(verdict, reaction string) string {
	if strings.TrimSpace(reaction) != "" {
		return reaction
	}
	switch verdict {
	case aura.VerdictStrong:
		return "🗿"
	case aura.VerdictWeak:
		return "💀"
	default:
		return "🌚"
	}
}

func FormatCard(verdict string, delta int, reaction, comment string) string {
	body := strings.TrimSpace(comment)
	head := fmt.Sprintf("%s %s · %s", cardEmoji(verdict, reaction), verdictLabel(verdict), formatDelta(delta))
	if body == "" {
		return head
	}
	return head + "\n\n" + body
}

func FormatScore(name string, sc store.Score, place, total int, found bool) string {
	if !found || (sc.StrongCount+sc.WeakCount) == 0 {
		return fmt.Sprintf("%s: %s", name, MsgNoScore)
	}
	placePart := "место: нет в рейтинге"
	if place > 0 && total > 0 {
		placePart = fmt.Sprintf("место: %d из %d", place, total)
	}
	return fmt.Sprintf("%s: %s · %d сильных / %d слабых\n%s",
		name, formatDelta(sc.Points), sc.StrongCount, sc.WeakCount, placePart)
}

func FormatTop(rows []store.Score) string {
	if len(rows) == 0 {
		return MsgTopEmpty
	}
	var b strings.Builder
	b.WriteString("топ ауры чата:\n")
	for i, sc := range rows {
		fmt.Fprintf(&b, "%d. %s · %s · %d↑ %d↓\n", i+1, store.DisplayName(store.User{
			ID: sc.UserID, Username: sc.Username, FirstName: sc.FirstName, LastName: sc.LastName,
		}), formatDelta(sc.Points), sc.StrongCount, sc.WeakCount)
	}
	return strings.TrimSpace(b.String())
}
