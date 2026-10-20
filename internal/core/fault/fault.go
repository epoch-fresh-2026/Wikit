// Package fault defines stable business error categories shared by consumers.
package fault

type Error string

func (e Error) Error() string { return string(e) }

const (
	Invalid            Error = "validation_failed"
	BadRequest         Error = "invalid_request"
	Unauthenticated    Error = "unauthenticated"
	Forbidden          Error = "forbidden"
	NotFound           Error = "not_found"
	Stale              Error = "stale_revision"
	State              Error = "invalid_state"
	Limited            Error = "rate_limited"
	Idempotency        Error = "idempotency_conflict"
	ReviewRequired     Error = "review_required"
	MergeConflict      Error = "merge_conflict"
	UploadTooLarge     Error = "upload_too_large"
	UnsupportedMedia   Error = "unsupported_media"
	StorageUnavailable Error = "storage_unavailable"
)

type Conflict struct {
	Path   string  `json:"path"`
	Base   *string `json:"base"`
	Ours   *string `json:"ours"`
	Theirs *string `json:"theirs"`
}
type Conflicts struct{ Fields []Conflict }

func (e *Conflicts) Error() string { return string(MergeConflict) }
func (e *Conflicts) Unwrap() error { return MergeConflict }
