package stacks

import (
	"os"
	"os/exec"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

// The contracts deploy is one-shot per set of volumes, so init has to be able to
// see leftover state and say it is resuming. Runs against real docker in a
// throwaway compose project (the temp dir name becomes the project name, so
// nothing can touch a real stack).
func TestExistingStackVolumes(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose not available")
	}
	t.Chdir(t.TempDir())

	compose := "services:\n" +
		"    probe:\n" +
		"        image: alpine:3.20\n" +
		"        command: [\"true\"]\n" +
		"        volumes:\n" +
		"            - probe-data:/data\n" +
		"volumes:\n" +
		"    probe-data: {}\n"
	if err := os.WriteFile("docker-compose.yaml", []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		exec.Command("docker", "compose", "down", "-v", "--remove-orphans").Run()
	})

	if got := existingStackVolumes(docker.ComposeV2); len(got) != 0 {
		t.Fatalf("fresh project reports volumes %v, want none", got)
	}

	// `up` creates the named volume — this is the state that survives `rayls
	// stop`/`rayls down` and makes the contracts deploy refuse to run again.
	if out, err := exec.Command("docker", "compose", "up", "-d").CombinedOutput(); err != nil {
		t.Fatalf("docker compose up: %v\n%s", err, out)
	}
	if volumes := existingStackVolumes(docker.ComposeV2); len(volumes) != 1 {
		t.Fatalf("existingStackVolumes = %v, want the one probe-data volume", volumes)
	}

	// `rayls down -v` is the documented way back to a clean slate.
	if out, err := exec.Command("docker", "compose", "down", "-v").CombinedOutput(); err != nil {
		t.Fatalf("docker compose down -v: %v\n%s", err, out)
	}
	if got := existingStackVolumes(docker.ComposeV2); len(got) != 0 {
		t.Errorf("volumes still present after down -v: %v", got)
	}
}
