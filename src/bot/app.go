package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Ra1ze505/aura-radar/src/aura"
	"github.com/Ra1ze505/aura-radar/src/config"
	"github.com/Ra1ze505/aura-radar/src/judge"
	"github.com/Ra1ze505/aura-radar/src/store"
)

type Poster interface {
	Reply(ctx context.Context, chatID int64, replyTo int, text string) error
	React(ctx context.Context, chatID int64, messageID int, emoji string, big bool) error
	IsAdmin(ctx context.Context, chatID, userID int64) (bool, error)
}

type Incoming struct {
	ChatID    int64
	ChatType  string
	ChatTitle string
	MessageID int
	User      store.User
	UserIsBot bool
	ViaBot    bool
	Text      string
	Time      time.Time
	Reply     *Incoming
}

type App struct {
	cfg      config.Config
	store    store.Store
	judge    judge.Judge
	poster   Poster
	inflight *aura.InFlight
	log      *slog.Logger
	now      func() time.Time
	botUser  string
}

func New(cfg config.Config, st store.Store, j judge.Judge, poster Poster, log *slog.Logger, botUser string) *App {
	if log == nil {
		log = slog.Default()
	}
	return &App{
		cfg:      cfg,
		store:    st,
		judge:    j,
		poster:   poster,
		inflight: aura.NewInFlight(),
		log:      log,
		now:      time.Now,
		botUser:  botUser,
	}
}

func (a *App) Handle(ctx context.Context, in Incoming) error {
	if in.ChatType == "channel" {
		return nil
	}
	if !a.cfg.AllowsChat(in.ChatID, in.ChatTitle, in.ChatType) {
		return nil
	}
	if in.Time.IsZero() {
		in.Time = a.now()
	}
	if in.ChatType == "group" || in.ChatType == "supergroup" {
		_ = a.store.UpsertChat(ctx, store.Chat{ID: in.ChatID, Title: in.ChatTitle, RadarEnabled: true})
	}
	if in.User.ID != 0 && !in.UserIsBot {
		_ = a.store.UpsertUser(ctx, in.User)
	}
	if in.Reply != nil && in.Reply.User.ID != 0 && !in.Reply.UserIsBot {
		_ = a.store.UpsertUser(ctx, in.Reply.User)
	}
	cmd, payload, isCmd := parseCommand(in.Text, a.botUser)
	if !isCmd && strings.TrimSpace(in.Text) != "" && !looksLikeCommand(in.Text) {
		var uid *int64
		if in.User.ID != 0 {
			id := in.User.ID
			uid = &id
		}
		keep := a.cfg.ContextMessages
		_ = a.store.AppendMessage(ctx, store.BufferMessage{
			ChatID:    in.ChatID,
			MessageID: in.MessageID,
			UserID:    uid,
			Text:      in.Text,
			SentAt:    in.Time,
		}, keep, 7*24*time.Hour)
	}

	if isCmd {
		return a.handleCommand(ctx, in, cmd, payload)
	}
	if in.ChatType == "group" || in.ChatType == "supergroup" {
		return a.maybeRadar(ctx, in)
	}
	return nil
}

func (a *App) handleCommand(ctx context.Context, in Incoming, cmd, payload string) error {
	switch cmd {
	case "start":
		return a.cmdStart(ctx, in)
	case "help":
		return a.reply(ctx, in, MsgHelp)
	case "aura":
		return a.cmdAura(ctx, in, payload)
	case "top":
		return a.cmdTop(ctx, in)
	case "radar":
		return a.cmdRadar(ctx, in, payload)
	default:
		return nil
	}
}

func (a *App) cmdStart(ctx context.Context, in Incoming) error {
	if in.ChatType == "private" {
		return a.reply(ctx, in, MsgStartPrivate)
	}
	chat, err := a.store.Chat(ctx, in.ChatID)
	if err != nil {
		return err
	}
	if chat.RadarEnabled {
		return a.reply(ctx, in, MsgStartGroupOn)
	}
	return a.reply(ctx, in, MsgStartGroupOff)
}

func (a *App) cmdAura(ctx context.Context, in Incoming, payload string) error {
	if in.ChatType == "private" {
		return a.reply(ctx, in, MsgGroupOnly)
	}
	if uname := parseUsername(payload); uname != "" {
		return a.replyScoreByUsername(ctx, in, uname)
	}
	if in.Reply == nil {
		return a.replyOwnScore(ctx, in)
	}
	return a.cmdAuraReply(ctx, in, *in.Reply)
}

func (a *App) cmdAuraReply(ctx context.Context, in Incoming, target Incoming) error {
	if target.UserIsBot {
		return a.reply(ctx, in, MsgSkipBots)
	}
	text := strings.TrimSpace(target.Text)
	ev, ok, err := a.store.GetEvent(ctx, in.ChatID, target.MessageID)
	if err != nil {
		return err
	}
	if ok {
		_ = a.poster.React(ctx, in.ChatID, target.MessageID, ev.Reaction, aura.BigReaction(ev.Delta))
		return a.reply(ctx, in, FormatCard(ev.Verdict, ev.Delta, ev.Reaction, ev.Comment))
	}
	if text == "" {
		return a.reply(ctx, in, MsgNoText)
	}
	if a.cfg.Blind() || a.judge == nil {
		return a.reply(ctx, in, MsgBlind)
	}
	res, err := a.evaluate(ctx, in.ChatID, in.ChatTitle, target)
	if err != nil {
		a.log.Warn("judge failed", "err", err)
		return a.reply(ctx, in, MsgBlind)
	}
	card := FormatCard(res.Verdict, res.Delta, res.ReactionEmoji(), res.Comment)
	if err := a.reply(ctx, in, card); err != nil {
		return err
	}
	if res.Verdict == aura.VerdictNeutral {
		return nil
	}
	emoji := res.ReactionEmoji()
	if emoji != "" {
		if err := a.poster.React(ctx, in.ChatID, target.MessageID, emoji, aura.BigReaction(res.Delta)); err != nil {
			a.log.Warn("react failed", "err", err)
		}
	}
	return a.store.InsertEventAndScore(ctx, store.Event{
		ChatID:            in.ChatID,
		UserID:            target.User.ID,
		MessageID:         target.MessageID,
		Verdict:           res.Verdict,
		Delta:             res.Delta,
		Confidence:        res.Confidence,
		Comment:           res.Comment,
		Reaction:          emoji,
		Trigger:           aura.TriggerCommand,
		PublishedReaction: emoji != "",
		PublishedText:     true,
		CreatedAt:         a.now().UTC(),
	})
}

func (a *App) replyOwnScore(ctx context.Context, in Incoming) error {
	sc, found, err := a.store.GetScore(ctx, in.ChatID, in.User.ID)
	if err != nil {
		return err
	}
	place, total, err := a.store.Rank(ctx, in.ChatID, in.User.ID)
	if err != nil {
		return err
	}
	name := store.DisplayName(in.User)
	return a.reply(ctx, in, FormatScore(name, sc, place, total, found))
}

func (a *App) replyScoreByUsername(ctx context.Context, in Incoming, username string) error {
	sc, found, err := a.store.GetScoreByUsername(ctx, in.ChatID, username)
	if err != nil {
		return err
	}
	if !found {
		return a.reply(ctx, in, MsgUnknownUser)
	}
	place, total, err := a.store.Rank(ctx, in.ChatID, sc.UserID)
	if err != nil {
		return err
	}
	name := store.DisplayName(store.User{ID: sc.UserID, Username: sc.Username, FirstName: sc.FirstName, LastName: sc.LastName})
	return a.reply(ctx, in, FormatScore(name, sc, place, total, true))
}

func (a *App) cmdTop(ctx context.Context, in Incoming) error {
	if in.ChatType == "private" {
		return a.reply(ctx, in, MsgGroupOnly)
	}
	rows, err := a.store.Top(ctx, in.ChatID, 10)
	if err != nil {
		return err
	}
	return a.reply(ctx, in, FormatTop(rows))
}

func (a *App) cmdRadar(ctx context.Context, in Incoming, payload string) error {
	if in.ChatType == "private" {
		return a.reply(ctx, in, MsgGroupOnly)
	}
	arg := strings.ToLower(strings.TrimSpace(payload))
	switch arg {
	case "on", "off":
		ok, err := a.poster.IsAdmin(ctx, in.ChatID, in.User.ID)
		if err != nil {
			a.log.Warn("admin check failed", "err", err)
			return a.reply(ctx, in, MsgNotAdmin)
		}
		if !ok {
			return a.reply(ctx, in, MsgNotAdmin)
		}
		if err := a.store.SetRadarEnabled(ctx, in.ChatID, arg == "on"); err != nil {
			return err
		}
		if arg == "on" {
			return a.reply(ctx, in, MsgRadarOn)
		}
		return a.reply(ctx, in, MsgRadarOff)
	default:
		chat, err := a.store.Chat(ctx, in.ChatID)
		if err != nil {
			return err
		}
		if chat.RadarEnabled {
			return a.reply(ctx, in, MsgRadarOn)
		}
		return a.reply(ctx, in, MsgRadarOff)
	}
}

func (a *App) maybeRadar(ctx context.Context, in Incoming) error {
	chat, err := a.store.Chat(ctx, in.ChatID)
	if err != nil {
		return err
	}
	ev, hasEvent, err := a.store.GetEvent(ctx, in.ChatID, in.MessageID)
	if err != nil {
		return err
	}
	_ = ev
	ok, _ := aura.CheapFilters(aura.FilterInput{
		ChatType:     in.ChatType,
		FromBot:      in.UserIsBot,
		ViaBot:       in.ViaBot,
		Text:         in.Text,
		IsCommand:    looksLikeCommand(in.Text),
		HasEvent:     hasEvent,
		RadarEnabled: chat.RadarEnabled,
		InFlight:     a.inflight.Busy(in.ChatID),
		MinTextLen:   a.cfg.MinTextLen,
	})
	if !ok {
		return nil
	}
	if a.cfg.Blind() || a.judge == nil {
		return nil
	}
	if !a.inflight.TryLock(in.ChatID) {
		return nil
	}
	defer a.inflight.Unlock(in.ChatID)

	res, err := a.evaluate(ctx, in.ChatID, in.ChatTitle, in)
	if err != nil {
		a.log.Warn("auto judge failed", "err", err)
		return nil
	}
	cd, err := a.cooldowns(ctx, in)
	if err != nil {
		return err
	}
	decision := aura.DecideAuto(res.ToVerdict(), aura.Thresholds{
		ReactMinConfidence: a.cfg.ReactMinConfidence,
		ReactMinAbsDelta:   a.cfg.ReactMinAbsDelta,
		TextMinConfidence:  a.cfg.TextMinConfidence,
		TextMinAbsDelta:    a.cfg.TextMinAbsDelta,
	}, cd)
	if decision == aura.DecisionNone {
		return nil
	}
	emoji := res.ReactionEmoji()
	if emoji == "" {
		return nil
	}
	if err := a.poster.React(ctx, in.ChatID, in.MessageID, emoji, aura.BigReaction(res.Delta)); err != nil {
		a.log.Warn("auto react failed, skip score", "err", err)
		return nil
	}
	publishText := decision == aura.DecisionReactAndText
	if err := a.store.InsertEventAndScore(ctx, store.Event{
		ChatID:            in.ChatID,
		UserID:            in.User.ID,
		MessageID:         in.MessageID,
		Verdict:           res.Verdict,
		Delta:             res.Delta,
		Confidence:        res.Confidence,
		Comment:           res.Comment,
		Reaction:          emoji,
		Trigger:           aura.TriggerAuto,
		PublishedReaction: true,
		PublishedText:     false,
		CreatedAt:         a.now().UTC(),
	}); err != nil {
		return err
	}
	if !publishText {
		return nil
	}
	card := FormatCard(res.Verdict, res.Delta, emoji, res.Comment)
	if err := a.poster.Reply(ctx, in.ChatID, in.MessageID, card); err != nil {
		a.log.Warn("auto text failed", "err", err)
		return nil
	}
	return a.store.MarkPublishedText(ctx, in.ChatID, in.MessageID)
}

func (a *App) cooldowns(ctx context.Context, in Incoming) (aura.Cooldowns, error) {
	st, err := a.store.CooldownState(ctx, in.ChatID, in.User.ID, aura.DayStartUTC(a.now()))
	if err != nil {
		return aura.Cooldowns{}, err
	}
	now := a.now()
	reactOK := st.ReactToday < a.cfg.ReactChatDailyCap &&
		cooled(st.LastAutoReaction, now, a.cfg.ReactChatCooldown) &&
		cooled(st.LastUserReaction, now, a.cfg.ReactUserCooldown)
	textOK := st.TextToday < a.cfg.TextChatDailyCap &&
		cooled(st.LastAutoText, now, a.cfg.TextChatCooldown) &&
		cooled(st.LastUserText, now, a.cfg.TextUserCooldown)
	return aura.Cooldowns{ReactOK: reactOK, TextOK: textOK}, nil
}

func cooled(last *time.Time, now time.Time, d time.Duration) bool {
	if last == nil {
		return true
	}
	return now.Sub(*last) >= d
}

func (a *App) evaluate(ctx context.Context, chatID int64, title string, target Incoming) (judge.Result, error) {
	ctxMsgs, err := a.store.ContextBefore(ctx, chatID, target.MessageID, a.cfg.ContextMessages)
	if err != nil {
		return judge.Result{}, err
	}
	var uid *int64
	if target.User.ID != 0 {
		id := target.User.ID
		uid = &id
	}
	jctx, cancel := context.WithTimeout(ctx, a.cfg.LLMTimeout)
	defer cancel()
	return a.judge.Evaluate(jctx, judge.PromptInput{
		ChatTitle: title,
		Target: store.BufferMessage{
			ChatID:    chatID,
			MessageID: target.MessageID,
			UserID:    uid,
			Text:      target.Text,
			Display:   store.DisplayName(target.User),
		},
		Context: ctxMsgs,
	})
}

func (a *App) reply(ctx context.Context, in Incoming, text string) error {
	return a.poster.Reply(ctx, in.ChatID, in.MessageID, text)
}
