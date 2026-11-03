package domain

// archivedRawCloseBasis preserves the units of immutable observations imported
// by older releases. It grants no provider routing or network capability.
func archivedRawCloseBasis(basis string) bool {
	return basis == "tiingo_raw_close_v1"
}

// ArchivedInstrumentQuality recognizes only the original, fully qualified
// session-close facts. Their source identity must never be relabeled.
func ArchivedInstrumentQuality(source, basis, policy, timestamp string) (bool, bool) {
	if source != "tiingo" {
		return false, false
	}
	return true, archivedRawCloseBasis(basis) && policy == basis && timestamp == "session_close"
}
