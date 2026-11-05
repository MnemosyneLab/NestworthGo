package assistant_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	ai "github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
)

func TestUnresolvedReviewSurvivesRetentionAndBlocksDeletion(t *testing.T) {
	dir := privateTestDirectory(t)
	store, err := aistate.Open(dir, "synthetic-ledger")
	if err != nil {
		t.Fatal(err)
	}
	state := ai.InitialState("synthetic-ledger")
	id := uuid.NewString()
	state.Conversations = []ai.ConversationState{{ID: id, Updated: time.Now().Add(-60 * 24 * time.Hour), Reviews: []ai.ReviewRef{{ID: uuid.NewString()}}}}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	r, _ := open(t, dir, &credentials{available: true, key: []byte(canary)}, factory(func(context.Context, []byte, string) (ai.Completion, error) {
		t.Fatal("retention called model")
		return ai.Completion{}, nil
	}), time.Second)
	defer r.Close()
	epoch := r.Snapshot().Epoch
	list, err := r.List(t.Context(), epoch)
	if err != nil || len(list) != 1 {
		t.Fatal("restart pruned unresolved operation")
	}
	if _, err := r.Create(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Inspect(t.Context(), epoch, id); err != nil {
		t.Fatal("creation pruned unresolved operation")
	}
	if err := r.Delete(t.Context(), epoch, id); err == nil {
		t.Fatal("deleted unresolved operation")
	}
}
