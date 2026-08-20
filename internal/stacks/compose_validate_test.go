package stacks

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"

	"gopkg.in/yaml.v3"
)

// TestGeneratedComposeValidates round-trips the real generator output through
// `docker compose config`: base file with git build contexts + ssh forwarding,
// merged with a `rayls dev` override (the !override tag needs Compose >= 2.24).
// Skipped when docker compose isn't available on the host.
func TestGeneratedComposeValidates(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose not available")
	}
	t.Chdir(t.TempDir())

	srcs, err := docker.ResolveSources()
	if err != nil {
		t.Fatal(err)
	}
	pc := docker.PublicChainPresets["rayls-testnet"]
	compose := docker.GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, false, srcs)
	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("docker-compose.yaml", out, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePostgresInitScript(); err != nil {
		t.Fatal(err)
	}

	validate := func(stage string) {
		cmd := exec.Command("docker", "compose", "config", "-q")
		// SSH_AUTH_SOCK may be absent in CI; config only validates syntax.
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: docker compose config failed: %v\n%s", stage, err, stderr.String())
		}
	}
	validate("base compose")

	// The contracts command is a single-quoted shell script inside the compose
	// `command:` string. Compose splits that string shell-style and interpolates
	// `$`, so check what the container would actually be handed: three argv
	// entries with the already-deployed guard intact.
	t.Run("contracts command survives compose parsing", func(t *testing.T) {
		out, err := exec.Command("docker", "compose", "config", "--format", "json").Output()
		if err != nil {
			t.Fatalf("docker compose config --format json: %v", err)
		}
		var project struct {
			Services map[string]struct {
				Command []string `json:"command"`
			} `json:"services"`
		}
		if err := json.Unmarshal(out, &project); err != nil {
			t.Fatal(err)
		}
		argv := project.Services["contracts"].Command
		if len(argv) != 3 || argv[0] != "/bin/bash" || argv[1] != "-c" {
			t.Fatalf("contracts command = %q, want [/bin/bash -c <script>]", argv)
		}
		for _, want := range []string{
			"ls .openzeppelin/*.json",
			"exec node docker/dev/contracts_deploy_healthcheck.js",
			"exec docker/dev/deploy_contracts.sh",
			"1.0,Contracts already deployed",
		} {
			if !strings.Contains(argv[2], want) {
				t.Errorf("contracts script missing %q:\n%s", want, argv[2])
			}
		}
	})

	// Now flip relayer + contracts to dev mode and validate the merged config.
	srcDir := t.TempDir()
	if err := os.WriteFile(".env", []byte("RELAYER_SRC="+srcDir+"\nCONTRACTS_SRC="+srcDir+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RegenerateDevOverride(); err != nil {
		t.Fatal(err)
	}
	validate("with dev override")
}
