package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// startFakeCloud runs the container and waits for it to answer its health endpoint. It returns the container id.
func startFakeCloud(ctx context.Context, image string, port int) (string, error) {
	args := []string{
		"run", "--detach", "--rm", "-e", "FAKECLOUD_IAM=strict",
		"--publish", fmt.Sprintf("127.0.0.1:%d:4566", port), image,
	}

	slog.Info("starting fakecloud", "image", image, "port", port)
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return "", fmt.Errorf("start fakecloud: %w\n%s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("start fakecloud: %w", err)
	}
	container := strings.TrimSpace(string(out))

	health := fmt.Sprintf("http://127.0.0.1:%d/_fakecloud/health", port)
	deadline := time.Now().Add(90 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, health, nil)
		if err != nil {
			return "", err
		}
		if response, err := http.DefaultClient.Do(request); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				slog.Info("fakecloud ready", "container", container[:12])
				return container, nil
			}
		}
		if time.Now().After(deadline) {
			removeFakeCloud(container)
			return "", fmt.Errorf("fakecloud did not become ready (docker logs %s)", container[:12])
		}
		select {
		case <-ctx.Done():
			removeFakeCloud(container)
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func removeFakeCloud(container string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("removing fakecloud", "container", container[:12])
	if out, err := exec.CommandContext(ctx, "docker", "rm", "--force", container).CombinedOutput(); err != nil {
		slog.Error("failed to remove fakecloud", "error", strings.TrimSpace(string(out)))
	}
}
