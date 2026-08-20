package stacks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/envfile"

	"gopkg.in/yaml.v3"
)

// pinLocalSources records the hub-less build sources for a hub-less stack:
// the local sibling checkout via <PREFIX>_SRC when one exists, else the
// version/3.0.1 ref pin — never clobbering a source the user already chose.
// Set-but-empty env vars neutralize any ambient pins on the machine running
// the tests AND exercise the guard's file-over-empty-env behavior (a bare
// `export CONTRACTS_REF=` must not make pinLocalSources overwrite an explicit
// .env pin).
func TestPinLocalSources(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "stack")
	if err := os.Mkdir(stackDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(stackDir)
	for _, key := range []string{"CONTRACTS_REF", "RELAYER_REF", "CONTRACTS_SRC", "RELAYER_SRC", "RAYLS_SRC_DIR"} {
		t.Setenv(key, "")
	}

	// No sibling checkouts next to the stack dir: fall back to the ref pins.
	if err := pinLocalSources(); err != nil {
		t.Fatal(err)
	}
	vars, err := envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	if vars["CONTRACTS_REF"] != localSourcesRef || vars["RELAYER_REF"] != localSourcesRef {
		t.Errorf("no siblings: want both refs pinned to %s, got CONTRACTS_REF=%q RELAYER_REF=%q", localSourcesRef, vars["CONTRACTS_REF"], vars["RELAYER_REF"])
	}
	if vars["CONTRACTS_SRC"] != "" || vars["RELAYER_SRC"] != "" {
		t.Errorf("no siblings: no _SRC should be recorded, got CONTRACTS_SRC=%q RELAYER_SRC=%q", vars["CONTRACTS_SRC"], vars["RELAYER_SRC"])
	}

	// A sibling checkout wins over the ref pin: <srcRoot>/<DirName> with .git.
	contractsCheckout := filepath.Join(root, "rayls-privacy-contracts")
	if err := os.MkdirAll(filepath.Join(contractsCheckout, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	if err := pinLocalSources(); err != nil {
		t.Fatal(err)
	}
	vars, err = envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	// Recorded RELATIVE (resolved against the stack dir by compose and every
	// rayls command) so the .env stays portable across machines.
	wantSrc := filepath.Join("..", "rayls-privacy-contracts")
	if vars["CONTRACTS_SRC"] != wantSrc {
		t.Errorf("sibling present: CONTRACTS_SRC = %q, want %q", vars["CONTRACTS_SRC"], wantSrc)
	}
	if _, ok := vars["CONTRACTS_REF"]; ok {
		t.Errorf("sibling present: CONTRACTS_REF should not be pinned, got %q", vars["CONTRACTS_REF"])
	}
	if vars["RELAYER_REF"] != localSourcesRef {
		t.Errorf("relayer has no sibling here: RELAYER_REF should be pinned to %s, got %q", localSourcesRef, vars["RELAYER_REF"])
	}

	// A user-pinned ref survives even when a sibling checkout exists; only the
	// unpinned component is recorded.
	if err := os.WriteFile(".env", []byte("# my stack\nCONTRACTS_REF=my-branch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := pinLocalSources(); err != nil {
		t.Fatal(err)
	}
	vars, err = envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	if vars["CONTRACTS_REF"] != "my-branch" {
		t.Errorf("user CONTRACTS_REF should be preserved, got %q", vars["CONTRACTS_REF"])
	}
	if vars["CONTRACTS_SRC"] != "" {
		t.Errorf("user CONTRACTS_REF pin should suppress the _SRC record, got %q", vars["CONTRACTS_SRC"])
	}
	if vars["RELAYER_REF"] != localSourcesRef {
		t.Errorf("RELAYER_REF should be pinned to %s, got %q", localSourcesRef, vars["RELAYER_REF"])
	}

	// A non-empty process-env source suppresses the .env write entirely.
	t.Setenv("RELAYER_REF", "session-override")
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	if err := pinLocalSources(); err != nil {
		t.Fatal(err)
	}
	vars, err = envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["RELAYER_REF"]; ok {
		t.Errorf("RELAYER_REF should not be written when a non-empty env override exists, got %q", vars["RELAYER_REF"])
	}
	if vars["CONTRACTS_SRC"] != wantSrc {
		t.Errorf("CONTRACTS_SRC should still be recorded, got %q", vars["CONTRACTS_SRC"])
	}

	// RAYLS_VERSION is a user version pin (Sources.Ref honors it below _REF):
	// don't outrank it with auto-recorded values.
	t.Setenv("RELAYER_REF", "")
	if err := os.WriteFile(".env", []byte("RAYLS_VERSION=3.0.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := pinLocalSources(); err != nil {
		t.Fatal(err)
	}
	vars, err = envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["CONTRACTS_REF"]; ok {
		t.Errorf("RAYLS_VERSION pin should suppress CONTRACTS_REF, got %q", vars["CONTRACTS_REF"])
	}
	if _, ok := vars["CONTRACTS_SRC"]; ok {
		t.Errorf("RAYLS_VERSION pin should suppress CONTRACTS_SRC, got %q", vars["CONTRACTS_SRC"])
	}
	if _, ok := vars["RELAYER_REF"]; ok {
		t.Errorf("RAYLS_VERSION pin should suppress RELAYER_REF, got %q", vars["RELAYER_REF"])
	}
}

// TestGeneratedNoHubComposeValidates runs the full hub-less --local generation
// path (source pinning included) and round-trips the output through `docker
// compose config`. Skipped when docker compose isn't available on the host.
func TestGeneratedNoHubComposeValidates(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose not available")
	}
	t.Chdir(t.TempDir())
	// Neutralize ambient pins so the .env assertions below are hermetic
	// (set-but-empty vars don't count as user pins).
	for _, key := range []string{"CONTRACTS_REF", "RELAYER_REF", "CONTRACTS_SRC", "RELAYER_SRC", "RAYLS_SRC_DIR"} {
		t.Setenv(key, "")
	}

	pc := docker.PublicChainPresets["rayls-testnet"]
	generated, err := GenerateDockerCompose([]string{"a", "b"}, false, nil, true, &pc, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("expected a fresh docker-compose.yaml to be generated")
	}

	// No sibling checkouts next to a temp dir, so the generation must have
	// pinned the git refs.
	vars, err := envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	if vars["CONTRACTS_REF"] != localSourcesRef || vars["RELAYER_REF"] != localSourcesRef {
		t.Errorf("want CONTRACTS_REF/RELAYER_REF pinned to %s, got %q/%q", localSourcesRef, vars["CONTRACTS_REF"], vars["RELAYER_REF"])
	}

	data, err := os.ReadFile("docker-compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]interface{} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"private-network-hub", "proofs-api", "relayer-a", "relayer-b", "governance-api", "audit-explorer"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("no-hub compose should not contain service %q", name)
		}
	}

	cmd := exec.Command("docker", "compose", "config", "-q")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, stderr.String())
	}
}

// Topology changes are detected from the service-name sets; observability and
// explorer UIs (otel, blockscout-*) can be toggled freely.
func TestTopologyDiff(t *testing.T) {
	oldS := []string{"contracts", "kos-a", "pubrelayer-a", "public-chain", "nats", "certs-init"}
	newHub := append([]string{"private-network-hub", "relayer-a", "proofs-api"}, oldS...)
	diff := topologyDiff(oldS, newHub)
	if len(diff) != 3 {
		t.Errorf("expected 3 topology-defining additions, got %v", diff)
	}
	neutral := append([]string{"otel", "blockscout-db-a"}, oldS...)
	if diff := topologyDiff(oldS, neutral); len(diff) != 0 {
		t.Errorf("otel/blockscout must be topology-neutral, got %v", diff)
	}
	if diff := topologyDiff(oldS, oldS); len(diff) != 0 {
		t.Errorf("identical sets must produce no diff, got %v", diff)
	}
}

// A pure service rename (governance-postgres -> postgres) is not a topology
// change: pre-rename stacks must not be forced through `down -v`.
func TestTopologyDiffRenameEquivalence(t *testing.T) {
	oldS := []string{"contracts", "kos-a", "governance-postgres", "nats"}
	newS := []string{"contracts", "kos-a", "postgres", "nats"}
	if diff := topologyDiff(oldS, newS); len(diff) != 0 {
		t.Errorf("rename must not register as a topology change, got %v", diff)
	}
}
