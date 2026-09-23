package core

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidArgument         ErrorCode = "INVALID_ARGUMENT"
	CodeUnauthenticated         ErrorCode = "AUTHENTICATION_REQUIRED"
	CodeNotFound                ErrorCode = "NOT_FOUND"
	CodeIdempotencyMismatch     ErrorCode = "IDEMPOTENCY_PAYLOAD_MISMATCH"
	CodeCommandInProgress       ErrorCode = "COMMAND_IN_PROGRESS"
	CodeBranchConflict          ErrorCode = "BRANCH_VERSION_CONFLICT"
	CodeInsufficientFunds       ErrorCode = "INSUFFICIENT_FUNDS"
	CodeInsufficientStock       ErrorCode = "INSUFFICIENT_STOCK"
	CodeIntegerOverflow         ErrorCode = "INTEGER_OVERFLOW"
	CodeUnauthorized            ErrorCode = "PERMISSION_DENIED"
	CodeSnapshotMismatch        ErrorCode = "SNAPSHOT_HASH_MISMATCH"
	CodeProjectionDiverged      ErrorCode = "PROJECTION_DIVERGED"
	CodeOutboxDelivery          ErrorCode = "OUTBOX_DELIVERY_FAILED"
	CodeIssuanceLimit           ErrorCode = "ISSUANCE_LIMIT_EXCEEDED"
	CodeMaterializationConflict ErrorCode = "MATERIALIZATION_ID_CONFLICT"
	CodeConservationFailed      ErrorCode = "CONSERVATION_FAILED"
	CodeStorageFailure          ErrorCode = "STORAGE_FAILURE"
	CodeInjectedFailure         ErrorCode = "INJECTED_FAILURE"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code ErrorCode, message string) error {
	return &Error{Code: code, Message: message}
}

func WrapError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func HasCode(err error, code ErrorCode) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
