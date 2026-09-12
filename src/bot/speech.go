package bot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	MediaVoice  = "voice"
	MediaCircle = "video_note"
	MediaVideo  = "video"
)

var errNoSpeech = errors.New("no speech")

type MediaOpener interface {
	OpenMedia(ctx context.Context, fileID string) (io.ReadCloser, error)
}

type Transcriber interface {
	Transcribe(ctx context.Context, r io.Reader, filename, mime string) (string, error)
}

func (a *App) SetSpeech(opener MediaOpener, stt Transcriber) {
	a.opener = opener
	a.stt = stt
}

func speechPrefix(kind string) string {
	switch kind {
	case MediaVoice:
		return "[голос]"
	case MediaCircle:
		return "[кружок]"
	case MediaVideo:
		return "[видео]"
	default:
		return "[голос]"
	}
}

func needsSpeech(in Incoming, minLen int, command bool) bool {
	if in.MediaKind == "" || strings.TrimSpace(in.FileID) == "" {
		return false
	}
	n := runeCount(strings.TrimSpace(in.Text))
	if command {
		return n == 0
	}
	if minLen < 1 {
		minLen = 1
	}
	return n < minLen
}

func defaultMediaName(kind, name string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	switch kind {
	case MediaVoice:
		return "voice.ogg"
	case MediaCircle:
		return "circle.mp4"
	default:
		return "video.mp4"
	}
}

func defaultMediaMIME(kind, mime string) string {
	if strings.TrimSpace(mime) != "" {
		return mime
	}
	if kind == MediaVoice {
		return "audio/ogg"
	}
	return "video/mp4"
}

func filterText(in Incoming) string {
	if strings.TrimSpace(in.SpeechRaw) != "" {
		return in.SpeechRaw
	}
	return in.Text
}

func (a *App) fillSpeech(ctx context.Context, in *Incoming, command bool) error {
	if in == nil || !needsSpeech(*in, a.cfg.MinTextLen, command) {
		return nil
	}
	if a.cfg.MediaMaxSec > 0 && in.Duration > a.cfg.MediaMaxSec {
		a.log.Info("stt skip", "reason", "too_long", "duration", in.Duration, "kind", in.MediaKind)
		return errNoSpeech
	}
	if a.cfg.MediaMaxBytes > 0 && in.FileSize > int64(a.cfg.MediaMaxBytes) {
		a.log.Info("stt skip", "reason", "too_big", "bytes", in.FileSize, "kind", in.MediaKind)
		return errNoSpeech
	}
	if a.opener == nil || a.stt == nil || strings.TrimSpace(a.cfg.OpenAIAPIKey) == "" {
		return errNoSpeech
	}

	rc, err := a.opener.OpenMedia(ctx, in.FileID)
	if err != nil {
		a.log.Warn("stt open failed", "err", err, "kind", in.MediaKind)
		return errNoSpeech
	}
	defer rc.Close()

	limit := int64(a.cfg.MediaMaxBytes)
	if limit <= 0 {
		limit = 20_000_000
	}
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		a.log.Warn("stt download failed", "err", err, "kind", in.MediaKind)
		return errNoSpeech
	}
	if int64(len(data)) > limit {
		a.log.Info("stt skip", "reason", "too_big", "bytes", len(data), "kind", in.MediaKind)
		return errNoSpeech
	}

	timeout := a.cfg.STTTimeout
	if timeout <= 0 {
		timeout = 40 * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := a.stt.Transcribe(
		tctx,
		bytes.NewReader(data),
		defaultMediaName(in.MediaKind, in.FileName),
		defaultMediaMIME(in.MediaKind, in.MIME),
	)
	if err != nil {
		a.log.Warn("stt failed", "err", err, "kind", in.MediaKind)
		return errNoSpeech
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errNoSpeech
	}
	in.SpeechRaw = raw
	in.Text = speechPrefix(in.MediaKind) + " " + raw
	a.log.Info("stt ok", "kind", in.MediaKind, "raw_len", runeCount(raw), "message_id", in.MessageID)
	return nil
}
