package twitch

import (
	"fmt"
	"time"
)

type PrivateMessage struct {
	From    string
	In      string
	Message string
	SentAt  time.Time
}

func (p *PrivateMessage) IRC() string {
	return fmt.Sprintf("PRIVMSG #%s :%s", p.In, p.Message)
}

type PongMessage struct{}

func (p PongMessage) IRC() string {
	return "PONG :tmi.twitch.tv"
}

type PingMessage struct{}

func (p PingMessage) IRC() string {
	return "PING :tmi.twitch.tv"
}

type JoinMessage struct {
	Channel string
}

func (j JoinMessage) IRC() string {
	return "JOIN #" + j.Channel
}
