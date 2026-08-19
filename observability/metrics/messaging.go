package metrics

import "time"

func RecordPublish(service, eventType, exchange, result string, started time.Time) {
	ensure()
	svc := SanitizeService(service)
	ev := SanitizeEventType(eventType)
	ex := SanitizeExchange(exchange)
	res := SanitizeResult(result)
	msgPublishTotal.WithLabelValues(svc, ev, ex, res).Inc()
	if res != ResultSuccess {
		msgPublishFails.WithLabelValues(svc, ev, ex).Inc()
	}
	if !started.IsZero() {
		msgPublishDur.WithLabelValues(svc, ev, ex).Observe(time.Since(started).Seconds())
	}
}

func RecordConsume(service, eventType, result string, started time.Time) {
	ensure()
	svc := SanitizeService(service)
	ev := SanitizeEventType(eventType)
	res := SanitizeResult(result)
	msgConsumeTotal.WithLabelValues(svc, ev, res).Inc()
	if res != ResultSuccess {
		msgConsumeFails.WithLabelValues(svc, ev, res).Inc()
	}
	if !started.IsZero() {
		msgProcessDur.WithLabelValues(svc, ev).Observe(time.Since(started).Seconds())
	}
}

func RecordRetry(service, eventType string) {
	ensure()
	msgRetryTotal.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType)).Inc()
}

func RecordDLQ(service, eventType string) {
	ensure()
	msgDLQTotal.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType)).Inc()
}

func RecordUnroutable(service, eventType, exchange string) {
	ensure()
	msgUnroutableTotal.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType), SanitizeExchange(exchange)).Inc()
}

func RecordConfirmTimeout(service, eventType, exchange string) {
	ensure()
	msgConfirmTimeout.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType), SanitizeExchange(exchange)).Inc()
}

func RecordDuplicate(service, eventType string) {
	ensure()
	consumerDuplicate.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType)).Inc()
}

func SetOldestMessageAge(service, eventType string, age time.Duration) {
	ensure()
	if age < 0 {
		age = 0
	}
	msgOldestAge.WithLabelValues(SanitizeService(service), SanitizeEventType(eventType)).Set(age.Seconds())
}

func ObserveMessageTimestamp(service, eventType string, ts time.Time) {
	if ts.IsZero() {
		return
	}
	SetOldestMessageAge(service, eventType, time.Since(ts))
}
