package metrics

func RecordS2SAuth(source, destination string) {
	ensure()
	s2sAuthRequests.WithLabelValues(SanitizeService(source), SanitizeService(destination)).Inc()
}

func RecordS2SAuthFailure(source, destination, reason string) {
	ensure()
	src := SanitizeService(source)
	dst := SanitizeService(destination)
	r := SanitizeReason(reason)
	s2sAuthRequests.WithLabelValues(src, dst).Inc()
	s2sAuthFailures.WithLabelValues(src, dst, r).Inc()
}

func RecordTokenRequest(source, destination string) {
	ensure()
	s2sTokenRequests.WithLabelValues(SanitizeService(source), SanitizeService(destination)).Inc()
}

func RecordTokenRequestFailure(source, destination, reason string) {
	ensure()
	src := SanitizeService(source)
	dst := SanitizeService(destination)
	s2sTokenRequests.WithLabelValues(src, dst).Inc()
	s2sTokenFailures.WithLabelValues(src, dst, SanitizeReason(reason)).Inc()
}

func RecordTokenCacheHit(source, destination string) {
	ensure()
	s2sTokenCacheHit.WithLabelValues(SanitizeService(source), SanitizeService(destination)).Inc()
}
