package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/meteorsky/agentx/internal/version"
)

func TestReleaseVersion(t *testing.T) {
	if got := releaseVersion(selfUpdateChannelRelease, &githubRelease{TagName: "v1.2.3"}); got != "1.2.3" {
		t.Fatalf("releaseVersion(release) = %q, want 1.2.3", got)
	}

	got := releaseVersion(selfUpdateChannelDev, &githubRelease{TargetCommitish: "abcdef1234567890"})
	if got != "dev+abcdef123456" {
		t.Fatalf("releaseVersion(dev) = %q, want dev+abcdef123456", got)
	}

	if got := releaseVersion(selfUpdateChannelDev, &githubRelease{}); got != "dev" {
		t.Fatalf("releaseVersion(dev empty target) = %q, want dev", got)
	}
}

func TestNewSelfUpdateServiceUsesReleaseRepository(t *testing.T) {
	service := newSelfUpdateService(t.TempDir(), SelfUpdateOptions{})
	if service.githubRepo != "MeteorSkyOne/AgentX" {
		t.Fatalf("githubRepo = %q, want MeteorSkyOne/AgentX", service.githubRepo)
	}
}

func TestNormalizeReleaseTag(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "v1.2.3", want: "1.2.3"},
		{in: " 1.2.3 ", want: "1.2.3"},
		{in: "1.2.3-dirty", want: "1.2.3"},
		{in: "v1.2.3-dirty", want: "1.2.3"},
	} {
		if got := normalizeReleaseTag(tc.in); got != tc.want {
			t.Fatalf("normalizeReleaseTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSelfUpdateAvailableRelease(t *testing.T) {
	restoreAppVersionVars(t)

	version.Version = "1.2.3"
	if selfUpdateAvailable(selfUpdateChannelRelease, "1.2.3", "") {
		t.Fatal("selfUpdateAvailable() = true, want false for matching release")
	}

	version.Version = "1.2.3-dirty"
	if selfUpdateAvailable(selfUpdateChannelRelease, "1.2.3", "") {
		t.Fatal("selfUpdateAvailable() = true, want false for dirty matching release")
	}

	version.Version = "1.2.2"
	if !selfUpdateAvailable(selfUpdateChannelRelease, "1.2.3", "") {
		t.Fatal("selfUpdateAvailable() = false, want true for newer release")
	}
}

func TestSelfUpdateAvailableDev(t *testing.T) {
	restoreAppVersionVars(t)

	version.Version = "0.1.0-dev.3"
	version.Commit = "abcdef1"
	if selfUpdateAvailable(selfUpdateChannelDev, "dev+abcdef123456", "abcdef1234567890") {
		t.Fatal("selfUpdateAvailable() = true, want false for matching dev commit prefix")
	}
	if !selfUpdateAvailable(selfUpdateChannelDev, "dev+123456789abc", "123456789abcdef0") {
		t.Fatal("selfUpdateAvailable() = false, want true for different dev commit")
	}

	version.Commit = "unknown"
	if !selfUpdateAvailable(selfUpdateChannelDev, "dev+fedcba987654", "fedcba9876543210") {
		t.Fatal("selfUpdateAvailable() = false, want true when commit is unknown and latest label differs")
	}
}

func TestReleaseAssetsForPlatform(t *testing.T) {
	archiveName := "agentx-" + runtimeTarget() + ".tar.gz"
	release := &githubRelease{
		TagName: "v1.2.3",
		Assets: []githubReleaseAsset{
			{Name: "agentx-other-other.tar.gz"},
			{Name: archiveName},
			{Name: "SHA256SUMS"},
		},
	}
	asset, checksum, err := releaseAssetsForPlatform(release)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != archiveName || checksum.Name != "SHA256SUMS" {
		t.Fatalf("releaseAssetsForPlatform() asset=%q checksum=%q", asset.Name, checksum.Name)
	}
}

func TestChecksumForAssetAndVerifyArchiveChecksum(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "agentx-test.tar.gz")
	body := []byte("archive")
	if err := os.WriteFile(archivePath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	checksumPath := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(checksumPath, []byte(fmt.Sprintf("%x  %s\nbad  other-file\n", sum, filepath.Base(archivePath))), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := checksumForAsset(checksumPath, filepath.Base(archivePath))
	if err != nil {
		t.Fatal(err)
	}
	if got != fmt.Sprintf("%x", sum) {
		t.Fatalf("checksumForAsset() = %q, want %x", got, sum)
	}
	if err := verifyArchiveChecksum(archivePath, checksumPath, filepath.Base(archivePath)); err != nil {
		t.Fatalf("verifyArchiveChecksum() error = %v", err)
	}

	if err := os.WriteFile(checksumPath, []byte("0000  agentx-test.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveChecksum(archivePath, checksumPath, filepath.Base(archivePath)); err == nil {
		t.Fatal("verifyArchiveChecksum() error = nil, want mismatch")
	}
}

func TestExtractAgentXTarGz(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "agentx-test.tar.gz")
	if err := writeTarGzArchive(archivePath, executableName(), []byte("binary")); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, executableName())
	if err := extractAgentXBinary(archivePath, outPath); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, outPath, "binary")
}

func TestExtractAgentXZip(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "agentx-test.zip")
	if err := writeZipArchive(archivePath, executableName(), []byte("binary")); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, executableName())
	if err := extractAgentXBinary(archivePath, outPath); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, outPath, "binary")
}

func TestExtractAgentXBinaryRejectsMissingBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "agentx-test.tar.gz")
	if err := writeTarGzArchive(archivePath, "not-agentx", []byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := extractAgentXBinary(archivePath, filepath.Join(dir, executableName())); err == nil {
		t.Fatal("extractAgentXBinary() error = nil, want missing binary error")
	}
}

func restoreAppVersionVars(t *testing.T) {
	t.Helper()
	oldVersion := version.Version
	oldCommit := version.Commit
	oldDate := version.Date
	t.Cleanup(func() {
		version.Version = oldVersion
		version.Commit = oldCommit
		version.Date = oldDate
	})
}

func runtimeTarget() string {
	return fmt.Sprintf("%s-%s", goruntime.GOOS, goruntime.GOARCH)
}

func writeTarGzArchive(path string, name string, body []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	writer := tar.NewWriter(gz)
	defer writer.Close()
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func writeZipArchive(path string, name string, body []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	defer writer.Close()
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(body)
	return err
}

func assertFileBody(t *testing.T, path string, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("%s body = %q, want %q", path, string(body), want)
	}
}
