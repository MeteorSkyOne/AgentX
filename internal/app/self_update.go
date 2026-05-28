package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/config"
	"github.com/meteorsky/agentx/internal/version"
	"github.com/robfig/cron/v3"
)

const (
	selfUpdateChannelRelease   = "release"
	selfUpdateChannelDev       = "dev"
	defaultSelfUpdateTimeout   = 10 * time.Minute
	defaultSelfUpdateRepo      = "MeteorSkyOne/AgentX"
	selfUpdateUserAgent        = "AgentX/" + "self-update"
	selfUpdateStateIdle        = "idle"
	selfUpdateStateChecking    = "checking"
	selfUpdateStateDownloading = "downloading"
	selfUpdateStateReplacing   = "replacing"
	selfUpdateStateError       = "error"
)

type SelfUpdateOptions struct {
	Settings   config.SelfUpdateSettings
	GitHubRepo string
	HTTPClient *http.Client
	Now        func() time.Time
	Executable func() (string, error)
}

type SelfUpdateSettings = config.SelfUpdateSettings

type SelfUpdateOverview struct {
	Settings config.SelfUpdateSettings `json:"settings"`
	Status   SelfUpdateStatus          `json:"status"`
}

type SelfUpdateStatus struct {
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version,omitempty"`
	UpdateAvailable *bool      `json:"update_available,omitempty"`
	State           string     `json:"state"`
	Message         string     `json:"message,omitempty"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty"`
	LastUpdatedAt   *time.Time `json:"last_updated_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	RestartRequired bool       `json:"restart_required"`
	Channel         string     `json:"channel"`
}

type selfUpdateService struct {
	dataDir    string
	settings   config.SelfUpdateSettings
	githubRepo string
	client     *http.Client
	now        func() time.Time
	executable func() (string, error)

	mu     sync.Mutex
	state  selfUpdateState
	latest *githubRelease
	cron   *cron.Cron
}

type selfUpdateState struct {
	LatestVersion   string
	UpdateAvailable *bool
	State           string
	Message         string
	LastCheckedAt   *time.Time
	LastUpdatedAt   *time.Time
	LastError       string
	RestartRequired bool
}

type githubRelease struct {
	TagName         string               `json:"tag_name"`
	Name            string               `json:"name"`
	Draft           bool                 `json:"draft"`
	Prerelease      bool                 `json:"prerelease"`
	TargetCommitish string               `json:"target_commitish"`
	Assets          []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func newSelfUpdateService(dataDir string, opts SelfUpdateOptions) *selfUpdateService {
	settings := opts.Settings
	if normalized, err := config.NormalizeSelfUpdateSettings(settings, "self update settings"); err == nil {
		settings = normalized
	} else {
		settings = config.DefaultSelfUpdateSettings()
	}
	repo := strings.TrimSpace(opts.GitHubRepo)
	if repo == "" {
		repo = defaultSelfUpdateRepo
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultSelfUpdateTimeout}
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	executable := opts.Executable
	if executable == nil {
		executable = os.Executable
	}
	return &selfUpdateService{
		dataDir:    dataDir,
		settings:   settings,
		githubRepo: repo,
		client:     client,
		now:        now,
		executable: executable,
		state:      selfUpdateState{State: selfUpdateStateIdle},
	}
}

func (a *App) StartSelfUpdate(ctx context.Context) error {
	if a.selfUpdates == nil {
		return nil
	}
	return a.selfUpdates.start(ctx, func() {
		a.startBackground("self-auto-update", func(ctx context.Context) {
			if _, err := a.RunSelfUpdate(ctx); err != nil {
				slog.Warn("self auto update failed", "error", err)
			}
		})
	})
}

func (a *App) StopSelfUpdate() {
	if a.selfUpdates != nil {
		a.selfUpdates.stop()
	}
}

func (s *selfUpdateService) start(_ context.Context, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		return nil
	}
	if !s.settings.AutoEnabled {
		return nil
	}
	scheduler := cron.New(cron.WithParser(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow)), cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cronSlogLogger{})))
	spec := selfUpdateCronSpec(s.settings)
	if _, err := scheduler.AddFunc(spec, fn); err != nil {
		return err
	}
	scheduler.Start()
	s.cron = scheduler
	return nil
}

func (s *selfUpdateService) stop() {
	s.mu.Lock()
	scheduler := s.cron
	s.cron = nil
	s.mu.Unlock()
	if scheduler != nil {
		ctx := scheduler.Stop()
		<-ctx.Done()
	}
}

func (s *selfUpdateService) restart(ctx context.Context, fn func()) error {
	s.stop()
	return s.start(ctx, fn)
}

func selfUpdateCronSpec(settings config.SelfUpdateSettings) string {
	parts := strings.Split(settings.TimeOfDay, ":")
	timezone := strings.TrimSpace(settings.Timezone)
	if timezone == "" || timezone == "Local" {
		timezone = "UTC"
	}
	return fmt.Sprintf("CRON_TZ=%s %s %s * * *", timezone, parts[1], parts[0])
}

func (a *App) SelfUpdateOverview(_ context.Context, _ string) (SelfUpdateOverview, error) {
	if a.selfUpdates == nil {
		return SelfUpdateOverview{}, nil
	}
	return a.selfUpdates.overview(), nil
}

func (a *App) UpdateSelfUpdateSettings(ctx context.Context, _ string, settings config.SelfUpdateSettings) (SelfUpdateOverview, error) {
	saved, err := config.SaveSelfUpdateSettings(a.opts.DataDir, settings)
	if err != nil {
		return SelfUpdateOverview{}, invalidInput(err.Error())
	}
	a.selfUpdates.mu.Lock()
	a.selfUpdates.settings = saved
	a.selfUpdates.mu.Unlock()
	if err := a.selfUpdates.restart(ctx, func() {
		a.startBackground("self-auto-update", func(ctx context.Context) {
			if _, err := a.RunSelfUpdate(ctx); err != nil {
				slog.Warn("self auto update failed", "error", err)
			}
		})
	}); err != nil {
		return SelfUpdateOverview{}, invalidInput(err.Error())
	}
	return a.SelfUpdateOverview(ctx, "")
}

func (a *App) CheckSelfUpdate(ctx context.Context) (SelfUpdateOverview, error) {
	if a.selfUpdates == nil {
		return SelfUpdateOverview{}, nil
	}
	if err := a.selfUpdates.check(ctx); err != nil {
		return SelfUpdateOverview{}, err
	}
	return a.SelfUpdateOverview(ctx, "")
}

func (a *App) RunSelfUpdate(ctx context.Context) (SelfUpdateOverview, error) {
	if a.selfUpdates == nil {
		return SelfUpdateOverview{}, nil
	}
	if err := a.selfUpdates.check(ctx); err != nil {
		return SelfUpdateOverview{}, err
	}
	if err := a.selfUpdates.update(ctx); err != nil {
		return SelfUpdateOverview{}, err
	}
	return a.SelfUpdateOverview(ctx, "")
}

func (a *App) StartRunSelfUpdate(ctx context.Context) (SelfUpdateOverview, error) {
	if a.selfUpdates == nil {
		return SelfUpdateOverview{}, nil
	}
	if a.selfUpdates.beginUpdate() {
		a.startBackground("self-manual-update", func(ctx context.Context) {
			if err := a.selfUpdates.checkStarted(ctx, "downloading"); err != nil {
				slog.Warn("self update check failed", "error", err)
				return
			}
			if err := a.selfUpdates.updateStarted(ctx); err != nil {
				slog.Warn("self update failed", "error", err)
			}
		})
	}
	return a.SelfUpdateOverview(ctx, "")
}

func (s *selfUpdateService) overview() SelfUpdateOverview {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SelfUpdateOverview{
		Settings: s.settings,
		Status: SelfUpdateStatus{
			CurrentVersion:  version.Short(),
			LatestVersion:   s.state.LatestVersion,
			UpdateAvailable: cloneBoolPtr(s.state.UpdateAvailable),
			State:           s.state.State,
			Message:         s.state.Message,
			LastCheckedAt:   cloneTimePtr(s.state.LastCheckedAt),
			LastUpdatedAt:   cloneTimePtr(s.state.LastUpdatedAt),
			LastError:       s.state.LastError,
			RestartRequired: s.state.RestartRequired,
			Channel:         s.settings.Channel,
		},
	}
}

func (s *selfUpdateService) check(ctx context.Context) error {
	s.setState(selfUpdateStateChecking, "", "")
	return s.checkStarted(ctx, selfUpdateStateIdle)
}

func (s *selfUpdateService) checkStarted(ctx context.Context, completedState string) error {
	settings := s.snapshotSettings()
	release, err := s.fetchTargetRelease(ctx, settings.Channel)
	now := s.now().UTC()
	if err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	latest := releaseVersion(settings.Channel, release)
	available := selfUpdateAvailable(settings.Channel, latest, release.TargetCommitish)
	s.mu.Lock()
	s.latest = release
	s.state.LatestVersion = latest
	s.state.UpdateAvailable = &available
	s.state.State = completedState
	s.state.Message = ""
	s.state.LastCheckedAt = &now
	s.state.LastError = ""
	s.mu.Unlock()
	return nil
}

func (s *selfUpdateService) update(ctx context.Context) error {
	s.setState(selfUpdateStateDownloading, "", "")
	return s.updateStarted(ctx)
}

func (s *selfUpdateService) updateStarted(ctx context.Context) error {
	s.mu.Lock()
	release := s.latest
	s.mu.Unlock()
	if release == nil {
		if err := s.checkStarted(ctx, selfUpdateStateDownloading); err != nil {
			return err
		}
		s.mu.Lock()
		release = s.latest
		s.mu.Unlock()
	}
	if release == nil {
		err := errors.New("no release available")
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	asset, checksumAsset, err := releaseAssetsForPlatform(release)
	if err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	tempDir, err := os.MkdirTemp("", "agentx-self-update-*")
	if err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, asset.Name)
	checksumPath := filepath.Join(tempDir, checksumAsset.Name)
	if err := s.downloadFile(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	if err := s.downloadFile(ctx, checksumAsset.BrowserDownloadURL, checksumPath); err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	if err := verifyArchiveChecksum(archivePath, checksumPath, asset.Name); err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	extractedPath := filepath.Join(tempDir, executableName())
	if err := extractAgentXBinary(archivePath, extractedPath); err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	s.setState(selfUpdateStateReplacing, "", "")
	if err := s.replaceExecutable(extractedPath); err != nil {
		s.setState(selfUpdateStateError, "", err.Error())
		return err
	}
	now := s.now().UTC()
	s.mu.Lock()
	st := &s.state
	st.State = selfUpdateStateIdle
	st.Message = "Updated. Restart AgentX to use the new version."
	st.LastUpdatedAt = &now
	st.LastError = ""
	st.RestartRequired = true
	v := false
	st.UpdateAvailable = &v
	if st.LatestVersion != "" {
		// The running process still reports the old embedded version until restart.
		st.Message = "Installed " + st.LatestVersion + ". Restart AgentX to use it."
	}
	s.mu.Unlock()
	return nil
}

func (s *selfUpdateService) beginUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.State == selfUpdateStateChecking || s.state.State == selfUpdateStateDownloading || s.state.State == selfUpdateStateReplacing {
		return false
	}
	s.state.State = selfUpdateStateDownloading
	s.state.Message = ""
	s.state.LastError = ""
	return true
}

func (s *selfUpdateService) snapshotSettings() config.SelfUpdateSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *selfUpdateService) setState(state string, message string, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.State = state
	s.state.Message = message
	s.state.LastError = lastErr
}

func (s *selfUpdateService) fetchTargetRelease(ctx context.Context, channel string) (*githubRelease, error) {
	releases, err := s.fetchReleases(ctx)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		release := &releases[i]
		if release.Draft {
			continue
		}
		switch channel {
		case selfUpdateChannelDev:
			if release.TagName == "dev" {
				return release, nil
			}
		default:
			if !release.Prerelease {
				return release, nil
			}
		}
	}
	return nil, fmt.Errorf("no %s release found", channel)
}

func (s *selfUpdateService) fetchReleases(ctx context.Context) ([]githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", strings.Trim(s.githubRepo, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", selfUpdateUserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub releases request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func (s *selfUpdateService) downloadFile(ctx context.Context, url string, path string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("release asset has no download URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", selfUpdateUserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download %s failed: %s: %s", filepath.Base(path), resp.Status, strings.TrimSpace(string(body)))
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (s *selfUpdateService) replaceExecutable(newBinary string) error {
	current, err := s.executable()
	if err != nil {
		return err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return err
	}
	info, err := os.Stat(current)
	if err != nil {
		return err
	}
	if err := os.Chmod(newBinary, info.Mode().Perm()|0o700); err != nil {
		return err
	}
	backup := current + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(newBinary, current); err != nil {
		_ = os.Rename(backup, current)
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

func releaseVersion(channel string, release *githubRelease) string {
	if release == nil {
		return ""
	}
	if channel == selfUpdateChannelDev {
		name := normalizeReleaseTag(release.Name)
		if looksLikeVersion(name) {
			return name
		}
		target := strings.TrimSpace(release.TargetCommitish)
		if target != "" {
			if len(target) > 12 {
				target = target[:12]
			}
			return "dev+" + target
		}
		return "dev"
	}
	return normalizeReleaseTag(release.TagName)
}

func selfUpdateAvailable(channel string, latest string, target string) bool {
	if channel == selfUpdateChannelDev {
		if looksLikeVersion(latest) {
			current := normalizeReleaseTag(version.Short())
			return latest != "" && current != latest
		}
		commit := strings.TrimSpace(version.Commit)
		target = strings.TrimSpace(target)
		if commit != "" && commit != "unknown" && target != "" {
			return !strings.HasPrefix(target, commit) && !strings.HasPrefix(commit, target)
		}
		return latest != "" && latest != version.Short()
	}
	current := normalizeReleaseTag(version.Short())
	return latest != "" && current != latest
}

func looksLikeVersion(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" || v[0] < '0' || v[0] > '9' {
		return false
	}
	return strings.ContainsRune(v, '.')
}

func normalizeReleaseTag(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.Index(v, "-dirty"); i >= 0 {
		v = v[:i]
	}
	return v
}

func releaseAssetsForPlatform(release *githubRelease) (githubReleaseAsset, githubReleaseAsset, error) {
	target := fmt.Sprintf("agentx-%s-%s", goruntime.GOOS, goruntime.GOARCH)
	names := []string{target + ".tar.gz", target + ".zip"}
	var asset githubReleaseAsset
	var checksum githubReleaseAsset
	for _, item := range release.Assets {
		if item.Name == "SHA256SUMS" {
			checksum = item
			continue
		}
		for _, name := range names {
			if item.Name == name {
				asset = item
			}
		}
	}
	if asset.Name == "" {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release %s has no asset for %s/%s", release.TagName, goruntime.GOOS, goruntime.GOARCH)
	}
	if checksum.Name == "" {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release %s has no SHA256SUMS asset", release.TagName)
	}
	return asset, checksum, nil
}

func verifyArchiveChecksum(archivePath string, checksumPath string, assetName string) error {
	want, err := checksumForAsset(checksumPath, assetName)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func checksumForAsset(path string, assetName string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", assetName)
}

func extractAgentXBinary(archivePath string, outPath string) error {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractAgentXTarGz(archivePath, outPath)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractAgentXZip(archivePath, outPath)
	default:
		return fmt.Errorf("unsupported archive %s", filepath.Base(archivePath))
	}
}

func extractAgentXTarGz(archivePath string, outPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != executableName() {
			continue
		}
		return writeExtractedBinary(outPath, reader, os.FileMode(header.Mode).Perm())
	}
	return errors.New("archive does not contain agentx binary")
}

func extractAgentXZip(archivePath string, outPath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != executableName() {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		err = writeExtractedBinary(outPath, src, file.Mode().Perm())
		closeErr := src.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return errors.New("archive does not contain agentx binary")
}

func writeExtractedBinary(outPath string, src io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o755
	}
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func executableName() string {
	if goruntime.GOOS == "windows" {
		return "agentx.exe"
	}
	return "agentx"
}
