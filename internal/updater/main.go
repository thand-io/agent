package updater

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/go-github/v57/github"
	"github.com/inconshreveable/go-update"
	"github.com/sirupsen/logrus"
)

type Updater struct {
	owner   string
	repo    string
	current string
	client  *github.Client
}

func NewUpdater(owner, repo, currentVersion string) *Updater {
	return &Updater{
		owner:   owner,
		repo:    repo,
		current: currentVersion,
		client:  github.NewClient(nil),
	}
}

func (u *Updater) CheckForUpdate(ctx context.Context) (*github.RepositoryRelease, error) {
	release, _, err := u.client.Repositories.GetLatestRelease(ctx, u.owner, u.repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	if release.GetTagName() == u.current {
		return nil, nil // No update available
	}

	return release, nil
}

func (u *Updater) Update(ctx context.Context, release *github.RepositoryRelease) error {
	assetName := fmt.Sprintf("%s-%s-%s", u.repo, runtime.GOOS, runtime.GOARCH)

	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.GetName(), assetName) {
			downloadURL = asset.GetBrowserDownloadURL()
			break
		}
	}

	if len(downloadURL) == 0 {
		return fmt.Errorf("no suitable asset found for %s", assetName)
	}

	client := resty.New()
	resp, err := client.R().
		SetContext(ctx).
		Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	err = update.Apply(bytes.NewReader(resp.Body()), update.Options{})
	if err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	logrus.Infof("Successfully updated to version %s", release.GetTagName())
	return nil
}

func (u *Updater) AutoUpdate(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			release, err := u.CheckForUpdate(ctx)
			if err != nil {
				logrus.Errorf("Update check failed: %v", err)
				continue
			}

			if release != nil {
				logrus.Infof("New version available: %s", release.GetTagName())
				if err := u.Update(ctx, release); err != nil {
					logrus.Errorf("Update failed: %v", err)
				}
			}
		}
	}
}
