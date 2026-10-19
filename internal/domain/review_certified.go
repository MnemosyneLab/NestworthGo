package domain

const ReviewCertifiedCompletenessVersion = "certified-full-review-v1"

type ReviewConnectionCheckpoint struct {
	Connection  uint64
	DataVersion int64
	OwnChanges  int64
}

// Provider revisions are process-local and include same-key/ABA assignments.
// Pinned selections use an explicit service provider, not a mutable default.
type ReviewProviderSelection struct {
	Key      string
	Revision uint64
	Pinned   bool
}

// Issued only by a coherent, fully loaded source capture. SourceMAC binds
// these trust decisions to that database handle's private session key.
type ReviewCertifiedCapture struct {
	SourceMAC         string
	Connection        ReviewConnectionCheckpoint
	ProjectionSeal    string
	TemporalJSON      string
	ReviewFingerprint string
	SchemaCookie      int
	MayCertify        bool
	MayReuse          bool
	PreviousProvider  ReviewProviderSelection
	Provider          ReviewProviderSelection // Supplied by the application provider lease.
}

type ReviewCertifiedFullOracle struct{ Rules []ReviewRulePublication }

func ReviewFullPublication(p ReviewPublication) ReviewPublication {
	if p.CertifiedFullOracle != nil {
		p.Rules = p.CertifiedFullOracle.Rules
	}
	p.CertifiedFullOracle = nil
	p.Capture.TemporalFullRecompute = true
	p.Capture.ReusableDailyEvidence = nil
	return p
}

// Reconstruct the independent full oracle and compare the actual bounded
// output again. No successful/forged preparation stamp grants authority.
func ValidateReviewCertifiedOutput(p ReviewPublication) string {
	if p.CertifiedFullOracle == nil {
		return "certified_full_oracle_missing"
	}
	full := ReviewFullPublication(p)
	full.WindowOracle = &ReviewWindowOracle{Version: ReviewWindowOracleVersion, Rules: p.Rules}
	return ValidateReviewWindowOracle(full)
}
