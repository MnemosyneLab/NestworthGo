package einoopenai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
)

func TestProviderHTTPFailuresAreFixedAndNeverReflectBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{400, assistant.ErrProviderRequest}, {401, assistant.ErrProviderAuthentication}, {403, assistant.ErrProviderAuthentication}, {429, assistant.ErrProviderRateLimited}, {503, assistant.ErrProviderUnavailable}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			fake := fakeModel(t, Limits{}, func(w http.ResponseWriter, r *http.Request, round int, request obj) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":{"message":"synthetic-canary-key financial-canary","code":"financial-canary"}}`)
			})
			_, session, _ := ledgerSession(t)
			engine, err := engineWithModel(t.Context(), assistant.ModelConfig{Provider: "openai-responses", Model: "mock"}, fake.model, builtinai.New(session, assistant.PageSelection{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			_, err = engine.Complete(context.Background(), nil, "hello")
			if !errors.Is(err, tc.want) || err.Error() != tc.want.Error() {
				t.Fatalf("unsafe failure %v", err)
			}
			if fake.calls.calls.Load() != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}
