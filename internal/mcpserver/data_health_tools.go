package mcpserver

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type DataRepairJobInput struct {
	JobID string `json:"jobId" jsonschema:"Exact jobId returned by start_data_repair. Jobs are retained only in the running application process."`
}

type DataRepairJobResult struct {
	Job    marketdata.SyncJobDTO                `json:"job"`
	Health marketdata.MarketDataHealthReportDTO `json:"health"`
}

func (s *Service) dataHealthTools(server *mcp.Server, mode string) {
	readTool(server, "scan_data_health", "Run the same local Data Health scan as the UI, without provider requests. Returns issues, executable repairs, prerequisites and missing/outdated/incomplete snapshots. Missing data is not zero.", func(ctx context.Context, _ Empty) (any, error) {
		report, err := s.app.ScanMarketDataHealth(ctx)
		return marketdata.HealthReportDTO(report), err
	})
	readTool(server, "preview_data_repair", "Preview the UI repair_all plan locally: targets, estimated provider requests, snapshot work and unresolved prerequisites. No data is repaired. This is an estimate, not a locked ledger preview; starting repair replans against current data.", func(ctx context.Context, _ Empty) (any, error) {
		preview, err := s.app.PreviewMarketDataSync(ctx, application.SyncRequest{Scope: application.SyncScopeRepairAll})
		return marketdata.SyncPreviewDTO(preview), err
	})
	readTool(server, "get_data_repair_job", "Read progress/result for the exact repair jobId plus a fresh local health scan. Inspect outcome, blockers, snapshotDaysPlanned and snapshotDaysRebuilt. Rebuilt snapshots can still be incomplete. health is current state, not an immutable completion report; during running it is provisional. Jobs are process-local; not_found after restart does not imply success or failure. Use scan_data_health to recheck actual data.", func(ctx context.Context, in DataRepairJobInput) (any, error) {
		id := strings.TrimSpace(in.JobID)
		if _, err := uuid.Parse(id); err != nil {
			return nil, fail("validation", "jobId must be a UUID returned by start_data_repair")
		}
		job, ok := s.app.GetSyncJob(id)
		if !ok {
			return nil, fail("not_found", "repair job is unavailable in this process; scan_data_health to check current data")
		}
		report, err := s.app.ScanMarketDataHealth(ctx)
		if err != nil {
			return nil, err
		}
		return DataRepairJobResult{Job: marketdata.SyncSnapshotDTO(job), Health: marketdata.HealthReportDTO(report)}, nil
	})
	if mode != LedgerWrite {
		return
	}
	open := true
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_data_repair",
		Description: "Start the same repair_all job as the UI, fetching configured provider data and rebuilding derived snapshots. Requires ledger_write. Inspect scan_data_health and preview_data_repair first. Uses provider limits and existing application write guards. Reuse operationId for identical retries. The durable receipt records job submission only: inspect attached/conflict and poll the returned jobId to learn completion and remaining issues. A conflicting job means repair_all was not scheduled. An accepted job continues independently of this request or MCP connection.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: &open},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in Mutation[Empty]) (*mcp.CallToolResult, Response, error) {
		value, err := s.execute(ctx, in.OperationID, "start_data_repair", in.Input, func(ctx context.Context) (any, error) {
			result, err := s.app.StartMarketDataSync(ctx, application.SyncRequest{Scope: application.SyncScopeRepairAll})
			if err != nil {
				return nil, err
			}
			return marketdata.SyncStartResultDTO{Job: marketdata.SyncSnapshotDTO(result.Job), Attached: result.Attached, Conflict: result.Conflict, Reason: result.Reason}, nil
		})
		if err != nil {
			return nil, Response{}, safeError(err)
		}
		return nil, Response{Data: value}, nil
	})
}
