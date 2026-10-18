package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxAPIResponseBodyBytes int64 = 32 << 20

// readAndRestoreBody reads a response body and restores it for later use.
func readAndRestoreBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}

	body, err := readBodyWithLimit(resp.Body, maxAPIResponseBodyBytes)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return body, nil
}

// ReadResponseBody reads a successful Omada response with a bounded memory
// footprint. HTTP errors include Omada's structured message when available,
// without echoing arbitrary response bodies that may contain sensitive data.
func ReadResponseBody(resp *http.Response, endpointName string) ([]byte, error) {
	return readResponseBody(resp, endpointName, maxAPIResponseBodyBytes)
}

func readResponseBody(resp *http.Response, endpointName string, limit int64) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("%s returned no HTTP response", endpointName)
	}
	if resp.Body == nil {
		return nil, fmt.Errorf("%s returned an empty HTTP body", endpointName)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%s response body exceeds %d bytes", endpointName, limit)
	}

	body, err := readBodyWithLimit(resp.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", endpointName, err)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return body, nil
	}

	status := resp.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	var apiErr apiErrorResponse
	if json.Unmarshal(body, &apiErr) == nil {
		message := strings.TrimSpace(firstNonEmpty(apiErr.Msg, apiErr.ErrorMsg))
		if code, ok := apiErr.errorCode(); ok {
			if message != "" {
				return nil, fmt.Errorf("%s returned HTTP %s: errorCode %d: %s", endpointName, status, code, message)
			}
			return nil, fmt.Errorf("%s returned HTTP %s: errorCode %d", endpointName, status, code)
		}
		if message != "" {
			return nil, fmt.Errorf("%s returned HTTP %s: %s", endpointName, status, message)
		}
	}
	return nil, fmt.Errorf("%s returned HTTP %s", endpointName, status)
}

func readBodyWithLimit(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return body, nil
}

// redactURLString removes secrets from URLs before they are logged or returned.
func redactURLString(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsed.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "access_token", "client_secret", "password", "refresh_token", "token":
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactError(err error) error {
	if err == nil {
		return nil
	}

	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		redacted := *urlErr
		redacted.URL = redactURLString(urlErr.URL)
		return &redacted
	}

	return err
}

// apiErrorResponse represents an error payload returned by Omada.
type apiErrorResponse struct {
	ErrorCode      *int   `json:"errorCode"`
	ErrorCodeSnake *int   `json:"error_code"`
	Msg            string `json:"msg"`
	ErrorMsg       string `json:"errorMsg"`
}

func (r apiErrorResponse) errorCode() (int, bool) {
	if r.ErrorCodeSnake != nil {
		return *r.ErrorCodeSnake, true
	}
	if r.ErrorCode != nil {
		return *r.ErrorCode, true
	}
	return 0, false
}

// ValidateAPIResponse returns an error when Omada wraps a failed request in an
// otherwise successful HTTP response.
func ValidateAPIResponse(body []byte, endpointName string) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return nil
	}

	code, hasCode := apiErr.errorCode()
	if !hasCode || code == 0 {
		return nil
	}

	message := strings.TrimSpace(firstNonEmpty(apiErr.Msg, apiErr.ErrorMsg))
	if message == "" {
		message = "Omada API error"
	}
	return fmt.Errorf("%s returned errorCode %d: %s", endpointName, code, message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// isHTTPAuthStatus reports whether the status code indicates an authentication failure.
func isHTTPAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// isAuthRelatedMessage reports whether a message points to an authentication problem.
func isAuthRelatedMessage(message string) bool {
	message = strings.ToLower(message)
	if message == "" {
		return false
	}

	return strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden") ||
		(strings.Contains(message, "token") && (strings.Contains(message, "expired") || strings.Contains(message, "invalid") || strings.Contains(message, "missing"))) ||
		(strings.Contains(message, "auth") && (strings.Contains(message, "fail") || strings.Contains(message, "expired")))
}

// isWebAPIAuthFailure inspects a response for Web API authentication failures.
func isWebAPIAuthFailure(resp *http.Response) (bool, error) {
	if resp == nil {
		return false, nil
	}
	if isHTTPAuthStatus(resp.StatusCode) {
		return true, nil
	}

	body, err := readAndRestoreBody(resp)
	if err != nil {
		return false, err
	}

	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return false, nil
	}

	if apiErr.ErrorCode != nil && *apiErr.ErrorCode == -1200 {
		return true, nil
	}
	if apiErr.ErrorCodeSnake != nil && *apiErr.ErrorCodeSnake == -1200 {
		return true, nil
	}

	return isAuthRelatedMessage(apiErr.Msg) || isAuthRelatedMessage(apiErr.ErrorMsg), nil
}

// isOpenAPIAuthFailure inspects a response for Open API authentication failures.
func isOpenAPIAuthFailure(resp *http.Response) (bool, error) {
	if resp == nil {
		return false, nil
	}
	if isHTTPAuthStatus(resp.StatusCode) {
		return true, nil
	}

	body, err := readAndRestoreBody(resp)
	if err != nil {
		return false, err
	}

	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return false, nil
	}

	if apiErr.ErrorCode != nil && isHTTPAuthStatus(*apiErr.ErrorCode) {
		return true, nil
	}
	if apiErr.ErrorCodeSnake != nil && isHTTPAuthStatus(*apiErr.ErrorCodeSnake) {
		return true, nil
	}

	return isAuthRelatedMessage(apiErr.Msg) || isAuthRelatedMessage(apiErr.ErrorMsg), nil
}
