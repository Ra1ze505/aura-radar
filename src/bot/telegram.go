package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Ra1ze505/aura-radar/src/store"
	tele "gopkg.in/telebot.v4"
)

type TelePoster struct {
	bot   *tele.Bot
	mu    sync.Mutex
	admin map[adminKey]adminEntry
}

type adminKey struct {
	chatID int64
	userID int64
}

type adminEntry struct {
	ok      bool
	expires time.Time
}

func NewTelePoster(bot *tele.Bot) *TelePoster {
	return &TelePoster{bot: bot, admin: make(map[adminKey]adminEntry)}
}

func (p *TelePoster) Reply(_ context.Context, chatID int64, replyTo, threadID int, text string) error {
	opts := &tele.SendOptions{
		ReplyTo:           &tele.Message{ID: replyTo, Chat: &tele.Chat{ID: chatID}},
		AllowWithoutReply: true,
		ThreadID:          threadID,
	}
	_, err := p.bot.Send(&tele.Chat{ID: chatID}, text, opts)
	return err
}

func (p *TelePoster) React(_ context.Context, chatID int64, messageID int, emoji string, big bool) error {
	if emoji == "" {
		return nil
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
		"is_big":     big,
	}
	_, err := p.bot.Raw("setMessageReaction", payload)
	if err != nil {
		return fmt.Errorf("setMessageReaction: %w", err)
	}
	return err
}

func (p *TelePoster) IsAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	p.mu.Lock()
	if e, ok := p.admin[adminKey{chatID, userID}]; ok && time.Now().Before(e.expires) {
		p.mu.Unlock()
		return e.ok, nil
	}
	p.mu.Unlock()

	member, err := p.bot.ChatMemberOf(&tele.Chat{ID: chatID}, &tele.User{ID: userID})
	if err != nil {
		return false, err
	}
	ok := member.Role == tele.Creator || member.Role == tele.Administrator
	p.mu.Lock()
	p.admin[adminKey{chatID, userID}] = adminEntry{ok: ok, expires: time.Now().Add(5 * time.Minute)}
	p.mu.Unlock()
	_ = ctx
	return ok, nil
}

func incomingFrom(c tele.Context) Incoming {
	msg := c.Message()
	if msg == nil {
		return Incoming{}
	}
	return incomingFromMessage(msg)
}

func incomingFromMessage(msg *tele.Message) Incoming {
	in := Incoming{
		Text:      firstNonEmptyText(msg.Text, msg.Caption),
		Time:      msg.Time(),
		MessageID: msg.ID,
		ThreadID:  msg.ThreadID,
	}
	if msg.Chat != nil {
		in.ChatID = msg.Chat.ID
		in.ChatTitle = msg.Chat.Title
		in.ChatType = chatType(msg.Chat)
	}
	if u := msg.Sender; u != nil {
		in.User = store.User{
			ID:        u.ID,
			Username:  u.Username,
			FirstName: u.FirstName,
			LastName:  u.LastName,
		}
		in.UserIsBot = u.IsBot
	}
	if msg.Via != nil {
		in.ViaBot = true
	}
	if msg.ReplyTo != nil {
		r := incomingFromMessage(msg.ReplyTo)
		in.Reply = &r
	}
	return in
}

func chatType(c *tele.Chat) string {
	if c == nil {
		return ""
	}
	switch c.Type {
	case tele.ChatPrivate:
		return "private"
	case tele.ChatGroup:
		return "group"
	case tele.ChatSuperGroup:
		return "supergroup"
	case tele.ChatChannel:
		return "channel"
	default:
		return string(c.Type)
	}
}

func Register(b *tele.Bot, app *App) {
	h := func(c tele.Context) error {
		err := app.Handle(context.Background(), incomingFrom(c))
		if err != nil {
			app.log.Error("handler", "err", err)
		}
		return nil
	}
	b.Handle("/start", h)
	b.Handle("/help", h)
	b.Handle("/aura", h)
	b.Handle("/top", h)
	b.Handle("/radar", h)
	b.Handle(tele.OnText, h)
	b.Handle(tele.OnPhoto, h)
	b.Handle(tele.OnVideo, h)
	b.Handle(tele.OnAnimation, h)
	b.Handle(tele.OnDocument, h)
	b.Handle(tele.OnVoice, h)
}
