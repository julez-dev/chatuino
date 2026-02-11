package save

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/spf13/afero"
)

const (
	channelHistoryFileName = "channel_history.json"
	maxChannelHistory      = 50
)

// ChannelHistoryEntry records a single visited channel with timestamp.
type ChannelHistoryEntry struct {
	ChannelLogin string    `json:"channel_login"`
	VisitedAt    time.Time `json:"visited_at"`
}

// ChannelHistoryManager persists recently visited channels to a JSON file.
type ChannelHistoryManager struct {
	fs afero.Fs
}

func NewChannelHistoryManager(fs afero.Fs) *ChannelHistoryManager {
	return &ChannelHistoryManager{fs: fs}
}

// LoadHistory reads the channel history from disk, sorted by most-recent first.
func (m *ChannelHistoryManager) LoadHistory() ([]ChannelHistoryEntry, error) {
	f, err := openCreateDataFile(m.fs, channelHistoryFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	var entries []ChannelHistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		syntaxErr := &json.SyntaxError{}
		if errors.As(err, &syntaxErr) {
			return nil, nil
		}
		return nil, err
	}

	sortHistoryDesc(entries)
	return entries, nil
}

// RecordChannel upserts a channel into the history, updating its timestamp.
// The history is capped at maxChannelHistory entries.
func (m *ChannelHistoryManager) RecordChannel(login string) error {
	f, err := openCreateDataFile(m.fs, channelHistoryFileName)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	var entries []ChannelHistoryEntry
	if len(data) > 0 {
		if err := json.Unmarshal(data, &entries); err != nil {
			syntaxErr := &json.SyntaxError{}
			if !errors.As(err, &syntaxErr) {
				return err
			}
			// corrupted file, start fresh
			entries = nil
		}
	}

	now := time.Now()
	found := false
	for i := range entries {
		if entries[i].ChannelLogin == login {
			entries[i].VisitedAt = now
			found = true
			break
		}
	}

	if !found {
		entries = append(entries, ChannelHistoryEntry{
			ChannelLogin: login,
			VisitedAt:    now,
		})
	}

	sortHistoryDesc(entries)

	// prune to cap
	if len(entries) > maxChannelHistory {
		entries = entries[:maxChannelHistory]
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	if err := f.Truncate(0); err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = io.Copy(f, bytes.NewReader(out))
	return err
}

func sortHistoryDesc(entries []ChannelHistoryEntry) {
	slices.SortFunc(entries, func(a, b ChannelHistoryEntry) int {
		// descending: newer first
		return b.VisitedAt.Compare(a.VisitedAt)
	})
}
