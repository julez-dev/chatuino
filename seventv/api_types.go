package seventv

import "fmt"

type APIError struct {
	StatusCode int    `json:"status_code"`
	Status     string `json:"status"`
	ErrorText  string `json:"error"`
	ErrorCode  int    `json:"error_code"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s (%d)", a.Status, a.StatusCode, a.ErrorText, a.ErrorCode)
}

type (
	EmoteResponse struct {
		Emotes []Emotes `json:"emotes"`
	}
	Emotes struct {
		ID   string    `json:"id"`
		Name string    `json:"name"`
		Data EmoteData `json:"data"`
	}
	EmoteData struct {
		Animated bool `json:"animated"`
		Host     Host `json:"host"`
	}
	Files struct {
		Name   string `json:"name"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Format string `json:"format"`
	}
	Host struct {
		URL   string  `json:"url"`
		Files []Files `json:"files"`
	}
)
