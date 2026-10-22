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
	HealthReadInput
	JobID string `json:"jobId" jsonschema:"Exact jobId returned by start_data_repair. Jobs are retained only in the running application process."`
}

type DataRepairJobResult struct {
	Job    marketdata.SyncJobDTO                `json:"job"`
	Health marketdata.MarketDataHealthReportDTO `json:"health"`
}

func (s *Service) dataHealthTools(server *mcp.Server, mode string) {
	generation := s.contextGeneration
	readTool(server, "scan_data_health", "Run the same local Data Health scan as the UI, without provider requests. The bounded read contract returns frozen issue pages with complete header/counts; default 25/max 100 and 64 KiB complete wire. Missing data is not zero.", func(ctx context.Context, in HealthReadInput) (any, error) {
		read := func(ctx context.Context) (any, error) {
			report, err := s.app.ScanMarketDataHealth(ctx)
			return marketdata.HealthReportDTO(report), err
		}
		return s.pagedHealth(ctx, generation, in, "data_health", read, "issues")
	})
	readTool(server, "preview_data_repair", "Preview repair_all locally: targets, provider estimates, snapshot work and unresolved prerequisites. The bounded read contract independently pages items/unresolved while preserving all totals. Default 25/max 100, 64 KiB complete wire. No repairs; this is an estimate and start replans current data.", func(ctx context.Context, in HealthReadInput) (any, error) {
		read := func(ctx context.Context) (any, error) {
			preview, err := s.app.PreviewMarketDataSync(ctx, application.SyncRequest{Scope: application.SyncScopeRepairAll})
			return marketdata.SyncPreviewDTO(preview), err
		}
		return s.pagedHealth(ctx, generation, in, "data_repair_preview", read, "items", "unresolved")
	})
	readTool(server, "get_data_repair_job", "Read progress/result for the exact repair jobId plus a fresh local health scan. The bounded read contract separates issues and job items/blockers/prerequisites with 64 KiB pages (default 25/max 100). Inspect outcome, blockers, snapshotDaysPlanned and snapshotDaysRebuilt. Rebuilt snapshots can still be incomplete. health is current state, not an immutable completion report; during running it is provisional. Jobs are process-local; not_found after restart does not imply success or failure. Use scan_data_health to recheck actual data.", func(ctx context.Context, in DataRepairJobInput) (any, error) {
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
		value := DataRepairJobResult{Job: marketdata.SyncSnapshotDTO(job), Health: marketdata.HealthReportDTO(report)}
		return s.buildRead(ctx, generation, in.Limit, func(context.Context) (readCapture, error) {
			c, err := sectionedReadCapture("data_repair_job", in, value.Health, "issues")
			if err != nil {
				return c, err
			}
			j, err := sectionedReadCapture("data_repair_job", in, value.Job, "items", "blockers", "prerequisites")
			if err != nil {
				return c, err
			}
			c.Summary = map[string]any{"job": j.Summary, "health": c.Summary, "healthScope": "current local scan, independent of recorded job completion"}
			for name, rows := range j.Sections {
				c.Sections["job_"+name] = rows
			}
			return c, nil
		})
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
