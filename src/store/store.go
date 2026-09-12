package store

import (
	"context"
	"time"
)

type Chat struct {
	ID           int64
	Title        string
	RadarEnabled bool
}

type User struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
}

type BufferMessage struct {
	ChatID    int64
	MessageID int
	UserID    *int64
	Text      string
	SentAt    time.Time
	Display   string
}

type Event struct {
	ID                int64
	ChatID            int64
	UserID            int64
	MessageID         int
	Verdict           string
	Delta             int
	Confidence        float64
	Comment           string
	Reaction          string
	Trigger           string
	PublishedReaction bool
	PublishedText     bool
	CreatedAt         time.Time
}

type Score struct {
	ChatID      int64
	UserID      int64
	Points      int
	StrongCount int
	WeakCount   int
	Username    string
	FirstName   string
	LastName    string
}

type CooldownState struct {
	LastAutoReaction *time.Time
	LastUserReaction *time.Time
	LastAutoText     *time.Time
	LastUserText     *time.Time
	ReactToday       int
	TextToday        int
}

type Store interface {
	Close() error
	UpsertChat(ctx context.Context, chat Chat) error
	SetRadarEnabled(ctx context.Context, chatID int64, enabled bool) error
	Chat(ctx context.Context, chatID int64) (Chat, error)
	UpsertUser(ctx context.Context, user User) error
	AppendMessage(ctx context.Context, msg BufferMessage, keep int, maxAge time.Duration) error
	ContextBefore(ctx context.Context, chatID int64, beforeMessageID, limit int) ([]BufferMessage, error)
	GetEvent(ctx context.Context, chatID int64, messageID int) (Event, bool, error)
	InsertEventAndScore(ctx context.Context, ev Event) error
	MarkPublishedText(ctx context.Context, chatID int64, messageID int) error
	GetScore(ctx context.Context, chatID, userID int64) (Score, bool, error)
	GetScoreByUsername(ctx context.Context, chatID int64, username string) (Score, bool, error)
	Top(ctx context.Context, chatID int64, limit int) ([]Score, error)
	Rank(ctx context.Context, chatID, userID int64) (place, total int, err error)
	CooldownState(ctx context.Context, chatID, userID int64, dayStart time.Time) (CooldownState, error)
}
