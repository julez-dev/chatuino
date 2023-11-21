package twitch

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/command"
)

var (
	// ErrZeroLengthMessage is returned when parsing if the input is
	// zero-length.
	ErrZeroLengthMessage = errors.New("irc: cannot parse zero-length message")

	// ErrMissingDataAfterPrefix is returned when parsing if there is
	// no message data after the prefix.
	ErrMissingDataAfterPrefix = errors.New("irc: no message data after prefix")

	// ErrMissingDataAfterTags is returned when parsing if there is no
	// message data after the tags.
	ErrMissingDataAfterTags = errors.New("irc: no message data after tags")

	// ErrMissingCommand is returned when parsing if there is no
	// command in the parsed message.
	ErrMissingCommand = errors.New("irc: missing message command")

	ErrUnhandledCommand = errors.New("irc: message command not handled by parser")
)

type tagValue string

type tags map[string]tagValue

type rawTMI struct {
	// Each message can have IRCv3 tags
	tags `json:"tags"`

	// Each message can have a Prefix
	*prefix

	// Command is which command is being called.
	Command string `json:"command"`

	// Params are all the arguments for the command.
	Params []string `json:"params"`

	Message string `json:"message"`
}

type prefix struct {
	// Name will contain the nick of who sent the message, the
	// server who sent the message, or a blank string
	Name string `json:"name"`

	// User will either contain the user who sent the message or a blank string
	User string `json:"user"`

	// Host will either contain the host of who sent the message or a blank string
	Host string `json:"host"`
}

var tagDecodeSlashMap = map[rune]rune{
	':':  ';',
	's':  ' ',
	'\\': '\\',
	'r':  '\r',
	'n':  '\n',
}

func parseIRC(message string) (IRCer, error) {
	// Trim the line and make sure we have data
	message = strings.TrimRight(message, "\r\n")
	if len(message) == 0 {
		return nil, ErrZeroLengthMessage
	}

	c := &rawTMI{
		tags:    tags{},
		prefix:  &prefix{},
		Message: message,
	}

	if message[0] == '@' {
		loc := strings.Index(message, " ")
		if loc == -1 {
			return nil, ErrMissingDataAfterTags
		}

		c.tags = parseTags(message[1:loc])
		message = message[loc+1:]
	}

	if message[0] == ':' {
		loc := strings.Index(message, " ")
		if loc == -1 {
			return nil, ErrMissingDataAfterPrefix
		}

		// Parse the identity, if there was one
		c.prefix = parsePrefix(message[1:loc])
		message = message[loc+1:]
	}

	// Split out the trailing then the rest of the args. Because
	// we expect there to be at least one result as an arg (the
	// command) we don't need to special case the trailing arg and
	// can just attempt a split on " :"
	split := strings.SplitN(message, " :", 2)
	c.Params = strings.FieldsFunc(split[0], func(r rune) bool {
		return r == ' '
	})

	// If there are no args, we need to bail because we need at
	// least the command.
	if len(c.Params) == 0 {
		return nil, ErrMissingCommand
	}

	// If we had a trailing arg, append it to the other args
	if len(split) == 2 {
		c.Params = append(c.Params, split[1])
	}

	// Because of how it's parsed, the Command will show up as the
	// first arg.
	c.Command = strings.ToUpper(c.Params[0])
	c.Params = c.Params[1:]

	// If there are no params, set it to nil, to make writing tests and other
	// things simpler.
	if len(c.Params) == 0 {
		c.Params = nil
	}

	switch c.Command {
	case "PRIVMSG":

		p := command.PrivateMessage{
			ID:             string(c.tags["id"]),
			ParentThreadID: string(c.tags["reply-thread-parent-msg-id"]),
			ParentMsgID:    string(c.tags["reply-parent-msg-id"]),
			From:           string(c.tags["display-name"]),
			In:             strings.TrimPrefix(c.Params[0], "#"),
			Message:        c.Params[1],
			UserColor:      string(c.tags["color"]),
			SentAt:         parseTimestamp(string(c.tags["tmi-sent-ts"])),
		}

		if badgeStr := c.tags["badges"]; badgeStr != "" {
			badgeSplit := strings.Split(string(badgeStr), ",")
			p.Badges = make([]command.Badge, 0, len(badgeSplit))

			for _, badge := range badgeSplit {
				parts := strings.SplitN(badge, "/", 2)
				if len(parts) == 1 {
					p.Badges = append(p.Badges, command.Badge{Name: parts[0]})
					continue
				}

				count, err := strconv.Atoi(parts[1])
				if err != nil {
					p.Badges = append(p.Badges, command.Badge{Name: parts[0]})
					continue
				}

				p.Badges = append(p.Badges, command.Badge{Name: parts[0], Count: count})
			}
		}

		return &p, nil
	case "PING":
		return command.PingMessage{}, nil
	}

	return nil, ErrUnhandledCommand
}

func parseTimestamp(timeStr string) time.Time {
	i, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, i*1e6)
}

func parsePrefix(line string) *prefix {
	// Start by creating a Prefix with nothing but the host
	id := &prefix{
		Name: line,
	}

	uh := strings.SplitN(id.Name, "@", 2)
	if len(uh) == 2 {
		id.Name, id.Host = uh[0], uh[1]
	}

	nu := strings.SplitN(id.Name, "!", 2)
	if len(nu) == 2 {
		id.Name, id.User = nu[0], nu[1]
	}

	return id
}

func parseTagValue(v string) tagValue {
	ret := &bytes.Buffer{}

	input := bytes.NewBufferString(v)

	for {
		c, _, err := input.ReadRune()
		if err != nil {
			break
		}

		if c == '\\' {
			c2, _, err := input.ReadRune()
			// If we got a backslash then the end of the tag value, we should
			// just ignore the backslash.
			if err != nil {
				break
			}

			if replacement, ok := tagDecodeSlashMap[c2]; ok {
				ret.WriteRune(replacement)
			} else {
				ret.WriteRune(c2)
			}
		} else {
			ret.WriteRune(c)
		}
	}

	return tagValue(ret.String())
}

func parseTags(line string) tags {
	ret := tags{}

	tags := strings.Split(line, ";")
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) < 2 {
			ret[parts[0]] = ""
			continue
		}

		ret[parts[0]] = parseTagValue(parts[1])
	}

	return ret
}
