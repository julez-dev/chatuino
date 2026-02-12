package mainui

import (
	"context"
	"fmt"

	"golang.org/x/mod/semver"
)

// UpdateInfo holds the result of a version check.
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
}

// latestVersionFetcher is the subset of ChatuinoServer needed for version checks.
type latestVersionFetcher interface {
	GetLatestVersion(ctx context.Context) (string, error)
}

// checkForUpdate queries the server for the latest version and compares it
// against the current build version. Returns a zero UpdateInfo when the
// current version is a dev build or the fetcher is nil.
func checkForUpdate(ctx context.Context, checker latestVersionFetcher, currentVersion string) (UpdateInfo, error) {
	if checker == nil || currentVersion == "dev" {
		return UpdateInfo{}, nil
	}

	current := ensureVPrefix(currentVersion)
	if !semver.IsValid(current) {
		return UpdateInfo{}, fmt.Errorf("current version %q is not valid semver", currentVersion)
	}

	latest, err := checker.GetLatestVersion(ctx)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("fetch latest version: %w", err)
	}

	latestCanonical := ensureVPrefix(latest)
	if !semver.IsValid(latestCanonical) {
		return UpdateInfo{}, fmt.Errorf("server version %q is not valid semver", latest)
	}

	return UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latestCanonical,
		HasUpdate:      semver.Compare(latestCanonical, current) > 0,
	}, nil
}

func ensureVPrefix(v string) string {
	if len(v) > 0 && v[0] != 'v' {
		return "v" + v
	}
	return v
}
