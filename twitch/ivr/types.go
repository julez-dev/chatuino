package ivr

import "time"

type (
	SubAgeResponse struct {
		User         User       `json:"user"`
		Channel      Channel    `json:"channel"`
		StatusHidden bool       `json:"statusHidden"`
		FollowedAt   time.Time  `json:"followedAt"`
		Streak       Streak     `json:"streak"`
		Cumulative   Cumulative `json:"cumulative"`
	}

	Streak struct {
		ElapsedDays   int       `json:"elapsedDays"`
		DaysRemaining int       `json:"daysRemaining"`
		Months        int       `json:"months"`
		End           time.Time `json:"end"`
		Start         time.Time `json:"start"`
	}

	User struct {
		ID          string `json:"id"`
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
	}

	Channel struct {
		ID          string `json:"id"`
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
	}

	Cumulative struct {
		ElapsedDays   int       `json:"elapsedDays"`
		DaysRemaining int       `json:"daysRemaining"`
		Months        int       `json:"months"`
		End           time.Time `json:"end"`
		Start         time.Time `json:"start"`
	}
)

type (
	ModVIPResponse struct {
		Mods []PrivilegedUser `json:"mods"`
		VIPs []PrivilegedUser `json:"vips"`
	}
	PrivilegedUser struct {
		ID          string    `json:"id"`
		Login       string    `json:"login"`
		DisplayName string    `json:"displayName"`
		GrantedAt   time.Time `json:"grantedAt"`
	}
)
