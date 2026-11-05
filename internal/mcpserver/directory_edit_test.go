package mcpserver

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// update_member|institution|group accept name, iconKey or both. Supplied
// fields apply in one write; omitted fields keep the stored value.
func TestDirectoryUpdateAcceptsNameAndIconTogether(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	member := mutate(t, c, "create_member", map[string]string{"name": "Bob"})
	memberID := member["id"].(string)
	group := mutate(t, c, "create_group", map[string]string{"name": "Savings"})
	institution := mutate(t, c, "create_institution", map[string]string{"name": "Bank One", "institutionType": "bank"})

	for _, entity := range []struct {
		tool string
		id   string
	}{{"update_member", memberID}, {"update_group", group["id"].(string)}, {"update_institution", institution["id"].(string)}} {
		both := mutate(t, c, entity.tool, map[string]any{"id": entity.id, "name": "Renamed", "iconKey": "fund"})
		if both["name"] != "Renamed" || both["iconKey"] != "fund" {
			t.Fatalf("%s both = %v", entity.tool, both)
		}
		iconOnly := mutate(t, c, entity.tool, map[string]any{"id": entity.id, "iconKey": "bond"})
		if iconOnly["name"] != "Renamed" || iconOnly["iconKey"] != "bond" {
			t.Fatalf("%s icon only = %v", entity.tool, iconOnly)
		}
		nameOnly := mutate(t, c, entity.tool, map[string]any{"id": entity.id, "name": "Again"})
		if nameOnly["name"] != "Again" || nameOnly["iconKey"] != "bond" {
			t.Fatalf("%s name only = %v", entity.tool, nameOnly)
		}
	}
	if stored := storedMember(t, app, memberID); stored.Name != "Again" || stored.IconKey == nil || *stored.IconKey != "bond" {
		t.Fatalf("stored member %+v", stored)
	}
}

// Invalid input must never apply the other supplied field.
func TestDirectoryUpdateRejectsInvalidInputWithoutPartialWrite(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	member := mutate(t, c, "create_member", map[string]string{"name": "Bob"})
	id := member["id"].(string)
	for name, input := range map[string]map[string]any{
		"no fields":      {"id": id},
		"empty name":     {"id": id, "name": "", "iconKey": "fund"},
		"blank icon":     {"id": id, "name": "Changed", "iconKey": " "},
		"unknown icon":   {"id": id, "name": "Changed", "iconKey": "not-an-icon"},
		"missing member": {"id": uuid.NewString(), "name": "Changed", "iconKey": "fund"},
	} {
		call(t, c, "update_member", map[string]any{"operationId": uuid.NewString(), "input": input}, true)
		if stored := storedMember(t, app, id); stored.Name != "Bob" || stored.IconKey == nil || *stored.IconKey != domain.DefaultMemberIcon {
			t.Fatalf("%s changed the member: %+v", name, stored)
		}
	}
}

func storedMember(t *testing.T, app *application.Service, id string) domain.Member {
	t.Helper()
	members, err := app.ListMembers(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if member.ID.String() == id {
			return member
		}
	}
	t.Fatalf("member %s not found", id)
	return domain.Member{}
}

func TestDirectoryIconToolsAreRemoved(t *testing.T) {
	t.Parallel()
	s, _, _ := fixture(t)
	registry := s.toolRegistry(LedgerWrite)
	for _, name := range []string{"set_member_icon", "set_institution_icon", "set_group_icon"} {
		if _, err := registry.Definition(name); err == nil {
			t.Fatalf("%s should be merged into its update tool", name)
		}
	}
}
