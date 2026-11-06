// Package aitranscription uploads bounded, ephemeral recordings to the existing
// OpenAI provider. It never opens files, logs payloads, retries or persists audio.
package aitranscription

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

var errTranscription = errors.New("audio transcription unavailable")

func Transcribe(ctx context.Context, key, audio []byte, format string) (string, error) {
	return transcribe(ctx, key, audio, format, http.DefaultTransport)
}
func transcribe(ctx context.Context, key, audio []byte, format string, transport http.RoundTripper) (string, error) {
	if len(audio) == 0 || len(audio) > assistant.MaxAudioBytes || (format != "webm" && format != "mp4" && format != "wav") {
		return "", errTranscription
	}
	var body bytes.Buffer
	defer func() { clear(body.Bytes()) }()
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", assistant.TranscriptionModel); err != nil {
		return "", errTranscription
	}
	file, err := writer.CreateFormFile("file", "recording."+format)
	if err != nil {
		return "", errTranscription
	}
	if _, err = file.Write(audio); err != nil {
		return "", errTranscription
	}
	if writer.Close() != nil {
		return "", errTranscription
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", &body)
	if err != nil {
		return "", errTranscription
	}
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return "", errTranscription
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errTranscription
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	defer clear(raw)
	if err != nil || len(raw) > 64<<10 {
		return "", errTranscription
	}
	var result struct {
		Text string `json:"text"`
	}
	if jsoncodec.Unmarshal(raw, &result) != nil {
		return "", errTranscription
	}
	return result.Text, nil
}
