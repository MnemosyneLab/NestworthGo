package mcpserver

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

type DataRepairJobInput struct {
	HealthReadInput
	View  string `json:"view,omitempty" jsonschema:"progress (default) reads only job counters; evidence captures pageable job details. Neither scans health; call scan_data_health after completion."`
	JobID string `json:"jobId" jsonschema:"Exact jobId returned by start_data_repair. Jobs are retained only in the running application process."`
}

func (s *Service) dataHealthTools(server *toolSet, mode string) {
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
	readTool(server, "get_data_repair_job", "Poll exact process-local jobId. Default progress returns sequence, phase, outcome and counters without a health scan or capture. view=evidence freezes job items/blockers/prerequisites (25/max 100, 64 KiB). After terminal outcome, scan_data_health once for current gaps. Rebuilt days do not prove complete data; not_found after restart leaves completion unknown.", func(ctx context.Context, in DataRepairJobInput) (any, error) {
		id := strings.TrimSpace(in.JobID)
		if _, err := uuid.Parse(id); err != nil {
			return nil, fail("validation", "jobId must be a UUID returned by start_data_repair")
		}
		if in.View != "" && in.View != "progress" && in.View != "evidence" {
			return nil, fail("validation", "view must be progress or evidence")
		}
		if in.View != "evidence" {
			job, ok := s.app.GetSyncJobProgress(id)
			if !ok {
				return nil, fail("not_found", "repair job is unavailable in this process; scan_data_health to check current data; completion remains unknown")
			}
			return map[string]any{"job": map[string]any{
				"jobId": job.JobID, "sequence": job.Sequence, "phase": job.Phase, "outcome": job.Outcome,
				"estimatedRequests": job.EstimatedRequests, "completedRequests": job.CompletedRequests,
				"targetCount": job.TargetCount, "completedTargets": job.CompletedTargets,
				"snapshotDaysPlanned": job.SnapshotDaysPlanned, "snapshotDaysRebuilt": job.SnapshotDaysRebuilt,
				"committedBatches": job.CommittedBatches, "itemCount": job.ItemCount, "blockerCount": job.BlockerCount, "prerequisiteCount": job.PrerequisiteCount,
				"errorCode": job.ErrorCode, "nextEligibilityAt": job.NextEligibilityAt,
			}, "healthIncluded": false}, nil
		}
		job, ok := s.app.GetSyncJob(id)
		if !ok {
			return nil, fail("not_found", "repair job is unavailable in this process; scan_data_health to check current data; completion remains unknown")
		}
		return s.buildRead(ctx, generation, in.Limit, func(context.Context) (readCapture, error) {
			c, err := sectionedReadCapture("data_repair_job", in, marketdata.SyncSnapshotDTO(job), "items", "blockers", "prerequisites")
			if err != nil {
				return c, err
			}
			c.Summary = map[string]any{"job": c.Summary, "healthIncluded": false}
			for _, name := range []string{"items", "blockers", "prerequisites"} {
				c.Sections["job_"+name] = c.Sections[name]
				delete(c.Sections, name)
			}
			return c, nil
		})
	})

	if mode != LedgerWrite {
		return
	}
	open := true
	registerTool(server, &agentcapability.Definition{
		Name:        "start_data_repair",
		Description: "Start the same repair_all job as the UI, fetching configured provider data and rebuilding derived snapshots. Requires ledger_write. Inspect scan_data_health and preview_data_repair first. Uses provider limits and existing application write guards. Reuse operationId for identical retries. The durable receipt records job submission only: inspect attached/conflict and poll the returned jobId to learn completion and remaining issues. A conflicting job means repair_all was not scheduled. An accepted job continues independently of this request or MCP connection.",
		Effect:      agentcapability.Write, Annotations: &agentcapability.Annotations{IdempotentHint: true, OpenWorldHint: &open},
	}, func(ctx context.Context, in Mutation[Empty]) (Response, error) {
		value, err := s.execute(ctx, in.OperationID, "start_data_repair", in.Input, func(ctx context.Context) (any, error) {
			result, err := s.app.StartMarketDataSync(ctx, application.SyncRequest{Scope: application.SyncScopeRepairAll})
			if err != nil {
				return nil, err
			}
			return marketdata.SyncStartResultDTO{Job: marketdata.SyncSnapshotDTO(result.Job), Attached: result.Attached, Conflict: result.Conflict, Reason: result.Reason}, nil
		})
		if err != nil {
			return Response{}, safeError(err)
		}
		return Response{Data: value}, nil
	})
}
