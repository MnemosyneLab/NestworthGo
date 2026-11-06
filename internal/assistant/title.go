package assistant

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

const TitleModel = "gpt-6-luna"
const MaxTitleRunes = 64

// TitleGenerator has no conversation history, page reader or tool authority.
// The key is leased from the same configured provider as the main conversation.
type TitleGenerator func(context.Context, ModelConfig, []byte, string) (string, error)

func titleText(text string, limit int) string {
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }), " ")
	runes := []rune(text)
	if len(runes) > limit {
		text = string(runes[:limit-1]) + "…"
	}
	return text
}
func localTitle(prompt string) string { return titleText(prompt, 48) }
func validTitleState(c ConversationState) bool {
	if !utf8.ValidString(c.Title) || utf8.RuneCountInString(c.Title) > MaxTitleRunes || c.Title != titleText(c.Title, MaxTitleRunes) {
		return false
	}
	switch c.TitleSource {
	case "":
		return c.Title == "" && !c.TitleAttempted
	case "local":
		return true
	case "pending", "luna", "manual":
		return c.Title != "" && c.TitleAttempted
	}
	return false
}

func (r *Runtime) Rename(ctx context.Context, epoch, id, title string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(ctx, epoch); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > MaxTitleRunes || title != titleText(title, MaxTitleRunes) {
		return ErrBudget
	}
	c := r.find(id)
	if c == nil {
		return ErrStale
	}
	tainted := false
	_ = r.options.Credentials.With(ctx, func(key []byte) error {
		if validSecret(key) && strings.Contains(title, string(key)) {
			tainted = true
		}
		return nil
	})
	if tainted {
		return ErrUnavailable
	}
	r.cancelTitle(id)
	c.Title, c.TitleSource, c.TitleAttempted = title, "manual", true
	return r.persist()
}

// Caller holds mu. Persist the attempt before dispatch: no retries on restart,
// duplicate Start, failed generation or a later turn. Main answers never wait.
func (r *Runtime) startTitle(id string, cfg ModelConfig, prompt string) {
	c := r.find(id)
	if c == nil || c.TitleAttempted || c.TitleSource == "manual" || r.closed {
		return
	}
	if len(c.Runs) > 0 {
		prompt = c.Runs[0].Run.Prompt
	}
	c.TitleAttempted = true
	c.TitleSource = "local"
	if c.Title == "" || r.options.TitleGenerator == nil || !cfg.Enabled || cfg.Provider != "openai-responses" {
		_ = r.persist()
		return
	}
	c.TitleSource = "pending"
	if r.persist() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, r.options.TitleTimeout)
	if r.titles == nil {
		r.titles = map[string]context.CancelFunc{}
	}
	r.titles[id] = cancel
	revision := r.state.Revision
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		title, err := r.generateTitle(r.usageContext(ctx, id, "", "title"), cfg, revision, titleText(prompt, 512))
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.titles, id)
		c := r.find(id)
		if c == nil || c.TitleSource != "pending" {
			return
		}
		c.TitleSource = "local"
		if err == nil && ctx.Err() == nil && r.valid(ctx, r.epoch) == nil && r.state.Revision == revision && r.state.Config.Enabled {
			c.Title, c.TitleSource = title, "luna"
		}
		if !r.closed {
			_ = r.persist()
		}
	}()
}
func (r *Runtime) generateTitle(ctx context.Context, cfg ModelConfig, revision uint64, prompt string) (title string, err error) {
	defer func() {
		if recover() != nil {
			title, err = "", ErrUnavailable
		}
	}()
	var key []byte
	defer func() { clear(key) }()
	err = func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.valid(ctx, r.epoch) != nil || r.state.Revision != revision {
			return ErrStale
		}
		return r.options.Credentials.With(ctx, func(value []byte) error {
			if !validSecret(value) || strings.Contains(prompt, string(value)) {
				return ErrUnavailable
			}
			key = append([]byte(nil), value...)
			return nil
		})
	}()
	if err != nil {
		return "", ErrUnavailable
	}
	cfg.Model = TitleModel
	title, err = r.options.TitleGenerator(ctx, cfg, key, prompt)
	if err != nil || len(title) > 1024 || !utf8.ValidString(title) || strings.Contains(title, string(key)) {
		return "", ErrUnavailable
	}
	title = titleText(strings.Trim(title, "\"“”"), MaxTitleRunes)
	if title == "" {
		return "", ErrUnavailable
	}
	return title, nil
}
func (r *Runtime) cancelTitle(id string) {
	if cancel := r.titles[id]; cancel != nil {
		cancel()
	}
	if c := r.find(id); c != nil && c.TitleSource == "pending" {
		c.TitleSource = "local"
	}
}
func (r *Runtime) cancelTitles() {
	for id := range r.titles {
		r.cancelTitle(id)
	}
}
