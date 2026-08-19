package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	registerOnce sync.Once

	clientRequests *prometheus.CounterVec
	clientDuration *prometheus.HistogramVec

	s2sAuthRequests  *prometheus.CounterVec
	s2sAuthFailures  *prometheus.CounterVec
	s2sTokenRequests *prometheus.CounterVec
	s2sTokenFailures *prometheus.CounterVec
	s2sTokenCacheHit *prometheus.CounterVec

	msgPublishTotal    *prometheus.CounterVec
	msgPublishFails    *prometheus.CounterVec
	msgPublishDur      *prometheus.HistogramVec
	msgConsumeTotal    *prometheus.CounterVec
	msgConsumeFails    *prometheus.CounterVec
	msgProcessDur      *prometheus.HistogramVec
	msgRetryTotal      *prometheus.CounterVec
	msgDLQTotal        *prometheus.CounterVec
	msgOldestAge       *prometheus.GaugeVec
	msgUnroutableTotal *prometheus.CounterVec
	msgConfirmTimeout  *prometheus.CounterVec

	outboxPending   *prometheus.GaugeVec
	outboxFailed    *prometheus.GaugeVec
	outboxExhausted *prometheus.GaugeVec
	outboxOldest    *prometheus.GaugeVec
	outboxDispatch  *prometheus.CounterVec
	outboxDispFail  *prometheus.CounterVec
	outboxDispDur   *prometheus.HistogramVec

	consumerDuplicate *prometheus.CounterVec
	reconMismatch     *prometheus.CounterVec
	reconRepaired     *prometheus.CounterVec
	reconScanned      *prometheus.CounterVec
	reconFailed       *prometheus.CounterVec
	ambiguousOps      *prometheus.CounterVec
	circuitBreaker    *prometheus.GaugeVec
	circuitBreakerTx  *prometheus.CounterVec
	clientFailures    *prometheus.CounterVec
	clientRetries     *prometheus.CounterVec
	clientTimeouts    *prometheus.CounterVec
	clientGiveUp      *prometheus.CounterVec

	businessTotal *prometheus.CounterVec
	businessFails *prometheus.CounterVec
)

func rpcBuckets() []float64 {
	return []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
}

func ensure() {
	registerOnce.Do(func() {
		clientRequests = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ClientRequestsTotal,
			Help: "Outbound service-to-service requests by source, destination, protocol, operation, status and error type.",
		}, []string{LabelSource, LabelDestination, LabelProtocol, LabelOperation, LabelStatus, LabelErrorType})

		clientDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    ClientRequestDuration,
			Help:    "Outbound service-to-service request duration in seconds.",
			Buckets: rpcBuckets(),
		}, []string{LabelSource, LabelDestination, LabelProtocol, LabelOperation})

		s2sAuthRequests = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: S2SAuthRequestsTotal,
			Help: "Inbound S2S authentication attempts.",
		}, []string{LabelSource, LabelDestination})

		s2sAuthFailures = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: S2SAuthFailuresTotal,
			Help: "Inbound S2S authentication failures by bounded reason.",
		}, []string{LabelSource, LabelDestination, LabelReason})

		s2sTokenRequests = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: S2STokenRequestsTotal,
			Help: "Outbound service-token fetch attempts (cache misses).",
		}, []string{LabelSource, LabelDestination})

		s2sTokenFailures = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: S2STokenRequestFails,
			Help: "Outbound service-token fetch failures.",
		}, []string{LabelSource, LabelDestination, LabelReason})

		s2sTokenCacheHit = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: S2STokenCacheHitsTotal,
			Help: "Outbound service-token cache hits.",
		}, []string{LabelSource, LabelDestination})

		msgPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingPublishTotal,
			Help: "RabbitMQ publish attempts.",
		}, []string{LabelService, LabelEventType, LabelExchange, LabelResult})

		msgPublishFails = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingPublishFails,
			Help: "RabbitMQ publish failures.",
		}, []string{LabelService, LabelEventType, LabelExchange})

		msgPublishDur = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    MessagingPublishDur,
			Help:    "RabbitMQ publish duration in seconds.",
			Buckets: rpcBuckets(),
		}, []string{LabelService, LabelEventType, LabelExchange})

		msgConsumeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingConsumeTotal,
			Help: "RabbitMQ consume outcomes.",
		}, []string{LabelService, LabelEventType, LabelResult})

		msgConsumeFails = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingConsumeFails,
			Help: "RabbitMQ consume failures.",
		}, []string{LabelService, LabelEventType, LabelResult})

		msgProcessDur = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    MessagingProcessDur,
			Help:    "RabbitMQ message processing duration in seconds.",
			Buckets: rpcBuckets(),
		}, []string{LabelService, LabelEventType})

		msgRetryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingRetryTotal,
			Help: "RabbitMQ consumer retries.",
		}, []string{LabelService, LabelEventType})

		msgDLQTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingDLQTotal,
			Help: "RabbitMQ dead-letter publishes.",
		}, []string{LabelService, LabelEventType})

		msgOldestAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: MessagingOldestAge,
			Help: "Age in seconds of the oldest observed in-flight message (application view).",
		}, []string{LabelService, LabelEventType})

		msgUnroutableTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingUnroutableTotal,
			Help: "RabbitMQ publishes returned as unroutable (mandatory, no matching queue).",
		}, []string{LabelService, LabelEventType, LabelExchange})

		msgConfirmTimeout = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: MessagingConfirmTimeoutTotal,
			Help: "RabbitMQ publisher confirm timeouts.",
		}, []string{LabelService, LabelEventType, LabelExchange})

		outboxPending = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: OutboxPendingTotal,
			Help: "Unpublished outbox rows.",
		}, []string{LabelService, LabelDestination, LabelEventType})

		outboxFailed = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: OutboxFailedTotal,
			Help: "Failed outbox rows waiting for retry.",
		}, []string{LabelService, LabelDestination, LabelEventType})

		outboxExhausted = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: OutboxExhaustedTotal,
			Help: "Outbox rows that exhausted retries and require operator replay.",
		}, []string{LabelService, LabelDestination, LabelEventType})

		outboxOldest = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: OutboxOldestPending,
			Help: "Age in seconds of the oldest unpublished outbox row.",
		}, []string{LabelService, LabelDestination, LabelEventType})

		outboxDispatch = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: OutboxDispatchTotal,
			Help: "Outbox dispatcher outcomes (claimed, dispatched, failed, retried, abandoned).",
		}, []string{LabelService, LabelDestination, LabelEventType, LabelResult})

		outboxDispFail = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: OutboxDispatchFails,
			Help: "Outbox dispatch failures.",
		}, []string{LabelService, LabelDestination, LabelEventType})

		outboxDispDur = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    OutboxDispatchDur,
			Help:    "Outbox single-event dispatch duration in seconds.",
			Buckets: rpcBuckets(),
		}, []string{LabelService, LabelDestination, LabelEventType})

		consumerDuplicate = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ConsumerDuplicateTotal,
			Help: "Consumer deliveries skipped because the event was already processed.",
		}, []string{LabelService, LabelEventType})

		reconMismatch = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ReconciliationMismatchTotal,
			Help: "Reconciliation mismatches detected (business state without expected delivery).",
		}, []string{LabelService, LabelOperation})

		reconRepaired = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ReconciliationRepairedTotal,
			Help: "Reconciliation repairs that requeued missing delivery work.",
		}, []string{LabelService, LabelOperation})

		reconScanned = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ReconciliationScannedTotal,
			Help: "Items examined by a reconciliation pass.",
		}, []string{LabelService, LabelOperation})

		reconFailed = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ReconciliationFailedTotal,
			Help: "Reconciliation pass items that could not be resolved and require operator attention.",
		}, []string{LabelService, LabelOperation})

		ambiguousOps = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: AmbiguousOperationsTotal,
			Help: "Financial operations whose downstream outcome was unknown (e.g. timeout) and were kept pending instead of guessed.",
		}, []string{LabelService, LabelOperation})

		circuitBreaker = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: CircuitBreakerState,
			Help: "Circuit breaker state: 0=closed, 1=half_open, 2=open.",
		}, []string{LabelService, LabelDestination})

		circuitBreakerTx = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: CircuitBreakerTransitions,
			Help: "Circuit breaker state transitions by resulting state (open, half_open, closed).",
		}, []string{LabelService, LabelDestination, LabelResult})

		clientFailures = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ClientFailuresTotal,
			Help: "Outbound service-to-service failures (timeouts, 5xx, unavailable).",
		}, []string{LabelSource, LabelDestination, LabelProtocol, LabelErrorType})

		clientRetries = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ClientRetriesTotal,
			Help: "Outbound service-to-service request retries actually performed.",
		}, []string{LabelSource, LabelDestination, LabelProtocol})

		clientTimeouts = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ClientTimeoutsTotal,
			Help: "Outbound service-to-service request attempts that timed out.",
		}, []string{LabelSource, LabelDestination, LabelProtocol})

		clientGiveUp = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: ClientGiveUpTotal,
			Help: "Outbound service-to-service calls that failed after exhausting the retry policy or were rejected by the breaker.",
		}, []string{LabelSource, LabelDestination, LabelProtocol, LabelErrorType})

		businessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: BusinessOperationTotal,
			Help: "Selected business-flow operations. result is success, business_rejected, or system_failure.",
		}, []string{LabelService, LabelOperation, LabelResult})

		businessFails = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: BusinessOperationFails,
			Help: "Selected business-flow system/integration failures (excludes business_rejected).",
		}, []string{LabelService, LabelOperation})
	})
}
