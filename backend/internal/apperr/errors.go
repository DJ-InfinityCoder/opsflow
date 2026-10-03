package apperr

type ErrorKind string

const (
	KindNotFound             ErrorKind = "not_found"
	KindForbidden            ErrorKind = "forbidden"
	KindUnauthorized         ErrorKind = "unauthorized"
	KindConflict             ErrorKind = "conflict"
	KindValidation           ErrorKind = "validation_error"
	KindUnavailable          ErrorKind = "service_unavailable"
	KindInternal             ErrorKind = "internal_error"
	KindRateLimited          ErrorKind = "rate_limited"
	KindPreconditionRequired ErrorKind = "precondition_required"
	KindVersionConflict      ErrorKind = "version_conflict"
	KindIllegalTransition    ErrorKind = "illegal_transition"
	KindIdempotencyKeyReused ErrorKind = "idempotency_key_reused"
	KindIdempotencyKeyReuse  ErrorKind = "idempotency_key_reuse"
	KindRequestInProgress    ErrorKind = "request_in_progress"
	KindAlreadyClaimed       ErrorKind = "already_claimed"
)

type AppError struct {
	Kind    ErrorKind
	Message string
	Details any
	Cause   error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Cause }

func (e *AppError) Is(target error) bool {
	other, ok := target.(*AppError)
	return ok && e.Kind == other.Kind
}

func New(kind ErrorKind, message string, details any) *AppError {
	return &AppError{Kind: kind, Message: message, Details: details}
}

var (
	ErrNotFound             = &AppError{Kind: KindNotFound, Message: "resource not found"}
	ErrForbidden            = &AppError{Kind: KindForbidden, Message: "forbidden"}
	ErrUnauthorized         = &AppError{Kind: KindUnauthorized, Message: "unauthorized"}
	ErrConflict             = &AppError{Kind: KindConflict, Message: "conflict"}
	ErrValidation           = &AppError{Kind: KindValidation, Message: "validation failed"}
	ErrUnavailable          = &AppError{Kind: KindUnavailable, Message: "service unavailable"}
	ErrInternal             = &AppError{Kind: KindInternal, Message: "internal server error"}
	ErrRateLimited          = &AppError{Kind: KindRateLimited, Message: "too many requests"}
	ErrPreconditionRequired = &AppError{Kind: KindPreconditionRequired, Message: "If-Match is required"}
	ErrVersionConflict      = &AppError{Kind: KindVersionConflict, Message: "work item version conflict"}
	ErrIllegalTransition    = &AppError{Kind: KindIllegalTransition, Message: "illegal state transition"}
	ErrIdempotencyKeyReused = &AppError{Kind: KindIdempotencyKeyReused, Message: "idempotency key was already used with a different request body"}
	ErrIdempotencyKeyReuse  = &AppError{Kind: KindIdempotencyKeyReuse, Message: "idempotency key was already used with a different request body"}
	ErrRequestInProgress    = &AppError{Kind: KindRequestInProgress, Message: "request with this idempotency key is currently in progress"}
	ErrAlreadyClaimed       = &AppError{Kind: KindAlreadyClaimed, Message: "work item is already claimed"}
)
