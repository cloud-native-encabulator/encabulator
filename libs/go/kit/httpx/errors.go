package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrorEnvelope is the wire shape of every error response:
// {"error": {"code": "snake_case_code", "message": "Human readable"}}.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the error detail.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes used by more than one service.
const (
	CodeBadRequest     = "bad_request"
	CodeInvalidJSON    = "invalid_json"
	CodeUnauthorized   = "unauthorized"
	CodeForbidden      = "forbidden"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeRateLimited    = "rate_limited"
	CodeInternal       = "internal_error"
	CodeUnavailable    = "service_unavailable"
	CodeMalaise        = "malaise_injected"
	CodeCongested      = "congested"
	CodeUpstreamFailed = "upstream_failed"
)

// StatusError is an error that carries the HTTP status and code to report.
type StatusError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *StatusError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *StatusError) Unwrap() error { return e.Err }

// Errorf builds a StatusError.
func Errorf(status int, code, format string, args ...any) *StatusError {
	return &StatusError{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	// A write failure means the client is gone; there is nothing useful to do.
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the standard error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorEnvelope{Error: ErrorBody{Code: code, Message: message}})
}

// WriteErr writes err as an error response. A *StatusError reports its own
// status and code; anything else becomes a 500 internal_error, and the
// underlying message is not leaked to the client.
func WriteErr(w http.ResponseWriter, err error) {
	var se *StatusError
	if errors.As(err, &se) {
		WriteError(w, se.Status, se.Code, se.Message)
		return
	}
	WriteError(w, http.StatusInternalServerError, CodeInternal, "Internal error")
}

// DecodeJSON reads a JSON request body into v, rejecting unknown fields and
// bodies larger than limit bytes (0 means 1 MiB).
func DecodeJSON(r *http.Request, v any, limit int64) error {
	if limit <= 0 {
		limit = 1 << 20
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Errorf(http.StatusBadRequest, CodeInvalidJSON, "Request body is not valid JSON: %v", err)
	}
	return nil
}
