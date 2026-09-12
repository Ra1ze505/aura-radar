package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	appdb "github.com/Ra1ze505/aura-radar/db"
	"github.com/Ra1ze505/aura-radar/src/aura"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type SQLStore struct {
	db     *sql.DB
	driver string
}

func Open(databaseURL string) (*SQLStore, error) {
	driver, dsn, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		if err := os.MkdirAll(filepath.Dir(dsnPath(dsn)), 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Dir may be "."
			_ = err
		}
		if dir := filepath.Dir(dsnPath(dsn)); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}
	db, err := sql.Open(driverName(driver), dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;`); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	s := &SQLStore{db: db, driver: driver}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLStore) Close() error { return s.db.Close() }

func (s *SQLStore) migrate() error {
	entries, err := fs.Glob(appdb.Migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		body, err := fs.ReadFile(appdb.Migrations, name)
		if err != nil {
			return err
		}
		sqlText := string(body)
		if s.driver == "postgres" {
			sqlText = strings.ReplaceAll(sqlText, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
		}
		if _, err := s.db.Exec(sqlText); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *SQLStore) q(query string) string {
	if s.driver != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (s *SQLStore) UpsertChat(ctx context.Context, chat Chat) error {
	now := nowRFC3339()
	_, err := s.db.ExecContext(ctx, s.q(`
INSERT INTO chats (chat_id, title, radar_enabled, created_at, updated_at)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT (chat_id) DO UPDATE SET title = excluded.title, updated_at = excluded.updated_at
`), chat.ID, nullString(chat.Title), now, now)
	return err
}

func (s *SQLStore) SetRadarEnabled(ctx context.Context, chatID int64, enabled bool) error {
	now := nowRFC3339()
	_, err := s.db.ExecContext(ctx, s.q(`
INSERT INTO chats (chat_id, radar_enabled, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (chat_id) DO UPDATE SET radar_enabled = excluded.radar_enabled, updated_at = excluded.updated_at
`), chatID, boolInt(enabled), now, now)
	return err
}

func (s *SQLStore) Chat(ctx context.Context, chatID int64) (Chat, error) {
	var c Chat
	var title sql.NullString
	var enabled int
	err := s.db.QueryRowContext(ctx, s.q(`SELECT chat_id, title, radar_enabled FROM chats WHERE chat_id = ?`), chatID).
		Scan(&c.ID, &title, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return Chat{ID: chatID, RadarEnabled: true}, nil
	}
	if err != nil {
		return Chat{}, err
	}
	c.Title = title.String
	c.RadarEnabled = enabled != 0
	return c, nil
}

func (s *SQLStore) UpsertUser(ctx context.Context, user User) error {
	now := nowRFC3339()
	_, err := s.db.ExecContext(ctx, s.q(`
INSERT INTO users (user_id, username, first_name, last_name, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
  username = excluded.username,
  first_name = excluded.first_name,
  last_name = excluded.last_name,
  updated_at = excluded.updated_at
`), user.ID, nullString(user.Username), nullString(user.FirstName), nullString(user.LastName), now)
	return err
}

func (s *SQLStore) AppendMessage(ctx context.Context, msg BufferMessage, keep int, maxAge time.Duration) error {
	if keep < 50 {
		keep = 50
	}
	text := aura.ClipMessage(msg.Text)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var user any
	if msg.UserID != nil {
		user = *msg.UserID
	}
	_, err := s.db.ExecContext(ctx, s.q(`
INSERT INTO message_buffer (chat_id, message_id, user_id, text, sent_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (chat_id, message_id) DO UPDATE SET text = excluded.text
`), msg.ChatID, msg.MessageID, user, text, msg.SentAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, s.q(`
DELETE FROM message_buffer
WHERE chat_id = ?
  AND (
    sent_at < ?
    OR id NOT IN (
      SELECT id FROM message_buffer WHERE chat_id = ? ORDER BY id DESC LIMIT ?
    )
  )
`), msg.ChatID, cutoff, msg.ChatID, keep)
	return err
}

func (s *SQLStore) ContextBefore(ctx context.Context, chatID int64, beforeMessageID, limit int) ([]BufferMessage, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, s.q(`
SELECT b.chat_id, b.message_id, b.user_id, b.text, b.sent_at,
       COALESCE(u.username, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, '')
FROM message_buffer b
LEFT JOIN users u ON u.user_id = b.user_id
WHERE b.chat_id = ? AND b.message_id < ?
ORDER BY b.message_id DESC
LIMIT ?
`), chatID, beforeMessageID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var newestFirst []BufferMessage
	for rows.Next() {
		var m BufferMessage
		var uid sql.NullInt64
		var sent string
		var username, first, last string
		if err := rows.Scan(&m.ChatID, &m.MessageID, &uid, &m.Text, &sent, &username, &first, &last); err != nil {
			return nil, err
		}
		if uid.Valid {
			id := uid.Int64
			m.UserID = &id
		}
		m.SentAt, _ = time.Parse(time.RFC3339Nano, sent)
		m.Display = DisplayName(User{Username: username, FirstName: first, LastName: last, ID: uid.Int64})
		newestFirst = append(newestFirst, m)
	}
	// oldest → newest
	for i, j := 0, len(newestFirst)-1; i < j; i, j = i+1, j-1 {
		newestFirst[i], newestFirst[j] = newestFirst[j], newestFirst[i]
	}
	return newestFirst, rows.Err()
}

func (s *SQLStore) GetEvent(ctx context.Context, chatID int64, messageID int) (Event, bool, error) {
	var e Event
	var reaction sql.NullString
	var pubR, pubT int
	var created string
	err := s.db.QueryRowContext(ctx, s.q(`
SELECT id, chat_id, user_id, message_id, verdict, delta, confidence, comment, reaction,
       trigger, published_reaction, published_text, created_at
FROM aura_events WHERE chat_id = ? AND message_id = ?
`), chatID, messageID).Scan(
		&e.ID, &e.ChatID, &e.UserID, &e.MessageID, &e.Verdict, &e.Delta, &e.Confidence, &e.Comment, &reaction,
		&e.Trigger, &pubR, &pubT, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	e.Reaction = reaction.String
	e.PublishedReaction = pubR != 0
	e.PublishedText = pubT != 0
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return e, true, nil
}

func (s *SQLStore) InsertEventAndScore(ctx context.Context, ev Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := nowRFC3339()
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	_, err = tx.ExecContext(ctx, s.q(`
INSERT INTO aura_events (
  chat_id, user_id, message_id, verdict, delta, confidence, comment, reaction,
  trigger, published_reaction, published_text, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`), ev.ChatID, ev.UserID, ev.MessageID, ev.Verdict, ev.Delta, ev.Confidence, ev.Comment, nullString(ev.Reaction),
		ev.Trigger, boolInt(ev.PublishedReaction), boolInt(ev.PublishedText), ev.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	strong, weak := 0, 0
	if ev.Verdict == aura.VerdictStrong {
		strong = 1
	}
	if ev.Verdict == aura.VerdictWeak {
		weak = 1
	}
	_, err = tx.ExecContext(ctx, s.q(`
INSERT INTO chat_scores (chat_id, user_id, points, strong_count, weak_count, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (chat_id, user_id) DO UPDATE SET
  points = chat_scores.points + excluded.points,
  strong_count = chat_scores.strong_count + excluded.strong_count,
  weak_count = chat_scores.weak_count + excluded.weak_count,
  updated_at = excluded.updated_at
`), ev.ChatID, ev.UserID, ev.Delta, strong, weak, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLStore) MarkPublishedText(ctx context.Context, chatID int64, messageID int) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE aura_events SET published_text = 1 WHERE chat_id = ? AND message_id = ?`), chatID, messageID)
	return err
}

func (s *SQLStore) GetScore(ctx context.Context, chatID, userID int64) (Score, bool, error) {
	var sc Score
	var username, first, last sql.NullString
	err := s.db.QueryRowContext(ctx, s.q(`
SELECT cs.chat_id, cs.user_id, cs.points, cs.strong_count, cs.weak_count,
       COALESCE(u.username, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, '')
FROM chat_scores cs
LEFT JOIN users u ON u.user_id = cs.user_id
WHERE cs.chat_id = ? AND cs.user_id = ?
`), chatID, userID).Scan(&sc.ChatID, &sc.UserID, &sc.Points, &sc.StrongCount, &sc.WeakCount, &username, &first, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return Score{ChatID: chatID, UserID: userID}, false, nil
	}
	if err != nil {
		return Score{}, false, err
	}
	sc.Username = username.String
	sc.FirstName = first.String
	sc.LastName = last.String
	return sc, true, nil
}

func (s *SQLStore) GetScoreByUsername(ctx context.Context, chatID int64, username string) (Score, bool, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	var sc Score
	err := s.db.QueryRowContext(ctx, s.q(`
SELECT cs.chat_id, cs.user_id, cs.points, cs.strong_count, cs.weak_count,
       COALESCE(u.username, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, '')
FROM users u
JOIN chat_scores cs ON cs.user_id = u.user_id AND cs.chat_id = ?
WHERE LOWER(u.username) = LOWER(?)
`), chatID, username).Scan(&sc.ChatID, &sc.UserID, &sc.Points, &sc.StrongCount, &sc.WeakCount, &sc.Username, &sc.FirstName, &sc.LastName)
	if errors.Is(err, sql.ErrNoRows) {
		return Score{}, false, nil
	}
	if err != nil {
		return Score{}, false, err
	}
	return sc, true, nil
}

func (s *SQLStore) Top(ctx context.Context, chatID int64, limit int) ([]Score, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
SELECT cs.chat_id, cs.user_id, cs.points, cs.strong_count, cs.weak_count,
       COALESCE(u.username, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, '')
FROM chat_scores cs
LEFT JOIN users u ON u.user_id = cs.user_id
WHERE cs.chat_id = ? AND (cs.strong_count + cs.weak_count) > 0
ORDER BY cs.points DESC, cs.updated_at ASC
LIMIT ?
`), chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Score
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.ChatID, &sc.UserID, &sc.Points, &sc.StrongCount, &sc.WeakCount, &sc.Username, &sc.FirstName, &sc.LastName); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *SQLStore) Rank(ctx context.Context, chatID, userID int64) (place, total int, err error) {
	err = s.db.QueryRowContext(ctx, s.q(`
SELECT COUNT(*) FROM chat_scores WHERE chat_id = ? AND (strong_count + weak_count) > 0
`), chatID).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	var has int
	err = s.db.QueryRowContext(ctx, s.q(`
SELECT COUNT(*) FROM chat_scores WHERE chat_id = ? AND user_id = ? AND (strong_count + weak_count) > 0
`), chatID, userID).Scan(&has)
	if err != nil {
		return 0, 0, err
	}
	if has == 0 {
		return 0, total, nil
	}
	err = s.db.QueryRowContext(ctx, s.q(`
SELECT 1 + COUNT(*) FROM chat_scores a
JOIN chat_scores b ON b.chat_id = a.chat_id AND a.user_id = ?
WHERE a.chat_id = ?
  AND (b.strong_count + b.weak_count) > 0
  AND (
    b.points > a.points OR
    (b.points = a.points AND b.updated_at < a.updated_at) OR
    (b.points = a.points AND b.updated_at = a.updated_at AND b.user_id < a.user_id)
  )
`), userID, chatID).Scan(&place)
	return place, total, err
}

func (s *SQLStore) CooldownState(ctx context.Context, chatID, userID int64, dayStart time.Time) (CooldownState, error) {
	var st CooldownState
	st.LastAutoReaction = queryTime(ctx, s, `
SELECT created_at FROM aura_events
WHERE chat_id = ? AND trigger = 'auto' AND published_reaction = 1
ORDER BY created_at DESC LIMIT 1`, chatID)
	st.LastUserReaction = queryTime(ctx, s, `
SELECT created_at FROM aura_events
WHERE chat_id = ? AND user_id = ? AND trigger = 'auto' AND published_reaction = 1
ORDER BY created_at DESC LIMIT 1`, chatID, userID)
	st.LastAutoText = queryTime(ctx, s, `
SELECT created_at FROM aura_events
WHERE chat_id = ? AND trigger = 'auto' AND published_text = 1
ORDER BY created_at DESC LIMIT 1`, chatID)
	st.LastUserText = queryTime(ctx, s, `
SELECT created_at FROM aura_events
WHERE chat_id = ? AND user_id = ? AND trigger = 'auto' AND published_text = 1
ORDER BY created_at DESC LIMIT 1`, chatID, userID)
	day := dayStart.UTC().Format(time.RFC3339Nano)
	_ = s.db.QueryRowContext(ctx, s.q(`
SELECT COUNT(*) FROM aura_events
WHERE chat_id = ? AND trigger = 'auto' AND published_reaction = 1 AND created_at >= ?
`), chatID, day).Scan(&st.ReactToday)
	_ = s.db.QueryRowContext(ctx, s.q(`
SELECT COUNT(*) FROM aura_events
WHERE chat_id = ? AND trigger = 'auto' AND published_text = 1 AND created_at >= ?
`), chatID, day).Scan(&st.TextToday)
	return st, nil
}

func queryTime(ctx context.Context, s *SQLStore, q string, args ...any) *time.Time {
	var raw string
	err := s.db.QueryRowContext(ctx, s.q(q), args...).Scan(&raw)
	if err != nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil
		}
	}
	return &t
}

func DisplayName(u User) string {
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if name != "" {
		return name
	}
	if u.Username != "" {
		return "@" + strings.TrimPrefix(u.Username, "@")
	}
	if u.ID != 0 {
		return fmt.Sprintf("id:%d", u.ID)
	}
	return "участник"
}

func parseDatabaseURL(raw string) (driver, dsn string, err error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return "sqlite", DefaultSQLiteDSN("data/aura_radar.db"), nil
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		return "postgres", raw, nil
	case strings.HasPrefix(raw, "sqlite://"):
		return "sqlite", DefaultSQLiteDSN(strings.TrimPrefix(raw, "sqlite://")), nil
	case strings.HasPrefix(raw, "sqlite:"):
		return "sqlite", DefaultSQLiteDSN(strings.TrimPrefix(raw, "sqlite:")), nil
	default:
		return "sqlite", DefaultSQLiteDSN(raw), nil
	}
}

func DefaultSQLiteDSN(path string) string {
	path = strings.TrimPrefix(path, "//")
	if path == "" {
		path = "data/aura_radar.db"
	}
	return "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

func driverName(driver string) string {
	if driver == "postgres" {
		return "postgres"
	}
	return "sqlite"
}

func dsnPath(dsn string) string {
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return p
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
