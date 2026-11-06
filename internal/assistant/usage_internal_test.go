package assistant

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryUsage struct {
	calls map[string]UsageCall
	fail  bool
}

func (s *memoryUsage) Record(c UsageCall) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.calls[c.ID] = c
	return nil
}
func (s *memoryUsage) Calls() ([]UsageCall, error) {
	out := []UsageCall{}
	for _, c := range s.calls {
		out = append(out, c)
	}
	return out, nil
}
func (s *memoryUsage) Close() error { return nil }
func usageRuntime(s UsageStore) *Runtime {
	return &Runtime{ctx: context.Background(), epoch: "epoch", state: InitialState("test"), options: Options{Usage: s, Validate: func(ctx context.Context) bool { return ctx.Err() == nil }}}
}
func TestUsageCaptureUnknownZeroAndSubsets(t *testing.T) {
	c := UsageCall{}
	CaptureUsage(&c, []byte(`{"model":"actual-snapshot","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":80,"cache_write_tokens":10},"output_tokens":30,"output_tokens_details":{"reasoning_tokens":20}}}`))
	var total UsageTotal
	total.add(c)
	if c.Model != "actual-snapshot" || total.Tokens != 130 || total.CachedInput != 80 || total.CacheWrite != 10 || total.Reasoning != 20 {
		t.Fatalf("wrong usage: %+v", total)
	}
	CaptureUsage(&c, []byte(`{"usage":{"input_tokens":0,"output_tokens":0}}`))
	if c.Input == nil || *c.Input != 0 || c.CachedInput != nil || c.Reasoning != nil {
		t.Fatal("omission became zero")
	}
	CaptureUsage(&c, []byte(`{"usage":{"input_tokens":-1,"output_tokens":2.5,"input_tokens_details":{"cached_tokens":null}}}`))
	if c.Input != nil || c.Output != nil || c.CachedInput != nil {
		t.Fatal("invalid counter accepted")
	}
	CaptureUsage(&c, []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":4}}}`))
	if c.CachedInput != nil || c.Reasoning != nil {
		t.Fatal("impossible subset accepted")
	}
}
func TestUsageRoundsDedupFiltersDeletionAndTimezone(t *testing.T) {
	store := &memoryUsage{calls: map[string]UsageCall{}}
	r := usageRuntime(store)
	ctx := r.usageContext(context.Background(), "conversation", "run", "conversation")
	c := BeginUsage(ctx, "alias")
	c.Started = time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	CaptureUsage(&c, []byte(`{"model":"snapshot","usage":{"input_tokens":100,"output_tokens":25}}`))
	FinishUsage(ctx, c, true)
	FinishUsage(ctx, c, true) // replayed terminal observation
	d := BeginUsage(ctx, "alias")
	d.Started = c.Started.Add(time.Hour)
	FinishUsage(ctx, d, false)
	title := r.usageContext(context.Background(), "conversation", "", "title")
	e := BeginUsage(title, "title-model")
	e.Started = c.Started
	FinishUsage(title, e, true)
	query := UsageQuery{Timezone: "America/New_York", From: "2026-11-01", To: "2026-11-01", GroupBy: "day"}
	report, err := r.Usage(context.Background(), "epoch", query)
	if err != nil || report.Total.Calls != 3 || report.Total.Tokens != 125 || report.Total.MissingTokens != 2 || report.Total.Failed != 1 || len(report.Groups) != 1 {
		t.Fatalf("report %+v %v", report, err)
	}
	query.GroupBy = "conversation"
	report, _ = r.Usage(context.Background(), "epoch", query)
	if !report.Groups[0].Deleted {
		t.Fatal("deleted conversation lost")
	}
	query.ConversationID = "conversation"
	query.RunID = "run"
	query.GroupBy = "model"
	query.Model = "snapshot"
	report, _ = r.Usage(context.Background(), "epoch", query)
	if report.Total.Calls != 1 || report.Total.Tokens != 125 {
		t.Fatal("run/model filter")
	}
	query.Model = ""
	query.Timezone = "Pacific/Honolulu"
	query.To = "2026-10-31"
	query.From = "2026-10-31"
	report, _ = r.Usage(context.Background(), "epoch", query)
	if report.Total.Calls != 2 {
		t.Fatal("timezone boundary")
	}
	query.Timezone = "invalid"
	if _, err = r.Usage(context.Background(), "epoch", query); err == nil {
		t.Fatal("accepted invalid timezone")
	}
}
func TestUsageFailureIsIndependentAndAudioIsNotTokens(t *testing.T) {
	store := &memoryUsage{calls: map[string]UsageCall{}, fail: true}
	r := usageRuntime(store)
	c := BeginUsage(r.usageContext(context.Background(), "", "", "transcription"), TranscriptionModel)
	CaptureUsage(&c, []byte(`{"usage":{"type":"duration","seconds":12.345}}`))
	c.Kind = "transcription"
	var total UsageTotal
	total.add(c)
	if total.AudioSeconds != "12.345" || total.MissingTokens != 1 || !r.usageFailed.Load() {
		t.Fatal("coverage loss hidden")
	}
	ctx := WithUsage(context.Background(), func(UsageCall) { panic("observer failed") })
	FinishUsage(ctx, BeginUsage(ctx, "mock"), true)
}

func TestUsageDurationRejectsUnboundedDecimalExponent(t *testing.T) {
	for _, raw := range []string{`{"usage":{"type":"duration","seconds":1e-999999999}}`, `{"usage":{"type":"duration","seconds":1e999999999}}`} {
		c := UsageCall{}
		CaptureUsage(&c, []byte(raw))
		if c.AudioSeconds != nil {
			t.Fatal("unbounded duration accepted")
		}
	}
}
