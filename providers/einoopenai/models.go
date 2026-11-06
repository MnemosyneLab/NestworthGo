package einoopenai

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// The account-visible catalogue is not a Responses/function-tool capability
// guarantee. Return IDs, never a fabricated fixed list or provider error body.
func ListModels(ctx context.Context, key []byte) ([]string, error) {
	return listModels(ctx, key, http.DefaultTransport)
}
func listModels(ctx context.Context, key []byte, transport http.RoundTripper) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+string(key))
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, assistant.ErrUnavailable
	}
	raw, err := boundedRead(resp.Body, 1<<20)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	var list struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if jsoncodec.Unmarshal(raw, &list) != nil || list.Object != "list" || list.Data == nil {
		return nil, assistant.ErrUnavailable
	}
	result := []string{}
	seen := map[string]bool{}
	for _, m := range list.Data {
		if (assistant.ModelConfig{Provider: "openai-responses", Model: m.ID}).Valid() && !seen[m.ID] {
			seen[m.ID] = true
			result = append(result, m.ID)
		}
	}
	sort.Strings(result)
	return result, nil
}
