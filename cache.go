package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/urfave/cli/v3"
)

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
				if c.Bool("emotes") {
					if err := os.RemoveAll(filepath.Join(kittyimg.BaseImageDirectory, "emote")); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete emote cache: %w", err)
					}
					fmt.Print("badge cache directory deleted\n")
				}

				if c.Bool("badges") {
					if err := os.RemoveAll(filepath.Join(kittyimg.BaseImageDirectory, "badge")); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete badge cache: %w", err)
					}
					fmt.Print("badge cache directory deleted\n")
				}

				if c.Bool("database") {
					if err := os.Remove(dbFileName); err != nil && !errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("failed to delete database cache: %w", err)
					}
					fmt.Print("database file deleted\n")
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

		emoteTotalSizeBytes, emoteTotalImages, emoteTotalImageFrames, err := statsForImageDirectory(filepath.Join(kittyimg.BaseImageDirectory, "emote"))
		if err != nil {
			return fmt.Errorf("failed to calculate total file size: %w", err)
		}

		fmt.Printf("Emote cache total size: %s\n", humanize.Bytes(uint64(emoteTotalSizeBytes)))
		fmt.Printf("Emote cache total images: %d\n", emoteTotalImages)
		fmt.Printf("Emote cache total image frames: %d\n\n", emoteTotalImageFrames)

		badgeTotalSizeBytes, badgeTotalImages, badgeTotalImageFrames, err := statsForImageDirectory(filepath.Join(kittyimg.BaseImageDirectory, "badge"))
		if err != nil {
			return fmt.Errorf("failed to calculate total file size: %w", err)
		}

		fmt.Printf("Badge cache total size: %s\n", humanize.Bytes(uint64(badgeTotalSizeBytes)))
		fmt.Printf("Badge cache total images: %d\n", badgeTotalImages)
		fmt.Printf("Badge cache total image frames: %d\n\n", badgeTotalImageFrames)

		rows, err := db.QueryContext(ctx, "SELECT broadcast_channel, COUNT(*) as count FROM messages GROUP BY broadcast_channel ORDER BY count DESC")
		if err != nil {
			return fmt.Errorf("failed to query database: %w", err)
		}
		defer rows.Close()

		type row struct {
			Channel string
			Count   int64
		}

		var result []row
		var totalMessages int64
		for rows.Next() {
			var row row

			if err := rows.Scan(&row.Channel, &row.Count); err != nil {
				return fmt.Errorf("failed to scan row: %w", err)
			}

			totalMessages += row.Count

			result = append(result, row)
		}

		fmt.Printf("Total number of messags in database: %s\n", humanize.Comma(totalMessages))
		for _, r := range result {
			fmt.Printf("Number of messags for %s: %s\n", r.Channel, humanize.Comma(r.Count))
		}

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
