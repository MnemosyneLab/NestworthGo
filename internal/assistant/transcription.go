package assistant

import (
	"context"
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf8"
)

const TranscriptionModel = "gpt-transcribe"
const MaxAudioBytes = 8 << 20
const transcriptionSlot = "voice-input" // Not a conversation ID; shares runtime cancellation fences.

type AudioInput struct {
	Epoch      string `json:"epoch"`
	Revision   uint64 `json:"revision"`
	Audio      string `json:"audio"` // Base64, memory only. Never persisted or logged.
	Format     string `json:"format"`
	DurationMS int    `json:"durationMs"`
}
type Transcriber func(context.Context, []byte, []byte, string) (string, error)

// Transcribe has no tools, history or financial authority. It only returns a draft.
func (r *Runtime) Transcribe(caller context.Context, q AudioInput) (string, error) {
	if q.DurationMS < 1000 || q.DurationMS > 120000 || len(q.Audio) > base64.StdEncoding.EncodedLen(MaxAudioBytes) || (q.Format != "webm" && q.Format != "mp4" && q.Format != "wav") {
		return "", ErrBudget
	}
	audio, err := base64.StdEncoding.DecodeString(q.Audio)
	defer clear(audio)
	if err != nil || len(audio) == 0 || len(audio) > MaxAudioBytes {
		return "", ErrBudget
	}
	r.mu.Lock()
	if r.valid(caller, q.Epoch) != nil || q.Revision != r.state.Revision {
		r.mu.Unlock()
		return "", ErrStale
	}
	if !r.state.Config.Enabled || r.state.Config.Provider != "openai-responses" || r.options.Transcriber == nil || !r.options.Credentials.Available() {
		r.mu.Unlock()
		return "", ErrUnavailable
	}
	if r.active[transcriptionSlot] != nil {
		r.mu.Unlock()
		return "", ErrConflict
	}
	ctx, cancel := context.WithTimeout(r.ctx, 60*time.Second)
	stop := context.AfterFunc(caller, cancel)
	r.active[transcriptionSlot] = cancel
	r.wg.Add(1)
	r.mu.Unlock()
	defer r.wg.Done()
	defer cancel()
	defer stop()
	defer func() { r.mu.Lock(); delete(r.active, transcriptionSlot); r.mu.Unlock() }()
	var text string
	err = r.options.Credentials.With(ctx, func(key []byte) error {
		if !validSecret(key) {
			return ErrUnavailable
		}
		var e error
		text, e = r.options.Transcriber(ctx, key, audio, q.Format)
		if strings.Contains(text, string(key)) {
			return ErrUnavailable
		}
		return e
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil || r.valid(caller, q.Epoch) != nil || q.Revision != r.state.Revision || !r.state.Config.Enabled {
		return "", ErrStale
	}
	// Never expose upstream bodies/errors (which may contain credentials or audio).
	if err != nil {
		return "", ErrUnavailable
	}
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || len(text) > MaxPromptBytes {
		return "", ErrBudget
	}
	return text, nil
}
