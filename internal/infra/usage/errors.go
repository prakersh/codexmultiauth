package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// RequestError exposes status and recognized machine codes without retaining
// server descriptions, which may contain credentials echoed from a request.
type RequestError struct {
	Operation  string
	StatusCode int
	Code       string
}

// ErrorCode returns only recognized public machine codes.
func (e *RequestError) ErrorCode() string {
	return safeErrorCode(e.Code)
}

func (e *RequestError) Error() string {
	operation := "API request"
	switch e.Operation {
	case "refresh":
		operation = "refresh token request"
	case "usage":
		operation = "usage request"
	}
	message := fmt.Sprintf("%s failed: status %d", operation, e.StatusCode)
	if code := safeErrorCode(e.Code); code != "" {
		message += " (" + code + ")"
	}
	return message
}

// RequiresLogin distinguishes permanent credential rejection from failures
// such as a temporary outage or usage permission denial.
func (e *RequestError) RequiresLogin() bool {
	switch e.Operation {
	case "usage":
		return e.StatusCode == http.StatusUnauthorized
	case "refresh":
		if e.StatusCode == http.StatusUnauthorized {
			return true
		}
		switch safeErrorCode(e.Code) {
		case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated", "refresh_token_revoked":
			return true
		case "invalid_grant":
			return e.StatusCode == http.StatusBadRequest
		}
	}
	return false
}

// AuthenticationRejected also includes usage 403, which can indicate account
// permissions and does not establish that signing in will fix the failure.
func (e *RequestError) AuthenticationRejected() bool {
	return e.RequiresLogin() || (e.Operation == "usage" && e.StatusCode == http.StatusForbidden)
}

// IsUnauthorized reports usage 401 only, including wrapped errors. Refresh
// failures must not trigger another refresh attempt.
func IsUnauthorized(err error) bool {
	var requestErr *RequestError
	return errors.As(err, &requestErr) && requestErr.Operation == "usage" && requestErr.StatusCode == http.StatusUnauthorized
}

func newRequestError(operation string, status int, body []byte) *RequestError {
	err := &RequestError{Operation: operation, StatusCode: status}
	var payload struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return err
	}
	var nested struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(payload.Error, &nested) == nil && nested.Code != "" {
		err.Code = safeErrorCode(nested.Code)
		return err
	}
	var code string
	if json.Unmarshal(payload.Error, &code) == nil && code != "" {
		err.Code = safeErrorCode(code)
		return err
	}
	err.Code = safeErrorCode(payload.Code)
	return err
}

func safeErrorCode(code string) string {
	switch strings.ToLower(code) {
	case "refresh_token_expired":
		return "refresh_token_expired"
	case "refresh_token_reused":
		return "refresh_token_reused"
	case "refresh_token_invalidated":
		return "refresh_token_invalidated"
	case "refresh_token_revoked":
		return "refresh_token_revoked"
	case "invalid_grant":
		return "invalid_grant"
	case "invalid_client":
		return "invalid_client"
	case "invalid_request":
		return "invalid_request"
	case "unauthorized_client":
		return "unauthorized_client"
	case "temporarily_unavailable":
		return "temporarily_unavailable"
	case "server_error":
		return "server_error"
	case "token_expired":
		return "token_expired"
	default:
		return ""
	}
}
