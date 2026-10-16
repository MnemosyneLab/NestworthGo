package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type ReviewV2PageInput struct {
	Cursor   string `json:"cursor,omitempty"`
	PageSize int    `json:"pageSize" jsonschema:"1–100; follow the returned cursor until empty"`
}

type ReviewV2AlertPageInput struct {
	Filter   domain.ReviewAlertFilter `json:"filter"`
	Cursor   string                   `json:"cursor,omitempty"`
	PageSize int                      `json:"pageSize" jsonschema:"1–100 complete investment groups"`
}

type ReviewV2AlertInput struct {
	EpisodeID string `json:"episodeId"`
}

type ReviewV2CoverageInput struct {
	Filter   domain.ReviewCoverageFilter `json:"filter"`
	Cursor   string                      `json:"cursor,omitempty"`
	PageSize int                         `json:"pageSize" jsonschema:"1–100 current positive-holding investment groups"`
}

type ReviewV2PreviewInput struct {
	Spec                map[string]any `json:"spec"`
	InstrumentID        string         `json:"instrumentId"`
	LatestClosedOn      string         `json:"latestClosedOn,omitempty"`
	ExpectedSourceToken string         `json:"expectedSourceToken,omitempty"`
}

// The MCP SDK's JSON-schema inference rejects the recursive predicate node.
// A map retains an ordinary JSON object on the wire; the domain's strict
// decoder checks every nested key before either evaluation or persistence.
type ReviewV2RuleInput struct {
	RuleID           string         `json:"ruleId"`
	MutationID       string         `json:"mutationId"`
	ExpectedRevision int            `json:"expectedRevision"`
	Name             string         `json:"name"`
	Enabled          bool           `json:"enabled"`
	Severity         string         `json:"severity"`
	Spec             map[string]any `json:"spec"`
}

func reviewV2Spec(value map[string]any) (domain.ReviewRuleSpec, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return domain.ReviewRuleSpec{}, err
	}
	return domain.DecodeReviewRuleSpec(raw)
}

func reviewV2RuleInput(in ReviewV2RuleInput) (application.SaveReviewRuleV2Input, error) {
	spec, err := reviewV2Spec(in.Spec)
	if err != nil {
		return application.SaveReviewRuleV2Input{}, err
	}
	return application.SaveReviewRuleV2Input{RuleID: in.RuleID, MutationID: in.MutationID, ExpectedRevision: in.ExpectedRevision, Name: in.Name, Enabled: in.Enabled, Severity: in.Severity, Spec: spec}, nil
}

type ReviewV2ArchiveInput struct {
	RuleID           string `json:"ruleId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type ReviewV2AcknowledgeInput struct {
	EpisodeID        string `json:"episodeId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type ReviewV2ClearSuppressionInput struct {
	SuppressionID    string `json:"suppressionId"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

// New v2 tools never call the legacy report or policy writer. Read tools
// return only the last complete publication and leave the database untouched.
func (s *Service) reviewV2Tools(server *mcp.Server, mode string) {
	readTool(server, "list_investment_review_rules", "List daily investment review rules with independent predicate and triggerPolicy. Read-only, 1–100 at a time; use the last rule ID as cursor when a full page is returned.", func(ctx context.Context, in ReviewV2PageInput) (any, error) {
		return s.app.ListReviewRulesV2(ctx, in.Cursor, in.PageSize)
	})
	readTool(server, "preview_investment_review_rule", "Preview a v2 rule on closed historical civil days for one instrument, without saving it or writing projections. Unknown days do not count as hits. sourceToken can be supplied to reject changed input.", func(ctx context.Context, in ReviewV2PreviewInput) (any, error) {
		spec, err := reviewV2Spec(in.Spec)
		if err != nil {
			return nil, err
		}
		return s.app.PreviewReviewRuleV2(ctx, spec, in.InstrumentID, in.LatestClosedOn, in.ExpectedSourceToken)
	})
	readTool(server, "list_investment_review_alerts", "Read a published rule-aware investment review report v2. Groups are paged by instrument, never split by rule; cursor is bound to report revision and filters. This read does not reconcile or update data.", func(ctx context.Context, in ReviewV2AlertPageInput) (any, error) {
		return s.app.ListReviewAlertGroupsV2(ctx, in.Filter, in.Cursor, in.PageSize)
	})
	readTool(server, "get_investment_review_alert", "Read the currently open v2 episode by episodeId with its instrument's other active rule alerts. A resolved episode is not a current alert. This is a pure bounded read.", func(ctx context.Context, in ReviewV2AlertInput) (any, error) {
		return s.app.GetReviewAlertV2(ctx, in.EpisodeID)
	})
	readTool(server, "get_investment_review_coverage", "Page every current positive-holding investment's published daily rule outcomes, including unknown and uncomputed states with no open alert. No evaluation or write occurs. Filter by rule status; a group may contain several states.", func(ctx context.Context, in ReviewV2CoverageInput) (any, error) {
		return s.app.ListReviewCoverageV2(ctx, in.Filter, in.Cursor, in.PageSize)
	})
	readTool(server, "list_investment_review_suppressions", "List ignored investment/rule scopes with absolute resume dates. This is a pure read; suppressions are separate from acknowledgements.", func(ctx context.Context, in ReviewV2PageInput) (any, error) {
		return s.app.ListReviewSuppressionsV2(ctx, in.Cursor, in.PageSize)
	})
	if mode != LedgerWrite {
		return
	}
	ledgerWriteTool(s, server, "create_investment_review_rule", "Create one named v2 daily rule. Its condition checks each closed day; choose single, consecutive, or rolling triggerPolicy. Supply a stable mutationId inside input as well as the outer operationId.", func(ctx context.Context, in ReviewV2RuleInput) (any, error) {
		input, err := reviewV2RuleInput(in)
		if err != nil {
			return nil, err
		}
		return s.app.CreateReviewRuleV2(ctx, input)
	})
	ledgerWriteTool(s, server, "update_investment_review_rule", "Update a v2 rule with expected row revision; a semantic change starts a new condition version. Supply a stable inner mutationId on retries.", func(ctx context.Context, in ReviewV2RuleInput) (any, error) {
		input, err := reviewV2RuleInput(in)
		if err != nil {
			return nil, err
		}
		return s.app.UpdateReviewRuleV2(ctx, input)
	})
	ledgerWriteTool(s, server, "archive_investment_review_rule", "Archive a v2 rule without deleting evidence or other rules.", func(ctx context.Context, in ReviewV2ArchiveInput) (any, error) {
		return s.app.ArchiveReviewRuleV2(ctx, in.RuleID, in.MutationID, in.ExpectedRevision)
	})
	ledgerWriteTool(s, server, "acknowledge_investment_review_alert", "Acknowledge exactly one v2 episode at its expected revision. A new episode can still alert after the condition clears and returns.", func(ctx context.Context, in ReviewV2AcknowledgeInput) (any, error) {
		return s.app.AcknowledgeReviewEpisodeV2(ctx, in.EpisodeID, in.MutationID, in.ExpectedRevision)
	})
	ledgerWriteTool(s, server, "set_investment_review_suppression", "Ignore one investment for all current and future rules or one specified rule. Choose permanent, 7/30 days, or an absolute resume date. This does not acknowledge an episode.", func(ctx context.Context, in application.SetReviewSuppressionV2Input) (any, error) {
		return s.app.SetReviewSuppressionV2(ctx, in)
	})
	ledgerWriteTool(s, server, "clear_investment_review_suppression", "Resume alerts for the exact suppression record at its current revision.", func(ctx context.Context, in ReviewV2ClearSuppressionInput) (any, error) {
		return map[string]bool{"cleared": true}, s.app.ClearReviewSuppressionV2(ctx, in.SuppressionID, in.MutationID, in.ExpectedRevision)
	})
	ledgerWriteTool(s, server, "refresh_investment_reviews", "Explicitly reconcile daily investment review rules against the current local financial input. It publishes a complete revision or returns an error; it never fetches market data.", func(ctx context.Context, _ Empty) (any, error) {
		return s.app.ReconcileInvestmentReviewsV2(ctx)
	})
}
