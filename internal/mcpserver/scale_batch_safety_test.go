package mcpserver

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestScaleMetadataUnknownReceiptAndProtectedTargets(t *testing.T) {
	fx := newLedgerFixture(t)
	c := ledgerSession(t, fx)
	read := func() map[string]any {
		cap := scaleCapture(t, c, "list_instruments", map[string]any{"filter": map[string]any{"ids": []string{fx.instrument}}})
		return scaleSection(t, c, cap, "instruments")[0].(map[string]any)
	}
	before, err := fx.app.ListInstruments(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	row := read()
	args := map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"id": fx.instrument, "expectedMetadataVersion": row["metadataVersion"], "changes": map[string]any{"shortName": "Alias"}}}}}
	call(t, c, "update_instruments_batch", args, false)
	op, err := fx.service.GetOperation(args["operationId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	op.Status = "pending"
	path, _ := fx.service.operationPath(op.ID)
	if err := fx.service.writePrivate(path, op); err != nil {
		t.Fatal(err)
	}
	row = read()
	call(t, c, "update_instruments_batch", args, true)
	if !reflect.DeepEqual(row, read()) {
		t.Fatal("unknown receipt replayed")
	}
	after, err := fx.app.ListInstruments(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	after[0].ShortName = before[0].ShortName
	after[0].UpdatedAt = before[0].UpdatedAt
	if !reflect.DeepEqual(before, after) {
		t.Fatal("nonmetadata instrument state changed")
	}
	if err := fx.app.ArchiveInstrument(t.Context(), domain.InstrumentID(fx.instrument), true); err != nil {
		t.Fatal(err)
	}
	cap := scaleCapture(t, c, "list_instruments", map[string]any{"includeArchived": true, "filter": map[string]any{"ids": []string{fx.instrument}}})
	archived := scaleSection(t, c, cap, "instruments")[0].(map[string]any)
	call(t, c, "update_instruments_batch", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{map[string]any{"id": fx.instrument, "expectedMetadataVersion": archived["metadataVersion"], "changes": map[string]any{"name": "Rejected"}}}}}, true)
}

func TestScalePermissionMatrix(t *testing.T) {
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			fx := newLedgerFixture(t)
			if _, err := fx.service.Enable(mode); err != nil {
				t.Fatal(err)
			}
			c := connect(t, fx.service)
			tools := featureToolList(t, c)
			for _, name := range []string{"get_read_page", "get_read_item", "get_market_data_batch"} {
				if tools[name] == nil || !tools[name].Annotations.ReadOnlyHint {
					t.Fatalf("read tool unavailable %s", name)
				}
			}
			writes := map[string]bool{"update_instruments_batch": mode != ReadOnly}
			for name, allowed := range writes {
				if (tools[name] != nil) != allowed {
					t.Fatalf("permission %s %s", mode, name)
				}
				if !allowed {
					result, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"items": []any{}}}})
					if err == nil && (result == nil || !result.IsError) {
						t.Fatal("unauthorized call accepted")
					}
				}
			}
		})
	}
}
