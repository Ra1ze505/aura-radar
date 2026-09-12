package bot

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func parseCommand(text, botUsername string) (cmd, payload string, isCmd bool) {
	s := strings.TrimSpace(text)
	if !strings.HasPrefix(s, "/") {
		return "", "", false
	}
	s = strings.TrimPrefix(s, "/")
	parts := strings.SplitN(s, " ", 2)
	name := parts[0]
	if i := strings.Index(name, "@"); i >= 0 {
		mention := name[i+1:]
		name = name[:i]
		if botUsername != "" && !strings.EqualFold(mention, strings.TrimPrefix(botUsername, "@")) {
			return "", "", true // command for another bot
		}
	}
	name = strings.ToLower(name)
	if payload = ""; len(parts) == 2 {
		payload = strings.TrimSpace(parts[1])
	}
	return name, payload, true
}

func parseUsername(payload string) string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ""
	}
	fields := strings.Fields(payload)
	if len(fields) == 0 {
		return ""
	}
	u := strings.TrimPrefix(fields[0], "@")
	if u == "" {
		return ""
	}
	for _, r := range u {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return ""
	}
	return u
}

func firstNonEmptyText(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func looksLikeCommand(text string) bool {
	s := strings.TrimSpace(text)
	return strings.HasPrefix(s, "/")
}

func runeCount(s string) int { return utf8.RuneCountInString(s) }
