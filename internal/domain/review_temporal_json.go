package domain

import "encoding/json"

// Optional C3 authority is fail-closed metadata. Damaged/future authority
// fields must not make an otherwise readable C2 checkpoint unusable. Core
// temporal/source decoding errors retain the original integrity failure.
func (c *ReviewTemporalCertificate) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, name := range []string{"CertifiedProjectionSeal", "CertifiedMAC", "CertifiedProvider", "CertifiedConnection", "CertifiedNeedsReconcile"} {
		delete(fields, name)
	}
	base, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain ReviewTemporalCertificate
	var decoded plain
	if err := json.Unmarshal(base, &decoded); err != nil {
		return err
	}
	var authority struct {
		CertifiedProjectionSeal string
		CertifiedMAC            string
		CertifiedProvider       ReviewProviderSelection
		CertifiedConnection     ReviewConnectionCheckpoint
		CertifiedNeedsReconcile bool
	}
	if json.Unmarshal(raw, &authority) == nil {
		decoded.CertifiedProjectionSeal = authority.CertifiedProjectionSeal
		decoded.CertifiedMAC = authority.CertifiedMAC
		decoded.CertifiedProvider = authority.CertifiedProvider
		decoded.CertifiedConnection = authority.CertifiedConnection
		decoded.CertifiedNeedsReconcile = authority.CertifiedNeedsReconcile
	}
	*c = ReviewTemporalCertificate(decoded)
	return nil
}
