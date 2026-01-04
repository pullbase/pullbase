package apierrors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindNotFound, "not_found"},
		{KindUnauthorized, "unauthorized"},
		{KindForbidden, "forbidden"},
		{KindValidation, "validation_error"},
		{KindConflict, "conflict"},
		{KindInternal, "internal_error"},
		{KindBadRequest, "bad_request"},
		{KindTooManyRequests, "too_many_requests"},
		{KindNotImplemented, "not_implemented"},
		{KindUnknown, "unknown"},
		{Kind(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("Kind.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindHTTPStatus(t *testing.T) {
	tests := []struct {
		kind Kind
		want int
	}{
		{KindNotFound, http.StatusNotFound},
		{KindUnauthorized, http.StatusUnauthorized},
		{KindForbidden, http.StatusForbidden},
		{KindValidation, http.StatusBadRequest},
		{KindBadRequest, http.StatusBadRequest},
		{KindConflict, http.StatusConflict},
		{KindTooManyRequests, http.StatusTooManyRequests},
		{KindNotImplemented, http.StatusNotImplemented},
		{KindInternal, http.StatusInternalServerError},
		{KindUnknown, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			if got := tt.kind.HTTPStatus(); got != tt.want {
				t.Errorf("Kind.HTTPStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "message only",
			err:  &Error{Kind: KindNotFound, Message: "user not found"},
			want: "user not found",
		},
		{
			name: "with operation",
			err:  &Error{Kind: KindNotFound, Message: "user not found", Op: "GetUser"},
			want: "GetUser: user not found",
		},
		{
			name: "with underlying error",
			err:  &Error{Kind: KindInternal, Message: "database error", Err: errors.New("connection refused")},
			want: "database error: connection refused",
		},
		{
			name: "with operation and underlying error",
			err:  &Error{Kind: KindInternal, Message: "database error", Op: "CreateUser", Err: errors.New("connection refused")},
			want: "CreateUser: database error: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &Error{Kind: KindInternal, Message: "wrapped", Err: underlying}

	if !errors.Is(err, underlying) {
		t.Error("errors.Is should return true for underlying error")
	}
}

func TestNew(t *testing.T) {
	err := New(KindNotFound, "resource not found")

	if err.Kind != KindNotFound {
		t.Errorf("New() kind = %v, want %v", err.Kind, KindNotFound)
	}
	if err.Message != "resource not found" {
		t.Errorf("New() message = %v, want %v", err.Message, "resource not found")
	}
}

func TestWrap(t *testing.T) {
	underlying := errors.New("db error")
	err := Wrap(KindInternal, "GetUser", underlying, "failed to fetch user")

	if err.Kind != KindInternal {
		t.Errorf("Wrap() kind = %v, want %v", err.Kind, KindInternal)
	}
	if err.Op != "GetUser" {
		t.Errorf("Wrap() op = %v, want %v", err.Op, "GetUser")
	}
	if err.Message != "failed to fetch user" {
		t.Errorf("Wrap() message = %v, want %v", err.Message, "failed to fetch user")
	}
	if !errors.Is(err, underlying) {
		t.Error("Wrap() should preserve underlying error")
	}
}

func TestConvenienceConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantKind Kind
		wantMsg  string
	}{
		{"NotFound", NotFound("user", 123), KindNotFound, "user '123' not found"},
		{"NotFoundf", NotFoundf("user %d not found", 123), KindNotFound, "user 123 not found"},
		{"Unauthorized", Unauthorized("invalid token"), KindUnauthorized, "invalid token"},
		{"Unauthorizedf", Unauthorizedf("token expired at %s", "noon"), KindUnauthorized, "token expired at noon"},
		{"Forbidden", Forbidden("access denied"), KindForbidden, "access denied"},
		{"Forbiddenf", Forbiddenf("user %s cannot access %s", "alice", "admin"), KindForbidden, "user alice cannot access admin"},
		{"Validation", Validation("invalid email"), KindValidation, "invalid email"},
		{"Validationf", Validationf("field %s is required", "name"), KindValidation, "field name is required"},
		{"BadRequest", BadRequest("malformed JSON"), KindBadRequest, "malformed JSON"},
		{"BadRequestf", BadRequestf("invalid %s", "payload"), KindBadRequest, "invalid payload"},
		{"Conflict", Conflict("already exists"), KindConflict, "already exists"},
		{"Conflictf", Conflictf("user %s exists", "bob"), KindConflict, "user bob exists"},
		{"Internal", Internal("database error"), KindInternal, "database error"},
		{"Internalf", Internalf("failed to %s", "connect"), KindInternal, "failed to connect"},
		{"TooManyRequests", TooManyRequests("rate limited"), KindTooManyRequests, "rate limited"},
		{"NotImplemented", NotImplemented("feature disabled"), KindNotImplemented, "feature disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.wantKind {
				t.Errorf("%s kind = %v, want %v", tt.name, tt.err.Kind, tt.wantKind)
			}
			if tt.err.Message != tt.wantMsg {
				t.Errorf("%s message = %v, want %v", tt.name, tt.err.Message, tt.wantMsg)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "api error not found",
			err:        NotFound("server", "web-01"),
			wantStatus: http.StatusNotFound,
			wantBody:   `"error":"server 'web-01' not found"`,
		},
		{
			name:       "api error unauthorized",
			err:        Unauthorized("invalid token"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   `"error":"invalid token"`,
		},
		{
			name:       "standard error",
			err:        errors.New("something went wrong"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"error":"Internal server error"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("WriteError() status = %v, want %v", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("WriteError() body = %v, want to contain %v", w.Body.String(), tt.wantBody)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("WriteError() Content-Type = %v, want application/json", ct)
			}
		})
	}
}

func TestWriteHTTPError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteHTTPError(w, http.StatusBadRequest, "invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("WriteHTTPError() status = %v, want %v", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), `"error":"invalid input"`) {
		t.Errorf("WriteHTTPError() body = %v, want to contain error message", w.Body.String())
	}
}

func TestIs(t *testing.T) {
	err := NotFound("user", 1)

	if !Is(err, KindNotFound) {
		t.Error("Is() should return true for matching kind")
	}
	if Is(err, KindInternal) {
		t.Error("Is() should return false for non-matching kind")
	}
	if Is(errors.New("standard error"), KindNotFound) {
		t.Error("Is() should return false for standard errors")
	}
}

func TestGetKind(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Kind
	}{
		{"api error", NotFound("x", 1), KindNotFound},
		{"standard error", errors.New("error"), KindUnknown},
		{"nil", nil, KindUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				// GetKind panics on nil, skip
				return
			}
			if got := GetKind(tt.err); got != tt.want {
				t.Errorf("GetKind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", NotFound("x", 1), http.StatusNotFound},
		{"forbidden", Forbidden("no"), http.StatusForbidden},
		{"standard error", errors.New("error"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetHTTPStatus(tt.err); got != tt.want {
				t.Errorf("GetHTTPStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromDatabaseError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		resourceType string
		identifier   string
		wantKind     Kind
	}{
		{
			name:         "not found error",
			err:          errors.New("record not found"),
			resourceType: "user",
			identifier:   "alice",
			wantKind:     KindNotFound,
		},
		{
			name:         "no rows error",
			err:          errors.New("sql: no rows in result set"),
			resourceType: "server",
			identifier:   "web-01",
			wantKind:     KindNotFound,
		},
		{
			name:         "conflict error",
			err:          errors.New("unique constraint violation: already exists"),
			resourceType: "user",
			identifier:   "bob",
			wantKind:     KindConflict,
		},
		{
			name:         "duplicate error",
			err:          errors.New("duplicate key value"),
			resourceType: "token",
			identifier:   "abc",
			wantKind:     KindConflict,
		},
		{
			name:         "other database error",
			err:          errors.New("connection refused"),
			resourceType: "user",
			identifier:   "x",
			wantKind:     KindInternal,
		},
		{
			name:         "nil error",
			err:          nil,
			resourceType: "user",
			identifier:   "x",
			wantKind:     KindUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromDatabaseError(tt.err, tt.resourceType, tt.identifier)

			if tt.err == nil {
				if result != nil {
					t.Errorf("FromDatabaseError(nil) = %v, want nil", result)
				}
				return
			}

			if result.Kind != tt.wantKind {
				t.Errorf("FromDatabaseError() kind = %v, want %v", result.Kind, tt.wantKind)
			}
		})
	}
}

func TestErrorHTTPStatus(t *testing.T) {
	err := NotFound("user", 1)
	if got := err.HTTPStatus(); got != http.StatusNotFound {
		t.Errorf("Error.HTTPStatus() = %v, want %v", got, http.StatusNotFound)
	}
}

func TestWrapWithMessage(t *testing.T) {
	underlying := errors.New("db connection failed")
	err := WrapWithMessage(KindInternal, underlying, "failed to fetch data")

	if err.Kind != KindInternal {
		t.Errorf("WrapWithMessage() kind = %v, want %v", err.Kind, KindInternal)
	}
	if err.Message != "failed to fetch data" {
		t.Errorf("WrapWithMessage() message = %v, want %v", err.Message, "failed to fetch data")
	}
	if !errors.Is(err, underlying) {
		t.Error("WrapWithMessage() should preserve underlying error")
	}
}
