package agentcapability

// WriteFailure proves an exact operation has a durable failed receipt. It is
// never inferred from a business error alone, cancellation or a missing receipt.
type WriteFailure struct {
	OperationID string
	Cause       error
}

func (e WriteFailure) Error() string { return "write has a durable failed receipt" }
func (e WriteFailure) Unwrap() error { return e.Cause }
