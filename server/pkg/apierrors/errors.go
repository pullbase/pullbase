// Package apierrors provides custom error types for the Pullbase API.
// These error types carry HTTP status codes and support error wrapping
// for consistent error handling across all API endpoints.
package apierrors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Kind represents the category of an API error.
type Kind int

const (
	KindUnknown Kind = iota
	KindNotFound
	KindUnauthorized
	KindForbidden
	KindValidation
	KindConflict
	KindInternal
	KindBadRequest
	KindTooManyRequests
	KindGone
	KindNotImplemented
)

// String returns the string representation of the error kind.
func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not_found"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindValidation:
		return "validation_error"
	case KindConflict:
		return "conflict"
	case KindInternal:
		return "internal_error"
	case KindBadRequest:
		return "bad_request"
	case KindTooManyRequests:
		return "too_many_requests"
	case KindGone:
		return "gone"
	case KindNotImplemented:
		return "not_implemented"
	default:
		return "unknown"
	}
}

// HTTPStatus returns the HTTP status code for the error kind.
func (k Kind) HTTPStatus() int {
	switch k {
	case KindNotFound:
		return http.StatusNotFound
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindValidation, KindBadRequest:
		return http.StatusBadRequest
	case KindConflict:
		return http.StatusConflict
	case KindTooManyRequests:
		return http.StatusTooManyRequests
	case KindGone:
		return http.StatusGone
	case KindNotImplemented:
		return http.StatusNotImplemented
	case KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Error represents an API error with a kind, message, and optional cause.
type Error struct {
	Kind    Kind
	Message string
	Op      string
	Err     error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Op != "" {
		if e.Err != nil {
			return fmt.Sprintf("%s: %s: %v", e.Op, e.Message, e.Err)
		}
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) HTTPStatus() int {
	return e.Kind.HTTPStatus()
}

func New(kind Kind, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
	}
}

func Wrap(kind Kind, op string, err error, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Op:      op,
		Err:     err,
	}
}

func WrapWithMessage(kind Kind, err error, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Err:     err,
	}
}

func NotFound(resource string, identifier interface{}) *Error {
	return &Error{
		Kind:    KindNotFound,
		Message: fmt.Sprintf("%s '%v' not found", resource, identifier),
	}
}

func NotFoundf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindNotFound,
		Message: fmt.Sprintf(format, args...),
	}
}

func Unauthorized(message string) *Error {
	return &Error{
		Kind:    KindUnauthorized,
		Message: message,
	}
}

func Unauthorizedf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindUnauthorized,
		Message: fmt.Sprintf(format, args...),
	}
}

func Forbidden(message string) *Error {
	return &Error{
		Kind:    KindForbidden,
		Message: message,
	}
}

func Forbiddenf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindForbidden,
		Message: fmt.Sprintf(format, args...),
	}
}

func Validation(message string) *Error {
	return &Error{
		Kind:    KindValidation,
		Message: message,
	}
}

func Validationf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindValidation,
		Message: fmt.Sprintf(format, args...),
	}
}

func BadRequest(message string) *Error {
	return &Error{
		Kind:    KindBadRequest,
		Message: message,
	}
}

func BadRequestf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindBadRequest,
		Message: fmt.Sprintf(format, args...),
	}
}

func Conflict(message string) *Error {
	return &Error{
		Kind:    KindConflict,
		Message: message,
	}
}

func Conflictf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindConflict,
		Message: fmt.Sprintf(format, args...),
	}
}

func Internal(message string) *Error {
	return &Error{
		Kind:    KindInternal,
		Message: message,
	}
}

func Internalf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindInternal,
		Message: fmt.Sprintf(format, args...),
	}
}

func TooManyRequests(message string) *Error {
	return &Error{
		Kind:    KindTooManyRequests,
		Message: message,
	}
}

func Gone(message string) *Error {
	return &Error{
		Kind:    KindGone,
		Message: message,
	}
}

func NotImplemented(message string) *Error {
	return &Error{
		Kind:    KindNotImplemented,
		Message: message,
	}
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Kind      string `json:"kind,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func WriteError(w http.ResponseWriter, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		writeErrorResponse(w, apiErr.HTTPStatus(), apiErr.Message, apiErr.Kind.String())
		return
	}

	writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", KindInternal.String())
}

func WriteErrorWithStatus(w http.ResponseWriter, status int, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		writeErrorResponse(w, status, apiErr.Message, apiErr.Kind.String())
		return
	}

	writeErrorResponse(w, status, err.Error(), "")
}

func WriteHTTPError(w http.ResponseWriter, status int, message string) {
	kind := statusToKind(status)
	writeErrorResponse(w, status, message, kind.String())
}

func writeErrorResponse(w http.ResponseWriter, status int, message, kind string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := ErrorResponse{
		Error: message,
	}
	if kind != "" && kind != "unknown" {
		resp.Kind = kind
	}

	json.NewEncoder(w).Encode(resp)
}

func statusToKind(status int) Kind {
	switch status {
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusUnauthorized:
		return KindUnauthorized
	case http.StatusForbidden:
		return KindForbidden
	case http.StatusBadRequest:
		return KindBadRequest
	case http.StatusConflict:
		return KindConflict
	case http.StatusTooManyRequests:
		return KindTooManyRequests
	case http.StatusGone:
		return KindGone
	case http.StatusNotImplemented:
		return KindNotImplemented
	case http.StatusInternalServerError:
		return KindInternal
	default:
		return KindUnknown
	}
}

func Is(err error, kind Kind) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Kind == kind
	}
	return false
}

func GetKind(err error) Kind {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Kind
	}
	return KindUnknown
}

func GetHTTPStatus(err error) int {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus()
	}
	return http.StatusInternalServerError
}

func FromDatabaseError(err error, resourceType, identifier string) *Error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	if strings.Contains(errStr, "not found") || strings.Contains(errStr, "no rows") {
		return NotFound(resourceType, identifier)
	}

	if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "unique constraint") || strings.Contains(errStr, "duplicate") {
		return Conflictf("%s '%s' already exists", resourceType, identifier)
	}

	return &Error{
		Kind:    KindInternal,
		Message: fmt.Sprintf("Failed to access %s", resourceType),
		Err:     err,
	}
}
