package deploy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDockerfileGoVersionMatchesModule(t *testing.T) {
	moduleGoVersion := readModuleGoVersion(t, filepath.Join("..", "go.mod"))
	dockerfile := readFile(t, filepath.Join("..", "Dockerfile"))

	want := "FROM golang:" + moduleGoVersion + "-alpine AS builder"
	if !strings.Contains(dockerfile, want) {
		t.Fatalf("Dockerfile builder image must match go.mod version; want %q", want)
	}
}

func TestBackendImageIncludesComposeHealthcheckCommand(t *testing.T) {
	dockerfile := readFile(t, filepath.Join("..", "Dockerfile"))
	compose := readFile(t, "docker-compose.cloud.yml")

	if strings.Contains(compose, `["CMD", "curl",`) && !regexp.MustCompile(`apk\s+--no-cache\s+add[^\n]*curl`).MatchString(dockerfile) {
		t.Fatalf("backend compose healthcheck uses curl, but runtime Dockerfile does not install curl")
	}
}

func TestBackendComposeUsesReadinessHealthcheck(t *testing.T) {
	compose := readFile(t, "docker-compose.cloud.yml")

	if !strings.Contains(compose, "http://localhost:8080/api/health/ready") {
		t.Fatalf("backend healthcheck must use readiness endpoint, not static liveness")
	}
}

func readModuleGoVersion(t *testing.T, path string) string {
	t.Helper()
	content := readFile(t, path)
	re := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+)\s*$`)
	match := re.FindStringSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("could not find go version in %s", path)
	}
	return match[1]
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
