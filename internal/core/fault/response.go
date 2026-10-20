package fault

import "errors"

type Detail struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	RequestID string      `json:"request_id"`
	Conflicts *[]Conflict `json:"conflicts,omitempty"`
}
type Envelope struct {
	Error Detail `json:"error"`
}

// Response never includes the text of an infrastructure error or request data.
func Response(err error, requestID string) Envelope {
	code := "internal_error"
	var business Error
	if errors.As(err, &business) {
		code = string(business)
	}
	out := Envelope{Error: Detail{Code: code, Message: code, RequestID: requestID}}
	var conflicts *Conflicts
	if errors.As(err, &conflicts) {
		out.Error.Conflicts = &conflicts.Fields
	}
	return out
}
