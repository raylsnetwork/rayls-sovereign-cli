package stacks

import (
	"os"
	"strings"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

// minimal compose file with the 3.0.0 lean single-participant service set
const testComposeYAML = `services:
    contracts:
        image: rayls-contracts:lean-no-pnh
    kos-a:
        image: rayls-kos:latest
    pubrelayer-a:
        image: rayls-pubrelayer:latest
    private-network-hub:
        image: public.ecr.aws/w0k9o1t3/rayls-demo/rayls-private-network-hub:latest
    nats:
        image: public.ecr.aws/w0k9o1t3/rayls-demo/rayls-nats:latest
`

// serviceSection extracts one service's block (4-space indented key) from the
// override YAML, up to the next service key at the same indent.
func serviceSection(s, name string) string {
	lines := strings.Split(s, "\n")
	var out []string
	in := false
	for _, line := range lines {
		if line == "    "+name+":" {
			in = true
			continue
		}
		if in {
			if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") || !strings.HasPrefix(line, " ") {
				break // next service or top-level key
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func componentByKeyOrFatal(t *testing.T, key string) *docker.Component {
	t.Helper()
	c := docker.ComponentByKey(key)
	if c == nil {
		t.Fatalf("component %q not registered", key)
	}
	return c
}

func TestRegenerateDevOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("docker-compose.yaml", []byte(testComposeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("RELAYER_SRC=/home/dev/rayls-sovereign-relayer\nCONTRACTS_SRC=/home/dev/rayls-sovereign-contracts\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RegenerateDevOverride(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(devOverrideFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	if !strings.HasPrefix(s, devOverrideMarker) {
		t.Errorf("override should start with the marker")
	}
	if !strings.Contains(s, "build: !override") {
		t.Errorf("build must use !override so the base git context (and its ssh option) is replaced:\n%s", s)
	}

	// relayer in dev mode -> kos-a AND pubrelayer-a get air dev builds
	kos := serviceSection(s, "kos-a")
	if !strings.Contains(kos, `dockerfile: "cts/Dockerfile.dev"`) {
		t.Errorf("kos-a should build the air dev image:\n%s", kos)
	}
	if !strings.Contains(kos, "command: air -c cts/air.toml") {
		t.Errorf("kos-a must override the entrypoint-style command with air:\n%s", kos)
	}
	if !strings.Contains(kos, "ENV_FILE=/parfin/rayls-privacy-relayer-api/.A.env") || !strings.Contains(kos, "GO_DEBUG_PORT=4000") {
		t.Errorf("kos-a needs the air env contract:\n%s", kos)
	}
	if !strings.Contains(kos, "action: sync") || !strings.Contains(kos, "target: /app") {
		t.Errorf("kos-a should have a compose-watch sync rule:\n%s", kos)
	}
	if !strings.Contains(kos, "gocache-kos-a:/root/.cache/go-build") {
		t.Errorf("kos-a should mount a go build cache volume:\n%s", kos)
	}

	pub := serviceSection(s, "pubrelayer-a")
	if !strings.Contains(pub, `dockerfile: "public-relayer/Dockerfile.dev"`) || !strings.Contains(pub, "command: air -c public-relayer/air.toml") {
		t.Errorf("pubrelayer-a should run the air dev image:\n%s", pub)
	}
	if !strings.Contains(pub, "GO_DEBUG_PORT=4050") {
		t.Errorf("pubrelayer-a debug port should be 4050:\n%s", pub)
	}

	// contracts builds from the checkout but never hot-reloads
	contracts := serviceSection(s, "contracts")
	if !strings.Contains(contracts, `context: "/home/dev/rayls-sovereign-contracts"`) {
		t.Errorf("contracts should build from the local checkout:\n%s", contracts)
	}
	for _, forbidden := range []string{"watch", "command: air", "gocache"} {
		if strings.Contains(contracts, forbidden) {
			t.Errorf("contracts must not have %q:\n%s", forbidden, contracts)
		}
	}

}

func TestRegenerateDevOverrideRemovesWhenEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("docker-compose.yaml", []byte(testComposeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devOverrideFile, []byte(devOverrideMarker+"\nservices: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RegenerateDevOverride(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(devOverrideFile); !os.IsNotExist(err) {
		t.Errorf("override should be removed when no component is in dev mode")
	}
}

func TestRegenerateDevOverrideRefusesForeignFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("docker-compose.yaml", []byte(testComposeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devOverrideFile, []byte("services:\n  hand: {written: true}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RegenerateDevOverride(); err == nil {
		t.Errorf("should refuse to overwrite a hand-written override")
	}
}

func TestServicesForComponentMapping(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("docker-compose.yaml", []byte(testComposeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	all, err := composeServiceNames()
	if err != nil {
		t.Fatal(err)
	}
	comp := componentByKeyOrFatal(t, "relayer")
	got := map[string]string{}
	for _, ds := range servicesForComponent(comp, all) {
		got[ds.name] = ds.build.DevDockerfile
	}
	if got["kos-a"] != "cts/Dockerfile.dev" || got["pubrelayer-a"] != "public-relayer/Dockerfile.dev" {
		t.Errorf("relayer mapping = %v", got)
	}
	if len(got) != 2 {
		t.Errorf("only the relayer component's services should map, got %v", got)
	}
}
