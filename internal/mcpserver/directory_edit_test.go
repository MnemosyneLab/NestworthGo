package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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
		"no fields":              {"id": id},
		"empty name":             {"id": id, "name": "", "iconKey": "fund"},
		"blank icon":             {"id": id, "name": "Changed", "iconKey": " "},
		"unknown icon":           {"id": id, "name": "Changed", "iconKey": "not-an-icon"},
		"missing member":         {"id": uuid.NewString(), "name": "Changed", "iconKey": "fund"},
		"empty name and archive": {"id": id, "name": "", "archived": true},
		"blank icon and archive": {"id": id, "iconKey": " ", "archived": true},
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

// update_* accepts archived on the same call and receipt as a name or icon
// edit. Omitted archived keeps the stored archive state.
func TestDirectoryUpdateArchivesAndRestores(t *testing.T) {
	t.Parallel()
	s, app, changes := fixture(t)
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
		before := changes.Load()
		receipt := call(t, c, entity.tool, map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"id": entity.id, "archived": true}}, false)["data"].(map[string]any)
		if receipt["tool"] != entity.tool || receipt["status"] != "succeeded" || changes.Load() != before+1 {
			t.Fatalf("%s archive receipt=%v notifications %d->%d", entity.tool, receipt, before, changes.Load())
		}
		archived := receipt["result"].(map[string]any)
		if archived["id"] != entity.id || archived["archivedAt"] == nil || archived["archivedAt"] == "" {
			t.Fatalf("%s archived result=%v", entity.tool, archived)
		}
		renamed := mutate(t, c, entity.tool, map[string]any{"id": entity.id, "name": "Archived name"})
		if renamed["name"] != "Archived name" || renamed["archivedAt"] == nil {
			t.Fatalf("%s rename dropped archive state: %v", entity.tool, renamed)
		}
		restored := mutate(t, c, entity.tool, map[string]any{"id": entity.id, "archived": false, "iconKey": "fund"})
		if restored["archivedAt"] != nil || restored["name"] != "Archived name" || restored["iconKey"] != "fund" {
			t.Fatalf("%s restore=%v", entity.tool, restored)
		}
	}
	if stored := storedMember(t, app, memberID); stored.ArchivedAt != nil || stored.Name != "Archived name" || stored.IconKey == nil || *stored.IconKey != "fund" {
		t.Fatalf("stored member %+v", stored)
	}
}

// The household's last active member cannot be archived. A rejected archive
// must not apply a name sent in the same call. A member who solely owns a
// live account cannot be archived while another member is active.
func TestDirectoryUpdateRejectsArchivingLastActiveMember(t *testing.T) {
	t.Parallel()
	s, app, changes := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	members, err := app.ListMembers(context.Background(), false)
	if err != nil || len(members) != 1 || members[0].Name != "Alice" {
		t.Fatalf("members %+v %v", members, err)
	}
	alice := members[0].ID.String()
	before := changes.Load()
	if got := rejectedUpdate(t, c, "update_member", map[string]any{"id": alice, "name": "Renamed", "archived": true}); !strings.Contains(got, "at least one active member") {
		t.Fatalf("last member archive error=%s", got)
	}
	if changes.Load() != before {
		t.Fatal("rejected archive emitted a change notification")
	}
	if stored := storedMember(t, app, alice); stored.Name != "Alice" || stored.ArchivedAt != nil {
		t.Fatalf("rejected archive changed Alice: %+v", stored)
	}
	bob := mutate(t, c, "create_member", map[string]string{"name": "Bob"})
	bobID := bob["id"].(string)
	mutate(t, c, "update_member", map[string]any{"id": bobID, "archived": true})
	if got := rejectedUpdate(t, c, "update_member", map[string]any{"id": alice, "archived": true}); !strings.Contains(got, "at least one active member") {
		t.Fatalf("archiving the only remaining active member: %s", got)
	}
	if stored := storedMember(t, app, alice); stored.Name != "Alice" || stored.ArchivedAt != nil {
		t.Fatalf("Alice changed after the second rejection: %+v", stored)
	}
	mutate(t, c, "update_member", map[string]any{"id": bobID, "archived": false})
	group := mutate(t, c, "create_group", map[string]string{"name": "Cash"})
	institution := mutate(t, c, "create_institution", map[string]string{"name": "Bank", "institutionType": "bank"})
	mutate(t, c, "create_account", map[string]any{"initialAmount": "0", "name": "Cash", "accountType": "cash_on_hand", "balanceSheetRole": "asset", "trackingMode": "balance", "defaultCurrency": "USD", "includeInNetWorth": true, "includeInPortfolio": false, "includeInLiquidAssets": true, "ownerIds": []string{alice}, "groupId": group["id"], "institutionId": institution["id"]})
	if got := rejectedUpdate(t, c, "update_member", map[string]any{"id": alice, "name": "Renamed", "archived": true}); !strings.Contains(got, "no other active owner") {
		t.Fatalf("sole owner archive error=%s", got)
	}
	if stored := storedMember(t, app, alice); stored.Name != "Alice" || stored.ArchivedAt != nil {
		t.Fatalf("rejected sole-owner archive changed Alice: %+v", stored)
	}
	if bobStored := storedMember(t, app, bobID); bobStored.ArchivedAt != nil || bobStored.Name != "Bob" {
		t.Fatalf("Bob changed: %+v", bobStored)
	}
}

func rejectedUpdate(t *testing.T, c *mcp.ClientSession, tool string, input map[string]any) string {
	t.Helper()
	res, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"operationId": uuid.NewString(), "input": input}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("%s succeeded", tool)
	}
	raw, err := jsoncodec.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDirectoryUpdateRejectsNullOnIdentityFields(t *testing.T) {
	t.Parallel()
	s, app, _ := fixture(t)
	if _, err := s.Enable(DirectoryWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, s)
	member := mutate(t, c, "create_member", map[string]string{"name": "Bob"})
	id := member["id"].(string)
	for _, input := range []map[string]any{
		{"id": id, "name": nil},
		{"id": id, "iconKey": nil},
		{"id": id, "archived": nil},
		{"id": id, "name": nil, "archived": true},
	} {
		if got := rejectedUpdate(t, c, "update_member", input); !strings.Contains(got, "invalid write arguments") {
			t.Fatalf("null directory field error=%s", got)
		}
		if stored := storedMember(t, app, id); stored.Name != "Bob" || stored.ArchivedAt != nil || stored.IconKey == nil || *stored.IconKey != domain.DefaultMemberIcon {
			t.Fatalf("null input changed the member: %+v", stored)
		}
	}
}

func TestDirectoryArchiveToolsAreFoldedIntoUpdate(t *testing.T) {
	t.Parallel()
	s, _, _ := fixture(t)
	registry := s.toolRegistry(LedgerWrite)
	for _, name := range []string{"archive_member", "archive_institution", "archive_group"} {
		if _, err := registry.Definition(name); err == nil {
			t.Fatalf("%s should be folded into its update tool", name)
		}
	}
	for _, name := range []string{"archive_account", "archive_instrument"} {
		if _, err := registry.Definition(name); err != nil {
			t.Fatalf("%s should stay a separate tool", name)
		}
	}
	for _, name := range []string{"update_member", "update_institution", "update_group"} {
		definition, err := registry.Definition(name)
		if err != nil || !strings.Contains(definition.Description, "archived") {
			t.Fatalf("%s description=%v %v", name, definition, err)
		}
	}
	if _, err := s.toolRegistry(ReadOnly).Definition("update_member"); err == nil {
		t.Fatal("member updates must require directory_write")
	}
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
