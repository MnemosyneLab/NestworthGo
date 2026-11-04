// Package assistant owns the installed assistant's capability gate. The provider
// runner and financial writer are deliberately absent until their activation
// gates are implemented. No credential value or configuration is accepted here.
package assistant

type Status struct {
	Version         int    `json:"version"`
	Surface         string `json:"surface"`
	PageGuide       bool   `json:"pageGuide"`
	Provider        string `json:"provider"`
	Credentials     string `json:"credentials"`
	Conversations   string `json:"conversations"`
	ModelRuns       bool   `json:"modelRuns"`
	FinancialWrites bool   `json:"financialWrites"`
	Reason          string `json:"reason"`
}

// Closed is shared by the trusted desktop IPC and authenticated mobile route.
// Mobile read permission never grants the household-wide in-process tools.
func Closed(surface string) Status {
	return Status{Version: 1, Surface: surface, PageGuide: true,
		Provider: "unavailable", Credentials: "unavailable", Conversations: "memory",
		Reason: "activation_incomplete"}
}
