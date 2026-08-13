// Package metrics is the shared telemetry contract for broker microservices.
//
// Services must not invent per-service names for HTTP/gRPC dependencies, S2S
// auth, RabbitMQ, or outbox. Domain-specific business metrics stay in owning
// services; use RecordBusiness only for a small set of bounded flow names.
package metrics

const (
	ClientRequestsTotal    = "service_client_requests_total"
	ClientRequestDuration  = "service_client_request_duration_seconds"
	S2SAuthRequestsTotal   = "s2s_auth_requests_total"
	S2SAuthFailuresTotal   = "s2s_auth_failures_total"
	S2STokenRequestsTotal  = "s2s_token_requests_total"
	S2STokenRequestFails   = "s2s_token_request_failures_total"
	S2STokenCacheHitsTotal = "s2s_token_cache_hits_total"
	MessagingPublishTotal  = "messaging_publish_total"
	MessagingPublishFails  = "messaging_publish_failures_total"
	MessagingPublishDur    = "messaging_publish_duration_seconds"
	MessagingConsumeTotal  = "messaging_consume_total"
	MessagingConsumeFails  = "messaging_consume_failures_total"
	MessagingProcessDur    = "messaging_processing_duration_seconds"
	MessagingRetryTotal    = "messaging_retry_total"
	MessagingDLQTotal      = "messaging_dlq_total"
	MessagingOldestAge     = "messaging_oldest_message_age_seconds"
	OutboxPendingTotal     = "outbox_pending_total"
	OutboxFailedTotal      = "outbox_failed_total"
	OutboxOldestPending    = "outbox_oldest_pending_seconds"
	OutboxDispatchTotal    = "outbox_dispatch_total"
	OutboxDispatchFails    = "outbox_dispatch_failures_total"
	OutboxDispatchDur      = "outbox_dispatch_duration_seconds"
	BusinessOperationTotal = "business_operation_total"
	BusinessOperationFails = "business_operation_failures_total"
)

const (
	LabelSource      = "source"
	LabelDestination = "destination"
	LabelProtocol    = "protocol"
	LabelOperation   = "operation"
	LabelStatus      = "status"
	LabelErrorType   = "error_type"
	LabelReason      = "reason"
	LabelService     = "service"
	LabelEventType   = "event_type"
	LabelExchange    = "exchange"
	LabelResult      = "result"
)

const (
	ProtocolHTTP = "http"
	ProtocolGRPC = "grpc"
)

const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// Bounded error_type values. Never put raw error text into this label.
const (
	ErrorNone            = "none"
	ErrorUnauthenticated = "unauthenticated"
	ErrorForbidden       = "forbidden"
	ErrorNotFound        = "not_found"
	ErrorInvalid         = "invalid"
	ErrorConflict        = "conflict"
	ErrorRateLimited     = "rate_limited"
	ErrorTimeout         = "timeout"
	ErrorCanceled        = "canceled"
	ErrorUnavailable     = "unavailable"
	ErrorServer          = "server_error"
	ErrorUnknown         = "unknown"
)

// Bounded S2S reason values.
const (
	ReasonNone             = "none"
	ReasonMissingToken     = "missing_token"
	ReasonExpired          = "expired"
	ReasonInvalidSignature = "invalid_signature"
	ReasonInvalidAudience  = "invalid_audience"
	ReasonInvalidIssuer    = "invalid_issuer"
	ReasonMissingScope     = "missing_scope"
	ReasonUnknownService   = "unknown_service"
	ReasonTokenFetchFailed = "token_fetch_failed"
)

const (
	ResultSuccess    = "success"
	ResultError      = "error"
	ResultRetry      = "retry"
	ResultDLQ        = "dlq"
	ResultAbandoned  = "abandoned"
	ResultTimeout    = "timeout"
	ResultCanceled   = "canceled"
	ResultClaimed    = "claimed"
	ResultDispatched = "dispatched"
	ResultFailed     = "failed"
	ResultRetried    = "retried"
)

const (
	BusinessSuccess       = "success"
	BusinessRejected      = "business_rejected"
	BusinessSystemFailure = "system_failure"
)

// Allowed business operations. Keep this list small and bounded.
const (
	OpWalletCredit       = "wallet_credit"
	OpWalletDebit        = "wallet_debit"
	OpDepositComplete    = "deposit_complete"
	OpWithdrawalComplete = "withdrawal_complete"
	OpTradeOpened        = "trade_opened"
	OpTradeClosed        = "trade_closed"
	OpKYCApproved        = "kyc_approved"
	OpKYCRejected        = "kyc_rejected"
	OpTicketCreated      = "ticket_created"
)
