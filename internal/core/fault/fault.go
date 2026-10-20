// Package fault defines stable business error categories shared by consumers.
package fault

type Error string

func (e Error) Error() string { return string(e) }

const (
	Invalid         Error = "validation_failed"
	BadRequest      Error = "invalid_request"
	Unauthenticated Error = "unauthenticated"
	Forbidden       Error = "forbidden"
	NotFound        Error = "not_found"
	Stale           Error = "stale_revision"
	State           Error = "invalid_state"
	Limited         Error = "rate_limited"
	Idempotency     Error = "idempotency_conflict"
)
