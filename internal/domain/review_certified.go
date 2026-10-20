package domain

const ReviewCertifiedCompletenessVersion = "certified-full-review-v3"

type ReviewConnectionCheckpoint struct {
	Connection  uint64
	DataVersion int64
	OwnChanges  int64
	OwnCommits  uint64 // Driver commit hook includes same-connection DDL.
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
type ReviewSuffixSourceProof struct {
	NonQuoteFingerprint string
	QuoteSeals          []ReviewQuoteDaySeal
}

type ReviewPrefixMemoObservation struct {
	ReusedDays, SQLDecodedDays, ArgumentEncodes int
}

type ReviewCertifiedCapture struct {
	FXDescription         *ReviewFXDescription         `json:",omitempty"` // Complete coherent-capture description, bound by SourceMAC.
	SuffixMemoReceipt     uint64                       `json:",omitempty"`
	SuffixMemoObservation *ReviewPrefixMemoObservation `json:",omitempty"`

	SuffixSourceProof  *ReviewSuffixSourceProof `json:",omitempty"` // Derived in the coherent capture; authenticated by SourceMAC.
	SourceMAC          string
	SuffixDigest       string // Exact typed prefix/plan digest; bound by SourceMAC.
	Connection         ReviewConnectionCheckpoint
	ProjectionSeal     string
	TemporalJSON       string
	ReviewFingerprint  string
	SchemaCookie       int
	InitialPublication bool // Captured private genesis eligibility; publisher must claim it.
	MayBootstrap       bool // Disabled: current absence does not prove historical absence.
	MayCertify         bool
	MayReuse           bool
	PreviousProvider   ReviewProviderSelection
	Provider           ReviewProviderSelection // Supplied by the application provider lease.
}

type ReviewCertifiedFullOracle struct{ Rules []ReviewRulePublication }

func ReviewFullPublication(p ReviewPublication) ReviewPublication {
	if p.CertifiedFullOracle != nil {
		p.Rules = p.CertifiedFullOracle.Rules
	}
	p.CertifiedFullOracle = nil
	p.Capture.Suffix = nil
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
