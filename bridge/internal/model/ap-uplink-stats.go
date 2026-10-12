package model

// APUplinkStats contains the packet counters reported by an AP's wired uplink.
// These are cumulative counts, not rates; the controller/device can reset them.
// Missing or null fields remain nil, while an explicit zero is a valid counter.
// uint64 preserves the integer payload without losing precision during decoding
// and rejects negative, fractional, and overflowing counters.
type APUplinkStats struct {
	UpPackets        *uint64 `json:"upPackets"`
	DownPackets      *uint64 `json:"downPackets"`
	UpDropPackets    *uint64 `json:"upDropPackets"`
	DownDropPackets  *uint64 `json:"downDropPackets"`
	UpErrorPackets   *uint64 `json:"upErrorsPackets"`
	DownErrorPackets *uint64 `json:"downErrorsPackets"`
}

// HasCounters reports whether at least one packet counter is available.
func (s *APUplinkStats) HasCounters() bool {
	return s != nil && (s.UpPackets != nil || s.DownPackets != nil ||
		s.UpDropPackets != nil || s.DownDropPackets != nil ||
		s.UpErrorPackets != nil || s.DownErrorPackets != nil)
}
