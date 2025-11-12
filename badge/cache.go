package badge

import (
	"context"
	"sync"

	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"golang.org/x/sync/singleflight"
)

type BadgeFetcher interface {
	GetGlobalChatBadges(ctx context.Context) ([]twitch.BadgeSet, error)
	GetChannelChatBadges(ctx context.Context, broadcasterID string) ([]twitch.BadgeSet, error)
}

type Cache struct {
	globalBadges  []twitch.BadgeSet
	channelBadges map[string][]twitch.BadgeSet // channelID:badgeSet

	single *singleflight.Group
	l      *sync.RWMutex

	fetcher BadgeFetcher
}

func NewCache(fetcher BadgeFetcher) *Cache {
	return &Cache{
		l:             &sync.RWMutex{},
		fetcher:       fetcher,
		single:        &singleflight.Group{},
		channelBadges: make(map[string][]twitch.BadgeSet),
	}
}

func (c *Cache) RefreshGlobal(ctx context.Context) error {
	badges, err := c.fetcher.GetGlobalChatBadges(ctx)
	if err != nil {
		return err
	}

	c.l.Lock()
	c.globalBadges = badges
	c.l.Unlock()
	return nil
}

func (c *Cache) RefreshChannel(ctx context.Context, broadcasterID string) error {
	_, err, _ := c.single.Do(broadcasterID, func() (any, error) {
		badges, err := c.fetcher.GetChannelChatBadges(ctx, broadcasterID)
		if err != nil {
			return nil, err
		}

		c.l.Lock()
		c.channelBadges[broadcasterID] = badges
		c.l.Unlock()

		return nil, nil
	})

	return err
}

// badges=subscriber/6,arc-raiders-launch-2025/1
// MatchBadgeSet uses the irc badge tag data to find and match the global and channel badges.
// The key of the result map is the badge set id with the matched version.
func (c *Cache) MatchBadgeSet(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
	// preprocess for more efficient look ups
	flattenIRCBadges := make(map[string]string, len(ircBadge))
	for _, b := range ircBadge {
		flattenIRCBadges[b.Name] = b.Version
	}

	c.l.RLock()
	defer c.l.RUnlock()

	result := make(map[string]twitch.BadgeVersion)

	c.findAndAdd(flattenIRCBadges, c.globalBadges, result)

	if broadcasterID == "" {
		return result
	}

	broadcasterBadges, ok := c.channelBadges[broadcasterID]
	if !ok {
		return result
	}

	c.findAndAdd(flattenIRCBadges, broadcasterBadges, result)

	return result
}

func (c *Cache) findAndAdd(ircBadges map[string]string, badgeSets []twitch.BadgeSet, result map[string]twitch.BadgeVersion) {
	for _, b := range badgeSets {
		version, ok := ircBadges[b.ID]
		if !ok {
			continue
		}

		for _, v := range b.Versions {
			if v.ID == version {
				result[b.ID] = v
				break
			}
		}
	}
}
