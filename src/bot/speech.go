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
	MediaPhoto  = "photo"
)

var (
	errNoSpeech = errors.New("no speech")
	errNoImage  = errors.New("no image")
)

type MediaOpener interface {
	OpenMedia(ctx context.Context, fileID string) (io.ReadCloser, error)
}

type Transcriber interface {
	Transcribe(ctx context.Context, r io.Reader, filename, mime string) (string, error)
}

type Describer interface {
	Describe(ctx context.Context, r io.Reader, filename, mime string) (string, error)
}

func (a *App) SetSpeech(opener MediaOpener, stt Transcriber) {
	a.opener = opener
	a.stt = stt
}

func (a *App) SetVision(vis Describer) {
	a.vision = vis
}

func isSpeechKind(kind string) bool {
	return kind == MediaVoice || kind == MediaCircle || kind == MediaVideo
}

func isImageKind(kind string) bool {
	return kind == MediaPhoto
}

func isImageMIME(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	if !strings.HasPrefix(m, "image/") {
		return false
	}
	return m != "image/svg+xml"
}

func speechPrefix(kind string) string {
	switch kind {
	case MediaVoice:
		return "[голос]"
	case MediaCircle:
		return "[кружок]"
	case MediaVideo:
		return "[видео]"
	case MediaPhoto:
		return "[фото]"
	default:
		return "[голос]"
	}
}

func needsSpeech(in Incoming, minLen int, command bool) bool {
	if !isSpeechKind(in.MediaKind) || strings.TrimSpace(in.FileID) == "" {
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

func needsVision(in Incoming) bool {
	return isImageKind(in.MediaKind) && strings.TrimSpace(in.FileID) != ""
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
	case MediaPhoto:
		return "photo.jpg"
	default:
		return "video.mp4"
	}
}

func defaultMediaMIME(kind, mime string) string {
	if strings.TrimSpace(mime) != "" {
		return mime
	}
	switch kind {
	case MediaVoice:
		return "audio/ogg"
	case MediaPhoto:
		return "image/jpeg"
	default:
		return "video/mp4"
	}
}

func sniffImageMIME(data []byte, fallback string) string {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(data) >= 12 && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}
	if fallback != "" {
		return fallback
	}
	return "image/jpeg"
}

func filterText(in Incoming) string {
	if strings.TrimSpace(in.SpeechRaw) != "" {
		return in.SpeechRaw
	}
	return in.Text
}

func mediaFailMessage(err error, in Incoming) string {
	if errors.Is(err, errNoImage) || isImageKind(in.MediaKind) {
		return MsgNoImage
	}
	if errors.Is(err, errNoSpeech) || isSpeechKind(in.MediaKind) {
		return MsgNoVoice
	}
	return MsgNoText
}

func (a *App) fillMedia(ctx context.Context, in *Incoming, command bool) error {
	if err := a.fillSpeech(ctx, in, command); err != nil {
		return err
	}
	return a.fillVision(ctx, in)
}

func (a *App) fillSpeech(ctx context.Context, in *Incoming, command bool) error {
	if in == nil || !needsSpeech(*in, a.cfg.MinTextLen, command) {
		return nil
	}
	if a.cfg.MediaMaxSec > 0 && in.Duration > a.cfg.MediaMaxSec {
		a.log.Info("stt skip", "reason", "too_long", "duration", in.Duration, "kind", in.MediaKind)
		return errNoSpeech
	}
	data, err := a.downloadMedia(ctx, in)
	if err != nil {
		return errNoSpeech
	}
	if a.stt == nil || strings.TrimSpace(a.cfg.OpenAIAPIKey) == "" {
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

func (a *App) fillVision(ctx context.Context, in *Incoming) error {
	if in == nil || !needsVision(*in) {
		return nil
	}
	caption := strings.TrimSpace(in.Text)
	if a.vision == nil || strings.TrimSpace(a.cfg.OpenAIAPIKey) == "" {
		if caption != "" {
			return nil
		}
		return errNoImage
	}
	data, err := a.downloadMedia(ctx, in)
	if err != nil {
		if caption != "" {
			return nil
		}
		return errNoImage
	}
	timeout := a.cfg.VisionTimeout
	if timeout <= 0 {
		timeout = 40 * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	mime := sniffImageMIME(data, defaultMediaMIME(MediaPhoto, in.MIME))
	raw, err := a.vision.Describe(
		tctx,
		bytes.NewReader(data),
		defaultMediaName(in.MediaKind, in.FileName),
		mime,
	)
	if err != nil {
		a.log.Warn("vision failed", "err", err, "kind", in.MediaKind)
		if caption != "" {
			return nil
		}
		return errNoImage
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if caption != "" {
			return nil
		}
		return errNoImage
	}
	in.SpeechRaw = raw
	in.Text = speechPrefix(MediaPhoto) + " " + raw
	if caption != "" {
		in.SpeechRaw = strings.TrimSpace(raw + " " + caption)
		in.Text += "\n" + caption
	}
	a.log.Info("vision ok", "kind", in.MediaKind, "raw_len", runeCount(raw), "message_id", in.MessageID)
	return nil
}

func (a *App) downloadMedia(ctx context.Context, in *Incoming) ([]byte, error) {
	if a.cfg.MediaMaxBytes > 0 && in.FileSize > int64(a.cfg.MediaMaxBytes) {
		a.log.Info("media skip", "reason", "too_big", "bytes", in.FileSize, "kind", in.MediaKind)
		return nil, errNoSpeech
	}
	if a.opener == nil {
		return nil, errNoSpeech
	}
	rc, err := a.opener.OpenMedia(ctx, in.FileID)
	if err != nil {
		a.log.Warn("media open failed", "err", err, "kind", in.MediaKind)
		return nil, err
	}
	defer rc.Close()
	limit := int64(a.cfg.MediaMaxBytes)
	if limit <= 0 {
		limit = 20_000_000
	}
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		a.log.Warn("media download failed", "err", err, "kind", in.MediaKind)
		return nil, err
	}
	if int64(len(data)) > limit {
		a.log.Info("media skip", "reason", "too_big", "bytes", len(data), "kind", in.MediaKind)
		return nil, errNoSpeech
	}
	if len(data) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return data, nil
}
