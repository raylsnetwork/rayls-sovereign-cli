package stacks

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

// Registry replies we must recognise as throttling (worth waiting out) versus
// failures a retry can't fix.
func TestIsRateLimited(t *testing.T) {
	rateLimited := []string{
		// What ECR Public answered when `up` pulled the infra images at once.
		"toomanyrequests: Rate exceeded",
		" ! Image public.ecr.aws/w0k9o1t3/rayls-demo/rayls-nats:latest  Interrupted\ntoomanyrequests: Rate exceeded",
		// Same message wrapped in the tty progress renderer's color codes.
		"\x1b[31mtoomanyrequests: Rate exceeded\x1b[0m",
		"toomanyrequests: You have reached your pull rate limit. You may increase the limit by authenticating",
		"received unexpected HTTP status: 429 Too Many Requests",
	}
	for _, output := range rateLimited {
		if !isRateLimited(output) {
			t.Errorf("isRateLimited(%q) = false, want true", output)
		}
	}

	notRateLimited := []string{
		"",
		"Error response from daemon: 403 Forbidden",
		"pull access denied for parfin/nope, repository does not exist or may require 'docker login'",
		"manifest unknown",
		"Bind for 0.0.0.0:8545 failed: port is already allocated",
	}
	for _, output := range notRateLimited {
		if isRateLimited(output) {
			t.Errorf("isRateLimited(%q) = true, want false", output)
		}
	}
}

// Rejections that another attempt can't fix must fail fast (a stale ECR token
// should print its fix now, not after three cool-offs); anything transient
// keeps its retries.
func TestIsPermanentFailure(t *testing.T) {
	permanent := []string{
		"Error response from daemon: 403 Forbidden",
		"pull access denied for parfin/nope, repository does not exist or may require 'docker login'",
		"unauthorized: authentication required",
		"manifest unknown",
		"invalid reference format",
		`dial tcp: lookup public.ecr.aws: no such host`,
	}
	for _, output := range permanent {
		if !isPermanentFailure(output) {
			t.Errorf("isPermanentFailure(%q) = false, want true", output)
		}
	}

	retryable := []string{
		"toomanyrequests: Rate exceeded",
		"net/http: TLS handshake timeout",
		"read tcp 10.0.0.1:443: connection reset by peer",
		"received unexpected HTTP status: 503 Service Unavailable",
		"unexpected EOF",
	}
	for _, output := range retryable {
		if isPermanentFailure(output) {
			t.Errorf("isPermanentFailure(%q) = true, want false", output)
		}
	}
}

// A run that got throttled is retried even if it also logged a permanent
// rejection for a different image; a run that only failed permanently isn't.
func TestWorthRetrying(t *testing.T) {
	cases := map[string]bool{
		"toomanyrequests: Rate exceeded":  true,
		"net/http: TLS handshake timeout": true,
		"":                                true,
		"Error response from daemon: 403 Forbidden":     false,
		"pull access denied for parfin/nope":            false,
		"403 Forbidden\ntoomanyrequests: Rate exceeded": true,
	}
	for output, want := range cases {
		if got := worthRetrying(output); got != want {
			t.Errorf("worthRetrying(%q) = %v, want %v", output, got, want)
		}
	}
}

// The cool-off grows so a throttled retry isn't just another burst.
func TestPullBackoffGrows(t *testing.T) {
	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second}
	for i, w := range want {
		if got := pullBackoff(i + 1); got != w {
			t.Errorf("pullBackoff(%d) = %s, want %s", i+1, got, w)
		}
	}
}

// Serialized runs must carry compose v2's global `--parallel 1` *before* the
// subcommand; anything after it would be parsed as a subcommand flag.
func TestComposeArgsSerialV2(t *testing.T) {
	name, argv := composeArgs(docker.ComposeV2, true, "pull")
	if name != "docker" {
		t.Errorf("name = %q, want docker", name)
	}
	if got := strings.Join(argv, " "); got != "compose --parallel 1 pull" {
		t.Errorf("argv = %q, want %q", got, "compose --parallel 1 pull")
	}
}

func TestComposeArgsParallelV2(t *testing.T) {
	_, argv := composeArgs(docker.ComposeV2, false, "up", "-d", "kos-a")
	if got := strings.Join(argv, " "); got != "compose up -d kos-a" {
		t.Errorf("argv = %q, want %q", got, "compose up -d kos-a")
	}
}

// Compose v1 has no --parallel flag (it honors COMPOSE_PARALLEL_LIMIT, which
// runCompose sets), so the argv must stay untouched.
func TestComposeArgsV1(t *testing.T) {
	name, argv := composeArgs(docker.ComposeV1, true, "pull")
	if name != "docker-compose" {
		t.Errorf("name = %q, want docker-compose", name)
	}
	if got := strings.Join(argv, " "); got != "pull" {
		t.Errorf("argv = %q, want %q", got, "pull")
	}
}

// Runs the real invocation against the installed compose — proving it accepts
// the global `--parallel` flag we pass — over a stack where every service is
// built or pull_policy=never. That's the --local shape: compose must skip them
// all and succeed without touching a registry.
func TestPullImagesSequentiallySkipsUnpullableServices(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose not available")
	}
	t.Chdir(t.TempDir())

	compose := "services:\n" +
		"    built:\n" +
		"        image: rayls-test-built:latest\n" +
		"        pull_policy: build\n" +
		"        build:\n" +
		"            context: .\n" +
		"    prebuilt:\n" +
		"        image: rayls-test-prebuilt:latest\n" +
		"        pull_policy: never\n"
	if err := os.WriteFile("docker-compose.yaml", []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := pullImagesSequentially(docker.ComposeV2); err != nil {
		t.Fatalf("pullImagesSequentially: %v", err)
	}
}
