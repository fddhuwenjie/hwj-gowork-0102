package platform

import "fmt"

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewAPIError(code, message string, details any) *APIError {
	return &APIError{Code: code, Message: message, Details: details}
}
