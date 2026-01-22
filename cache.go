package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/urfave/cli/v3"
)

// Cache output styling constants
const (
	cacheBoxWidth     = 66
	maxChannelsShown  = 10
	maxBarWidth       = 20
	maxChannelNameLen = 14
)

// Colors (Nord-inspired palette)
var (
	cacheBorderColor  = lipgloss.Color("#88c0d0") // cyan
	cacheHeaderColor  = lipgloss.Color("#ebcb8b") // yellow/gold
	cacheEmoteColor   = lipgloss.Color("#b48ead") // purple
	cacheBadgeColor   = lipgloss.Color("#d08770") // orange
	cacheBarColor     = lipgloss.Color("#a3be8c") // green
	cacheDimmedColor  = lipgloss.Color("#4c566a") // gray
	cacheTextColor    = lipgloss.Color("#d8dee9") // white/bright
	cacheSuccessColor = lipgloss.Color("#a3be8c") // green
)

// Styles
var (
	cacheBorderStyle  = lipgloss.NewStyle().Foreground(cacheBorderColor)
	cacheHeaderStyle  = lipgloss.NewStyle().Foreground(cacheHeaderColor).Bold(true)
	cacheEmoteStyle   = lipgloss.NewStyle().Foreground(cacheEmoteColor)
	cacheBadgeStyle   = lipgloss.NewStyle().Foreground(cacheBadgeColor)
	cacheBarStyle     = lipgloss.NewStyle().Foreground(cacheBarColor)
	cacheDimmedStyle  = lipgloss.NewStyle().Foreground(cacheDimmedColor)
	cacheTextStyle    = lipgloss.NewStyle().Foreground(cacheTextColor)
	cacheSuccessStyle = lipgloss.NewStyle().Foreground(cacheSuccessColor)
)

type imageStats struct {
	SizeBytes int64
	Images    int
	Frames    int
}

type channelMessageCount struct {
	Channel string
	Count   int64
}

// Border rendering helpers

func cacheTopBorder(label string) string {
	labelPart := "─[ " + label + " ]"
	fillWidth := cacheBoxWidth - 2 - lipgloss.Width(labelPart) // -2 for ┌ and ┐
	if fillWidth < 0 {
		fillWidth = 0
	}
	return cacheBorderStyle.Render("┌" + labelPart + strings.Repeat("─", fillWidth) + "┐")
}

func cacheMiddleBorder(label string) string {
	labelPart := "─[ " + label + " ]"
	fillWidth := cacheBoxWidth - 2 - lipgloss.Width(labelPart) // -2 for ├ and ┤
	if fillWidth < 0 {
		fillWidth = 0
	}
	return cacheBorderStyle.Render("├" + labelPart + strings.Repeat("─", fillWidth) + "┤")
}

func cacheBottomBorder() string {
	fillWidth := cacheBoxWidth - 2 // -2 for └ and ┘
	return cacheBorderStyle.Render("└" + strings.Repeat("─", fillWidth) + "┘")
}

func cacheRow(content string) string {
	innerWidth := cacheBoxWidth - 2 // -2 for left and right │
	contentWidth := lipgloss.Width(content)
	padWidth := innerWidth - 2 - contentWidth // -2 for "  " prefix
	if padWidth < 0 {
		padWidth = 0
	}
	return cacheBorderStyle.Render("│") + "  " + content + strings.Repeat(" ", padWidth) + cacheBorderStyle.Render("│")
}

func cacheEmptyRow() string {
	return cacheBorderStyle.Render("│") + strings.Repeat(" ", cacheBoxWidth-2) + cacheBorderStyle.Render("│")
}

// Helper functions

func truncateChannelName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-1] + "…"
}

func renderBar(count, maxCount int64) string {
	if maxCount == 0 {
		return ""
	}
	barLen := int(float64(count) / float64(maxCount) * float64(maxBarWidth))
	if barLen == 0 && count > 0 {
		barLen = 1 // at least 1 char for non-zero
	}
	return cacheBarStyle.Render(strings.Repeat("▓", barLen))
}

// Main render function

func renderCacheOutput(emote, badge imageStats, channels []channelMessageCount, totalMsgs int64) string {
	var b strings.Builder

	// Top section: Cache Statistics
	b.WriteString(cacheTopBorder("Cache Statistics"))
	b.WriteString("\n")
	b.WriteString(cacheEmptyRow())
	b.WriteString("\n")

	// Column headers
	headerRow := cacheTextStyle.Render(fmt.Sprintf("%-10s %-10s %-10s %-10s", "Category", "Size", "Images", "Frames"))
	b.WriteString(cacheRow(headerRow))
	b.WriteString("\n")

	// Separator - use plain dashes for consistent width
	sepRow := cacheDimmedStyle.Render("────────── ────────── ────────── ──────────")
	b.WriteString(cacheRow(sepRow))
	b.WriteString("\n")

	// Emote row
	emoteRow := cacheEmoteStyle.Render(fmt.Sprintf("%-10s", "Emotes")) +
		cacheTextStyle.Render(fmt.Sprintf(" %-10s %-10s %-10s",
			humanize.Bytes(uint64(emote.SizeBytes)),
			humanize.Comma(int64(emote.Images)),
			humanize.Comma(int64(emote.Frames))))
	b.WriteString(cacheRow(emoteRow))
	b.WriteString("\n")

	// Badge row
	badgeRow := cacheBadgeStyle.Render(fmt.Sprintf("%-10s", "Badges")) +
		cacheTextStyle.Render(fmt.Sprintf(" %-10s %-10s %-10s",
			humanize.Bytes(uint64(badge.SizeBytes)),
			humanize.Comma(int64(badge.Images)),
			humanize.Comma(int64(badge.Frames))))
	b.WriteString(cacheRow(badgeRow))
	b.WriteString("\n")

	b.WriteString(cacheEmptyRow())
	b.WriteString("\n")

	// Middle section: Messages by Channel
	b.WriteString(cacheMiddleBorder("Messages by Channel"))
	b.WriteString("\n")
	b.WriteString(cacheEmptyRow())
	b.WriteString("\n")

	if len(channels) == 0 {
		noMsgRow := cacheDimmedStyle.Render("No messages recorded")
		b.WriteString(cacheRow(noMsgRow))
		b.WriteString("\n")
	} else {
		// Find max count for bar scaling
		var maxCount int64
		for _, ch := range channels {
			if ch.Count > maxCount {
				maxCount = ch.Count
			}
		}

		// Show up to maxChannelsShown
		shown := channels
		remaining := 0
		if len(channels) > maxChannelsShown {
			shown = channels[:maxChannelsShown]
			remaining = len(channels) - maxChannelsShown
		}

		for _, ch := range shown {
			name := truncateChannelName(ch.Channel, maxChannelNameLen)
			bar := renderBar(ch.Count, maxCount)
			barPad := maxBarWidth - lipgloss.Width(bar)
			pct := float64(ch.Count) / float64(totalMsgs) * 100

			row := fmt.Sprintf("%-*s %s%s %8s  %s",
				maxChannelNameLen,
				name,
				bar,
				strings.Repeat(" ", barPad),
				humanize.Comma(ch.Count),
				cacheDimmedStyle.Render(fmt.Sprintf("(%5.1f%%)", pct)))
			b.WriteString(cacheRow(row))
			b.WriteString("\n")
		}

		if remaining > 0 {
			moreRow := cacheDimmedStyle.Render(fmt.Sprintf("...and %d more channels", remaining))
			b.WriteString(cacheRow(moreRow))
			b.WriteString("\n")
		}
	}

	b.WriteString(cacheEmptyRow())
	b.WriteString("\n")

	// Total row
	totalRow := cacheHeaderStyle.Render("Total: ") + cacheTextStyle.Render(humanize.Comma(totalMsgs)+" messages")
	b.WriteString(cacheRow(totalRow))
	b.WriteString("\n")

	// Bottom border
	b.WriteString(cacheBottomBorder())

	return b.String()
}

var cacheCMD = &cli.Command{
	Name:        "cache",
	Description: "Analyse cache data for images and message data used by Chatuino",
	Commands: []*cli.Command{
		{
			Name:        "clear",
			Usage:       "Manage deletion of cached data",
			Description: "Delete specified cached data",
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "emotes", Usage: "Delete emote image cache"},
				&cli.BoolFlag{Name: "database", Usage: "Delete database cache"},
				&cli.BoolFlag{Name: "badges", Usage: "Delete badge image cache"},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				checkmark := cacheSuccessStyle.Render("✓")

				if c.Bool("emotes") {
					if err := os.RemoveAll(filepath.Join(kittyimg.BaseImageDirectory, "emote")); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete emote cache: %w", err)
					}
					fmt.Println(checkmark + " " + cacheEmoteStyle.Render("Emote cache") + cacheTextStyle.Render(" deleted"))
				}

				if c.Bool("badges") {
					if err := os.RemoveAll(filepath.Join(kittyimg.BaseImageDirectory, "badge")); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete badge cache: %w", err)
					}
					fmt.Println(checkmark + " " + cacheBadgeStyle.Render("Badge cache") + cacheTextStyle.Render(" deleted"))
				}

				if c.Bool("database") {
					if err := os.Remove(dbFileName); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete database cache: %w", err)
					}
					fmt.Println(checkmark + " " + cacheHeaderStyle.Render("Database") + cacheTextStyle.Render(" deleted"))
				}

				return nil
			},
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		db, err := openDB(true)
		if err != nil {
			return fmt.Errorf("failed to open chatuino database: %w", err)
		}

		defer db.Close()

		// Collect emote stats
		emoteSizeBytes, emoteImages, emoteFrames, err := statsForImageDirectory(filepath.Join(kittyimg.BaseImageDirectory, "emote"))
		if err != nil {
			return fmt.Errorf("failed to calculate emote cache size: %w", err)
		}
		emoteStats := imageStats{
			SizeBytes: emoteSizeBytes,
			Images:    emoteImages,
			Frames:    emoteFrames,
		}

		// Collect badge stats
		badgeSizeBytes, badgeImages, badgeFrames, err := statsForImageDirectory(filepath.Join(kittyimg.BaseImageDirectory, "badge"))
		if err != nil {
			return fmt.Errorf("failed to calculate badge cache size: %w", err)
		}
		badgeStats := imageStats{
			SizeBytes: badgeSizeBytes,
			Images:    badgeImages,
			Frames:    badgeFrames,
		}

		// Collect message stats from database
		rows, err := db.QueryContext(ctx, "SELECT broadcast_channel, COUNT(*) as count FROM messages GROUP BY broadcast_channel ORDER BY count DESC")
		if err != nil {
			return fmt.Errorf("failed to query database: %w", err)
		}
		defer rows.Close()

		var channels []channelMessageCount
		var totalMessages int64
		for rows.Next() {
			var ch channelMessageCount
			if err := rows.Scan(&ch.Channel, &ch.Count); err != nil {
				return fmt.Errorf("failed to scan row: %w", err)
			}
			totalMessages += ch.Count
			channels = append(channels, ch)
		}

		// Render and print the styled output
		fmt.Println(renderCacheOutput(emoteStats, badgeStats, channels, totalMessages))

		return nil
	},
}

func statsForImageDirectory(path string) (int64, int, int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, 0, nil
		}

		return 0, 0, 0, fmt.Errorf("failed to read emote directory %s: %w", path, err)
	}

	return totalFileSize(entries)
}

func totalFileSize(entries []os.DirEntry) (int64, int, int, error) {
	var (
		totalSize        int64
		totalImageFrames int
		totalImages      int
	)

	for _, e := range entries {
		if e.IsDir() {
			continue // we don't expect any subdirs here
		}

		info, err := e.Info()
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to get info for %s: %w", e.Name(), err)
		}

		if filepath.Ext(e.Name()) == ".json" {
			totalImages++
		} else {
			totalImageFrames++
		}

		totalSize += info.Size()
	}

	return totalSize, totalImages, totalImageFrames, nil
}
