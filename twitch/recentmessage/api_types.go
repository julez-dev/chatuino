package recentmessage

import "fmt"

type APIError struct {
	Status        int    `json:"status"`
	StatusMessage string `json:"status_message"`
	ErrorMessage  string `json:"error"`
	ErrorCode     string `json:"error_code"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s (errcode: %s)", a.StatusMessage, a.Status, a.ErrorMessage, a.ErrorCode)
}

type (
	//easyjson:json
	responseData struct {
		Messages  []string `json:"messages"`
		Error     string   `json:"error"`
		ErrorCode string   `json:"error_code"`
	}
)
