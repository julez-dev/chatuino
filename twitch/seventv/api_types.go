package seventv

import "fmt"

type APIError struct {
	StatusCode int    `json:"status_code"`
	Status     string `json:"status"`
	ErrorText  string `json:"error"`
	ErrorCode  int    `json:"error_code"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s (errcode: %d)", a.Status, a.StatusCode, a.ErrorText, a.ErrorCode)
}

type (
	ChannelEmoteResponse struct {
		EmoteSet struct {
			Emotes []Emote `json:"emotes"`
		} `json:"emote_set"`
	}
)

type (
	EmoteResponse struct {
		Emotes []Emote `json:"emotes"`
	}
	Emote struct {
		ID   string    `json:"id"`
		Name string    `json:"name"`
		Data EmoteData `json:"data"`
	}
	EmoteData struct {
		Animated bool `json:"animated"`
		Host     Host `json:"host"`
	}
	Files struct {
		Name       string `json:"name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		Format     string `json:"format"`
		StaticName string `json:"static_name"`
		FrameCount int    `json:"frame_count"`
	}
	Host struct {
		URL   string  `json:"url"`
		Files []Files `json:"files"`
	}
)
