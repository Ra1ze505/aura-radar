package main

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Ra1ze505/aura-radar/src/bot"
	"github.com/Ra1ze505/aura-radar/src/config"
	"github.com/Ra1ze505/aura-radar/src/judge"
	"github.com/Ra1ze505/aura-radar/src/store"
	"github.com/Ra1ze505/aura-radar/src/transcribe"
	"github.com/Ra1ze505/aura-radar/src/vision"

	tele "gopkg.in/telebot.v4"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	log := newLogger(cfg.LogLevel)

	if strings.TrimSpace(cfg.BotToken) == "" {
		log.Error("BOT_TOKEN is empty; add Runtime Secret BOT_TOKEN or AURA_RADAR_BOT_TOKEN (docs/tz.md §13.4)")
		os.Exit(1)
	}

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	pref := tele.Settings{
		Token:  cfg.BotToken,
		Client: telegramHTTPClient(),
		Poller: &tele.LongPoller{
			Timeout:        10 * time.Second,
			AllowedUpdates: []string{"message"},
		},
		OnError: func(err error, _ tele.Context) {
			if err != nil {
				log.Error("telegram", "err", err)
			}
		},
	}
	b, err := tele.NewBot(pref)
	if err != nil {
		log.Error("telegram", "err", err)
		os.Exit(1)
	}

	var j judge.Judge
	if cfg.Blind() {
		log.Warn("LLM key or model missing; commands that need a judge will answer that the radar is blind (docs/tz.md §13.4)")
	} else {
		j = judge.NewOpenAI(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.OpenAIModel, cfg.LLMTimeout)
	}

	poster := bot.NewTelePoster(b)
	app := bot.New(cfg, st, j, poster, log, b.Me.Username)
	if cfg.OpenAIAPIKey != "" {
		stt := transcribe.New(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.TranscribeModel, cfg.STTTimeout)
		app.SetSpeech(poster, stt)
		if cfg.VisionModel != "" {
			vis := vision.New(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.VisionModel, cfg.VisionTimeout)
			app.SetVision(vis)
		}
	}
	bot.Register(b, app)

	cmds := []tele.Command{
		{Text: "start", Description: "что за радар"},
		{Text: "help", Description: "команды"},
		{Text: "aura", Description: "оценка реплаем или свой счёт"},
		{Text: "top", Description: "топ ауры чата"},
		{Text: "radar", Description: "статус авторадара"},
	}
	if err := b.SetCommands(cmds); err != nil {
		log.Warn("set commands", "err", err)
	}

	log.Info("polling",
		"bot", b.Me.Username,
		"model", cfg.OpenAIModel,
		"allowed_chat_id", cfg.AllowedChatID,
		"allowed_chat_title", cfg.AllowedChatTitle,
	)

	go b.Start()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	b.Stop()
}

func telegramHTTPClient() *http.Client {
	// HTTP/2 to api.telegram.org can stall getUpdates (Send-Q stuck,
	// default Client.Timeout never firing). Stay on HTTP/1.1 and cap the
	// whole request so a hung poll recovers.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          20,
	}
	return &http.Client{
		Timeout:   45 * time.Second,
		Transport: transport,
	}
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		lv = slog.LevelDebug
	case "WARN", "WARNING":
		lv = slog.LevelWarn
	case "ERROR":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
