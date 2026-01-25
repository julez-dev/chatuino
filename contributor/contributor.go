package contributor

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed contributors.json
var contributorsData []byte

//go:embed chatuino_logo_black.png
var logoPNG []byte

// Contributor represents a person who has contributed to Chatuino.
type Contributor struct {
	GitHubUser  string `json:"github_user"`
	Email       string `json:"email"`
	TwitchLogin string `json:"twitch_login,omitempty"`
}

var (
	once         sync.Once
	contributors []Contributor
	loginSet     map[string]struct{}
)

func parse() {
	var data struct {
		Contributors []Contributor `json:"contributors"`
	}

	if err := json.Unmarshal(contributorsData, &data); err != nil {
		return
	}

	contributors = data.Contributors
	loginSet = make(map[string]struct{}, len(contributors))

	for _, c := range contributors {
		if c.TwitchLogin != "" {
			loginSet[strings.ToLower(c.TwitchLogin)] = struct{}{}
		}
	}
}

// IsContributor returns true if the given Twitch login name belongs to a contributor.
func IsContributor(twitchLogin string) bool {
	once.Do(parse)
	_, ok := loginSet[strings.ToLower(twitchLogin)]
	return ok
}

// GetAll returns all contributors.
func GetAll() []Contributor {
	once.Do(parse)
	return contributors
}

// LogoPNG returns the raw bytes of the embedded Chatuino logo PNG.
func LogoPNG() []byte {
	return logoPNG
}
