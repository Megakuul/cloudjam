package aws

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const fakeCloudRepo = "faiscadev/fakecloud"

// resolveFakeCloudVersion returns the release tag to install. "" or "latest" asks github for the latest release.
func resolveFakeCloudVersion(ctx context.Context, version string) (string, error) {
	if version != "" && version != "latest" {
		return version, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", fakeCloudRepo), nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch latest fakecloud release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch latest fakecloud release: unexpected status %d", response.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse latest fakecloud release: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("could not determine latest fakecloud version")
	}
	return release.TagName, nil
}

// platformSuffix returns the fakecloud OS/arch (matching for fakeclouds release asset).
func platformSuffix() (string, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("unsupported OS for fakecloud: %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture for fakecloud: %s", runtime.GOARCH)
	}
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), nil
}

// fakeCloudBinary returns the path to a native fakecloud binary for the given
// version, downloading and caching it under the user cache dir if it is not there already.
func fakeCloudBinary(ctx context.Context, version string) (string, error) {
	version, err := resolveFakeCloudVersion(ctx, version)
	if err != nil {
		return "", err
	}
	platform, err := platformSuffix()
	if err != nil {
		return "", err
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache directory: %w", err)
	}
	installDir := filepath.Join(cacheDir, "cloudjam", "fakecloud", version)
	binaryPath := filepath.Join(installDir, "fakecloud")

	if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() {
		return binaryPath, nil
	}

	slog.Info("downloading fakecloud", "version", version, "platform", platform)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create fakecloud cache directory: %w", err)
	}

	tarball := fmt.Sprintf("fakecloud-%s-%s.tar.gz", version, platform)
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", fakeCloudRepo, version)

	archiveBytes, err := download(ctx, baseURL+"/"+tarball)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", tarball, err)
	}
	checksumBytes, err := download(ctx, baseURL+"/"+tarball+".sha256")
	if err != nil {
		return "", fmt.Errorf("download %s.sha256: %w", tarball, err)
	}
	if err := verifyChecksum(archiveBytes, checksumBytes); err != nil {
		return "", fmt.Errorf("verify %s: %w", tarball, err)
	}
	if err := extractBinary(archiveBytes, "fakecloud", binaryPath); err != nil {
		return "", fmt.Errorf("extract %s: %w", tarball, err)
	}
	slog.Info("fakecloud installed", "path", binaryPath)
	return binaryPath, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func verifyChecksum(archive, checksumFile []byte) error {
	fields := strings.Fields(string(checksumFile))
	if len(fields) == 0 {
		return fmt.Errorf("empty checksum file")
	}
	expected := fields[0]
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func extractBinary(archive []byte, name, destination string) error {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return err
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return fmt.Errorf("%s not found in archive", name)
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) != name || header.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, reader); err != nil {
			return err
		}
		return nil
	}
}

func startFakeCloud(ctx context.Context, version string, port int) (*exec.Cmd, error) {
	binary, err := fakeCloudBinary(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("start fakecloud: %w", err)
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
	connectAddr := fmt.Sprintf("127.0.0.1:%d", port)
	slog.Info("starting fakecloud", "binary", binary, "addr", listenAddr)

	cmd := exec.Command(binary, "--addr", listenAddr, "--iam", "strict")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start fakecloud: %w", err)
	}

	health := fmt.Sprintf("http://%s/_fakecloud/health", connectAddr)
	deadline := time.Now().Add(90 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, health, nil)
		if err != nil {
			stopFakeCloud(cmd)
			return nil, err
		}
		if response, err := http.DefaultClient.Do(request); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				slog.Info("fakecloud ready", "pid", cmd.Process.Pid)
				return cmd, nil
			}
		}
		if time.Now().After(deadline) {
			stopFakeCloud(cmd)
			return nil, fmt.Errorf("fakecloud did not become ready (pid %d)", cmd.Process.Pid)
		}
		select {
		case <-ctx.Done():
			stopFakeCloud(cmd)
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func stopFakeCloud(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	slog.Info("stopping fakecloud", "pid", cmd.Process.Pid)
	if err := cmd.Process.Kill(); err != nil {
		slog.Error("failed to stop fakecloud", "error", err.Error())
		return
	}
	_ = cmd.Wait()
}
