// Package apierrors provides custom error types for the Pullbase API.
// These error types carry HTTP status codes and support error wrapping
// for consistent error handling across all API endpoints.
package apierrors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Kind    Kind   // The category of error
	Message string // Human-readable error message
	Op      string // Operation that failed (e.g., "GetServer", "CreateUser")
	Err     error  // Underlying error, if any
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

// Unwrap returns the underlying error for use with errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.Err
}

// HTTPStatus returns the HTTP status code for this error.
func (e *Error) HTTPStatus() int {
	return e.Kind.HTTPStatus()
}

// New creates a new Error with the given kind and message.
func New(kind Kind, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
	}
}

// Wrap wraps an existing error with additional context.
func Wrap(kind Kind, op string, err error, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Op:      op,
		Err:     err,
	}
}

// WrapWithMessage wraps an error and uses the provided message.
func WrapWithMessage(kind Kind, err error, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Err:     err,
	}
}

// NotFound creates a not found error.
func NotFound(resource string, identifier interface{}) *Error {
	return &Error{
		Kind:    KindNotFound,
		Message: fmt.Sprintf("%s '%v' not found", resource, identifier),
	}
}

// NotFoundf creates a not found error with a formatted message.
func NotFoundf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindNotFound,
		Message: fmt.Sprintf(format, args...),
	}
}

// Unauthorized creates an unauthorized error.
func Unauthorized(message string) *Error {
	return &Error{
		Kind:    KindUnauthorized,
		Message: message,
	}
}

// Unauthorizedf creates an unauthorized error with a formatted message.
func Unauthorizedf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindUnauthorized,
		Message: fmt.Sprintf(format, args...),
	}
}

// Forbidden creates a forbidden error.
func Forbidden(message string) *Error {
	return &Error{
		Kind:    KindForbidden,
		Message: message,
	}
}

// Forbiddenf creates a forbidden error with a formatted message.
func Forbiddenf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindForbidden,
		Message: fmt.Sprintf(format, args...),
	}
}

// Validation creates a validation error.
func Validation(message string) *Error {
	return &Error{
		Kind:    KindValidation,
		Message: message,
	}
}

// Validationf creates a validation error with a formatted message.
func Validationf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindValidation,
		Message: fmt.Sprintf(format, args...),
	}
}

// BadRequest creates a bad request error.
func BadRequest(message string) *Error {
	return &Error{
		Kind:    KindBadRequest,
		Message: message,
	}
}

// BadRequestf creates a bad request error with a formatted message.
func BadRequestf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindBadRequest,
		Message: fmt.Sprintf(format, args...),
	}
}

// Conflict creates a conflict error.
func Conflict(message string) *Error {
	return &Error{
		Kind:    KindConflict,
		Message: message,
	}
}

// Conflictf creates a conflict error with a formatted message.
func Conflictf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindConflict,
		Message: fmt.Sprintf(format, args...),
	}
}

// Internal creates an internal server error.
func Internal(message string) *Error {
	return &Error{
		Kind:    KindInternal,
		Message: message,
	}
}

// Internalf creates an internal server error with a formatted message.
func Internalf(format string, args ...interface{}) *Error {
	return &Error{
		Kind:    KindInternal,
		Message: fmt.Sprintf(format, args...),
	}
}

// TooManyRequests creates a rate limit error.
func TooManyRequests(message string) *Error {
	return &Error{
		Kind:    KindTooManyRequests,
		Message: message,
	}
}

// NotImplemented creates a not implemented error.
func NotImplemented(message string) *Error {
	return &Error{
		Kind:    KindNotImplemented,
		Message: message,
	}
}

// ErrorResponse is the JSON structure returned for API errors.
type ErrorResponse struct {
	Error     string `json:"error"`
	Kind      string `json:"kind,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteError writes an error response to the HTTP response writer.
// It handles both *Error types and standard errors.
func WriteError(w http.ResponseWriter, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		writeErrorResponse(w, apiErr.HTTPStatus(), apiErr.Message, apiErr.Kind.String())
		return
	}

	// For unknown errors, return internal server error
	writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", KindInternal.String())
}

// WriteErrorWithStatus writes an error with a specific HTTP status code.
// Use this when you need to override the default status from the error kind.
func WriteErrorWithStatus(w http.ResponseWriter, status int, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		writeErrorResponse(w, status, apiErr.Message, apiErr.Kind.String())
		return
	}

	writeErrorResponse(w, status, err.Error(), "")
}

// WriteHTTPError writes an error with a specific status code and message.
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

// statusToKind maps HTTP status codes to error kinds.
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
	case http.StatusNotImplemented:
		return KindNotImplemented
	case http.StatusInternalServerError:
		return KindInternal
	default:
		return KindUnknown
	}
}

// Is reports whether err matches target using the error kind.
// This allows checking error kinds without exposing the full error type.
func Is(err error, kind Kind) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Kind == kind
	}
	return false
}

// GetKind extracts the error kind from an error.
// Returns KindUnknown if the error is not an API error.
func GetKind(err error) Kind {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Kind
	}
	return KindUnknown
}

// GetHTTPStatus extracts the HTTP status code from an error.
// Returns 500 for unknown errors.
func GetHTTPStatus(err error) int {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus()
	}
	return http.StatusInternalServerError
}

// FromDatabaseError converts common database errors to API errors.
// This is useful for mapping database-layer errors to appropriate API responses.
func FromDatabaseError(err error, resourceType, identifier string) *Error {
	if err == nil {
		return nil
	}

	// Check for common database error sentinels
	// These string checks allow compatibility without importing the database package
	errStr := err.Error()

	if contains(errStr, "not found") || contains(errStr, "no rows") {
		return NotFound(resourceType, identifier)
	}

	if contains(errStr, "already exists") || contains(errStr, "unique constraint") || contains(errStr, "duplicate") {
		return Conflictf("%s '%s' already exists", resourceType, identifier)
	}

	// For other database errors, return internal error
	return &Error{
		Kind:    KindInternal,
		Message: fmt.Sprintf("Failed to access %s", resourceType),
		Err:     err,
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
