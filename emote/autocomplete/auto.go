package autocomplete

import (
	"strings"

	"github.com/julez-dev/chatuino/emote"
)

type Completer struct {
	allEmotes   emote.EmoteSet
	search      string
	foundEmotes emote.EmoteSet
	index       int
}

func NewCompleter(emoteSet emote.EmoteSet) Completer {
	c := Completer{
		allEmotes: emoteSet,
		index:     -1,
	}

	c.refreshEmotes()

	return c
}

func (c *Completer) Prev() bool {
	if len(c.foundEmotes) < 1 {
		c.index = -1
		return false
	}

	if c.index-1 < 0 {
		c.index = len(c.foundEmotes) - 1
		return true
	}

	c.index--
	return true
}

func (c *Completer) Next() bool {
	if len(c.foundEmotes) < 1 {
		c.index = -1
		return false
	}

	if c.index+1 >= len(c.foundEmotes) {
		c.index = 0
		return true
	}

	c.index++

	return true
}

func (c *Completer) AddToSearch(s string) {
	c.search += strings.ToLower(s)
	c.refreshEmotes()
	c.index = -1
}

func (c *Completer) SetSearch(s string) {
	c.search = strings.ToLower(s)
	c.refreshEmotes()
	c.index = -1
}

func (c Completer) HasSearch() bool {
	return c.search != ""
}

func (c *Completer) Reset() {
	c.index = -1
	c.search = ""
	c.refreshEmotes()
}

func (c Completer) Current() emote.Emote {
	if c.index >= 0 && c.index < len(c.foundEmotes) {
		return c.foundEmotes[c.index]
	}

	return emote.Emote{}
}

func (c *Completer) refreshEmotes() {
	c.foundEmotes = nil

	for _, e := range c.allEmotes {
		lower := strings.ToLower(e.Text)

		if strings.Contains(lower, c.search) {
			c.foundEmotes = append(c.foundEmotes, e)
		}
	}
}
