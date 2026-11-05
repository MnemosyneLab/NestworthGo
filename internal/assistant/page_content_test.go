package assistant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestPageToolCapturesOnDemandPaginatesAndRejectsChangedScreen(t *testing.T) {
	for _, page := range []string{"return-analysis", "overview", "accounts", "account", "available-funds", "portfolio", "history", "future-business"} {
		t.Run(page, func(t *testing.T) { checkPageCapture(t, page) })
	}
}

func checkPageCapture(t *testing.T, page string) {
	calls := make(chan ai.PageContentReader, 1)
	done := make(chan error, 1)
	f := func(ctx context.Context, _ ai.ModelConfig, _ []byte, _ string) (ai.Engine, error) {
		return &engine{fn: func(ctx context.Context, _ []byte, _ string) (ai.Completion, error) {
			calls <- ai.CurrentPageReader(ctx)
			<-ctx.Done()
			return ai.Completion{}, ctx.Err()
		}}, nil
	}
	r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, f, 5*time.Second)
	defer r.Close()
	enable(t, r)
	q := request(t, r)
	run, err := r.Start(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	reader := <-calls
	if reader == nil {
		t.Fatal("missing internal page reader")
	}
	snapshot := ai.PageContent{ID: uuid.NewString(), Page: page, CapturedAt: time.Now(), Nodes: []ai.PageNode{}}
	for i := range 80 {
		snapshot.Nodes = append(snapshot.Nodes, ai.PageNode{ID: fmt.Sprint(i), Module: "filters", Role: "text", Name: fmt.Sprintf("合成 Synthetic %d", i)})
	}
	snapshot.Nodes = append(snapshot.Nodes, ai.PageNode{ID: "trend-secret", Module: "trend", Role: "text", Name: "unrequested module data"})
	capture := func(args string, mutate func(*ai.PageContent)) (map[string]any, error) {
		t.Helper()
		result := make(chan json.RawMessage, 1)
		go func() { raw, err := reader(t.Context(), []byte(args)); result <- raw; done <- err }()
		deadline := time.Now().Add(time.Second)
		var requests []ai.PageReadRequest
		for time.Now().Before(deadline) {
			requests, _ = r.PendingPageReads(t.Context(), q.Epoch)
			if len(requests) > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if len(requests) != 1 {
			t.Fatal("tool did not request fresh capture")
		}
		p := snapshot
		p.CapturedAt = time.Now()
		if mutate != nil {
			mutate(&p)
		}
		if err := r.DeliverPageContent(t.Context(), q.Epoch, requests[0].ID, p); err != nil {
			t.Fatal(err)
		}
		raw := <-result
		err := <-done
		var data map[string]any
		if err == nil {
			if jsoncodec.Unmarshal(raw, &data) != nil {
				t.Fatal("invalid response")
			}
		}
		return data, err
	}
	first, err := capture(`{"module":"filters","limit":20}`, nil)
	if err != nil || first["hasMore"] != true || first["complete"] != false || first["nextCursor"] != float64(20) {
		t.Fatalf("first: %v %v", first, err)
	}
	firstRaw, _ := jsoncodec.Marshal(first)
	if strings.Contains(string(firstRaw), "unrequested module data") {
		t.Fatal("module read widened disclosure")
	}
	args := fmt.Sprintf(`{"snapshotId":%q,"module":"filters","cursor":20,"limit":64}`, snapshot.ID)
	last, err := capture(args, nil)
	if err != nil || last["hasMore"] != false || len(last["nodes"].([]any)) != 60 || last["complete"] != false {
		t.Fatalf("continuation: %v %v", last, err)
	}
	if _, err = capture(args, func(p *ai.PageContent) { p.ID = uuid.NewString() }); err == nil {
		t.Fatal("mixed async generation")
	}
	if _, err = capture(`{}`, func(p *ai.PageContent) {
		p.ID = uuid.NewString()
		p.Nodes = []ai.PageNode{{ID: "new", Module: "trend", Role: "status", Name: "最新 Latest"}}
	}); err != nil {
		t.Fatal(err)
	}

	// Selecting a structural module includes nested automatically derived modules,
	// but never its sibling module. Node pagination still uses the selected subtree.
	snapshot.ID = uuid.NewString()
	snapshot.Nodes = []ai.PageNode{{ID: "parent", Module: "container", Role: "region"}, {ID: "child", Parent: "parent", Module: "automatic-child", Role: "textbox", Value: "12.3400"}, {ID: "sibling", Module: "outside", Role: "text", Name: "not selected"}}
	nested, err := capture(`{"module":"container","limit":64}`, nil)
	if err != nil || len(nested["nodes"].([]any)) != 2 {
		t.Fatalf("nested module: %v %v", nested, err)
	}
	// The frontend's real ContributionTab tests prove that these exact control
	// properties change the semantic generation even when financial rows match.
	// Continue the same desktop protocol with that new ID: no old slice is returned.
	for _, control := range []string{"contribution-return-type", "contribution-group-by", "contribution-sort", "contribution-limit", "contribution-search", "contribution-loading"} {
		snapshot.ID = uuid.NewString()
		snapshot.Nodes = []ai.PageNode{{ID: "contribution/" + control, Module: "contribution", Role: "textbox", Value: "before"}, {ID: "contribution/amount", Module: "contribution", Role: "text", Name: "12.34 USD"}}
		if control == "contribution-loading" {
			snapshot.Nodes[0].Role = "group"
			snapshot.Nodes[0].Value = ""
			snapshot.Nodes[0].State = []string{"busy=false"}
		}
		initial, err := capture(`{"module":"contribution","limit":1}`, nil)
		if err != nil || initial["hasMore"] != true {
			t.Fatal("missing contribution continuation", err)
		}
		args := fmt.Sprintf(`{"snapshotId":%q,"module":"contribution","cursor":1}`, snapshot.ID)
		if _, err := capture(args, func(p *ai.PageContent) {
			p.ID = uuid.NewString()
			p.Nodes = append([]ai.PageNode(nil), p.Nodes...)
			if control == "contribution-loading" {
				p.Nodes[0].State = []string{"busy=true"}
			} else {
				p.Nodes[0].Value = "after"
			}
		}); err == nil {
			t.Fatalf("%s kept an old cursor despite a changed control", control)
		}
	}
	if page == "account" {
		for _, control := range []string{"account-composition-type", "account-composition-detail"} {
			snapshot.ID = uuid.NewString()
			snapshot.Nodes = []ai.PageNode{{ID: "composition/" + control, Module: "composition", Role: "radio", State: []string{"checked=true"}}, {ID: "composition/amount", Module: "composition", Role: "text", Name: "12.34 USD"}}
			if _, err := capture(`{"module":"composition","limit":1}`, nil); err != nil {
				t.Fatal(err)
			}
			args := fmt.Sprintf(`{"snapshotId":%q,"module":"composition","cursor":1}`, snapshot.ID)
			if _, err := capture(args, func(p *ai.PageContent) {
				p.ID = uuid.NewString()
				p.Nodes = append([]ai.PageNode(nil), p.Nodes...)
				p.Nodes[0].State = []string{"checked=false"}
			}); err == nil {
				t.Fatalf("%s retained an old continuation after selection changed", control)
			}
		}
	}
	for _, args := range []string{`{"cursor":2}`, `{"html":true}`, `{"limit":65}`, `null`} {
		if _, err := reader(t.Context(), []byte(args)); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
	if _, err := r.PendingPageReads(t.Context(), "wrong-epoch"); err == nil {
		t.Fatal("wrong desktop epoch")
	}
	if err := r.Cancel(t.Context(), q.Epoch, q.ConversationID, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reader(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("finished/canceled run retained reader authority")
	}
	if pending, _ := r.PendingPageReads(t.Context(), q.Epoch); len(pending) != 0 {
		t.Fatal("canceled run requested capture")
	}
}

func TestPageContentRejectsSecretsAndMalformedTrees(t *testing.T) {
	calls := make(chan ai.PageContentReader, 1)
	f := func(ctx context.Context, _ ai.ModelConfig, _ []byte, _ string) (ai.Engine, error) {
		return &engine{fn: func(ctx context.Context, _ []byte, _ string) (ai.Completion, error) {
			calls <- ai.CurrentPageReader(ctx)
			<-ctx.Done()
			return ai.Completion{}, ctx.Err()
		}}, nil
	}
	r, _ := open(t, privateTestDirectory(t), &credentials{available: true}, f, time.Second)
	defer r.Close()
	enable(t, r)
	q := request(t, r)
	r.Start(t.Context(), q)
	reader := <-calls
	ctx, cancel := context.WithCancel(t.Context())
	go reader(ctx, []byte(`{}`))
	defer cancel()
	var req []ai.PageReadRequest
	for i := 0; i < 100; i++ {
		req, _ = r.PendingPageReads(t.Context(), q.Epoch)
		if len(req) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(req) != 1 {
		t.Fatal("missing request")
	}
	for _, n := range []ai.PageNode{{ID: "a", Module: "filters", Role: "text", Name: canary}, {ID: "a", Module: "filters", Role: "html", Name: "bad role"}, {ID: "a", Parent: "missing", Module: "filters", Role: "text"}, {ID: "a", Module: "filters", Role: "text", Name: strings.Repeat("x", 2049)}} {
		p := ai.PageContent{ID: uuid.NewString(), Page: "return-analysis", CapturedAt: time.Now(), Nodes: []ai.PageNode{n}}
		if r.DeliverPageContent(t.Context(), q.Epoch, req[0].ID, p) == nil {
			t.Fatal("unsafe snapshot accepted")
		}
	}
	for _, page := range []string{"", "../invalid", "UPPERCASE", strings.Repeat("a", 65), "unsupported"} {
		p := ai.PageContent{ID: uuid.NewString(), Page: page, CapturedAt: time.Now(), Nodes: []ai.PageNode{{ID: "safe", Module: "page", Role: "region"}}}
		if r.DeliverPageContent(t.Context(), q.Epoch, req[0].ID, p) == nil {
			t.Fatalf("accepted malformed page identity %q", page)
		}
	}
	cancel()
}
