package domain

// ProductOperationHistoryRow adds read metadata without changing persisted facts.
type ProductOperationHistoryRow struct {
	ProductOperation
	ReversedByOperationID *ProductOperationID
}

// ProductUndoEvidence contains complete candidate-linked history, never a display cap.
type ProductUndoEvidence struct {
	Operation    ProductOperationEvidence
	Contracts    map[ProductContractID]ProductContract
	Histories    map[ProductContractID][]ProductOperation
	Activities   map[ActivityID]Activity
	Reservations []LiquidityReservation
}

type ProductOperationHistorySnapshot struct {
	Rows                      []ProductOperationHistoryRow
	LatestRecordedOperationID *ProductOperationID
	Candidate                 *ProductOperation
	Portfolio                 PortfolioSnapshot
	UndoEvidence              *ProductUndoEvidence
}
