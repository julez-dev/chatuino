package command

import (
	"fmt"
	"time"
)

type PrivateMessage struct {
	ID             string
	ParentThreadID string
	ParentMsgID    string

	From      string
	In        string
	Message   string
	UserColor string
	SentAt    time.Time
}

// socket.send(`PRIVMSG ${room} :${message}`);
func (p *PrivateMessage) IRC() string {
	if p.ParentMsgID != "" {
		return fmt.Sprintf("@reply-parent-msg-id=%s PRIVMSG #%s :%s", p.ParentMsgID, p.In, p.Message)
	}

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
