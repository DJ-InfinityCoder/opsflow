package service

import "opsflow/backend/internal/apperr"

type ErrorKind = apperr.ErrorKind
type AppError = apperr.AppError

const (
	KindNotFound             = apperr.KindNotFound
	KindForbidden            = apperr.KindForbidden
	KindUnauthorized         = apperr.KindUnauthorized
	KindConflict             = apperr.KindConflict
	KindValidation           = apperr.KindValidation
	KindUnavailable          = apperr.KindUnavailable
	KindInternal             = apperr.KindInternal
	KindRateLimited          = apperr.KindRateLimited
	KindPreconditionRequired = apperr.KindPreconditionRequired
	KindVersionConflict      = apperr.KindVersionConflict
	KindIllegalTransition    = apperr.KindIllegalTransition
	KindIdempotencyKeyReused = apperr.KindIdempotencyKeyReused
	KindIdempotencyKeyReuse  = apperr.KindIdempotencyKeyReuse
	KindRequestInProgress    = apperr.KindRequestInProgress
	KindAlreadyClaimed       = apperr.KindAlreadyClaimed
)

var (
	ErrNotFound             = apperr.ErrNotFound
	ErrForbidden            = apperr.ErrForbidden
	ErrUnauthorized         = apperr.ErrUnauthorized
	ErrConflict             = apperr.ErrConflict
	ErrValidation           = apperr.ErrValidation
	ErrUnavailable          = apperr.ErrUnavailable
	ErrInternal             = apperr.ErrInternal
	ErrRateLimited          = apperr.ErrRateLimited
	ErrPreconditionRequired = apperr.ErrPreconditionRequired
	ErrVersionConflict      = apperr.ErrVersionConflict
	ErrIllegalTransition    = apperr.ErrIllegalTransition
	ErrIdempotencyKeyReused = apperr.ErrIdempotencyKeyReused
	ErrIdempotencyKeyReuse  = apperr.ErrIdempotencyKeyReuse
	ErrRequestInProgress    = apperr.ErrRequestInProgress
	ErrAlreadyClaimed       = apperr.ErrAlreadyClaimed
)

func NewAppError(kind ErrorKind, message string, details any) *AppError {
	return apperr.New(kind, message, details)
}
