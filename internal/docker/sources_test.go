package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func resolveIn(t *testing.T, dotenv string) *Sources {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if dotenv != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0644); err != nil {
			t.Fatal(err)
		}
	}
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	return srcs
}

func TestSourcesDefaults(t *testing.T) {
	srcs := resolveIn(t, "")
	for key, want := range map[string]string{
		"contracts": "lean-no-pnh-3.0.0",
		"relayer":   "v3.0.0",
	} {
		if got := srcs.Ref(*ComponentByKey(key)); got != want {
			t.Errorf("%s default ref = %q, want %q", key, got, want)
		}
	}
	c := *ComponentByKey("relayer")
	if got := srcs.Repo(c); got != "git@github.com:raylsnetwork/rayls-privacy-relayer-api.git" {
		t.Errorf("default repo = %q", got)
	}
	want := "${RELAYER_SRC:-git@github.com:raylsnetwork/rayls-privacy-relayer-api.git#v3.0.0}"
	if got := srcs.BuildContext(c); got != want {
		t.Errorf("context = %q, want %q", got, want)
	}
}

func TestSourcesRaylsVersion(t *testing.T) {
	t.Setenv("RAYLS_VERSION", "3.1.0")
	srcs := resolveIn(t, "")
	for _, c := range Components {
		if got := srcs.Ref(c); got != "v3.1.0" {
			t.Errorf("%s ref = %q, want v3.1.0", c.Key, got)
		}
	}
}

func TestSourcesPerComponentRefWinsOverVersion(t *testing.T) {
	t.Setenv("RAYLS_VERSION", "3.1.0")
	t.Setenv("CONTRACTS_REF", "fix/reorg-handling")
	srcs := resolveIn(t, "")
	if got := srcs.Ref(*ComponentByKey("contracts")); got != "fix/reorg-handling" {
		t.Errorf("contracts ref = %q, want fix/reorg-handling", got)
	}
	if got := srcs.Ref(*ComponentByKey("relayer")); got != "v3.1.0" {
		t.Errorf("relayer ref = %q, want v3.1.0", got)
	}
}

func TestSourcesEnvFileAndProcessPrecedence(t *testing.T) {
	t.Setenv("CONTRACTS_REF", "from-process")
	srcs := resolveIn(t, "CONTRACTS_REF=from-file\nRELAYER_REF=file-ref\n")
	if got := srcs.Ref(*ComponentByKey("contracts")); got != "from-process" {
		t.Errorf("process env should win over .env, got %q", got)
	}
	if got := srcs.Ref(*ComponentByKey("relayer")); got != "file-ref" {
		t.Errorf(".env value should apply, got %q", got)
	}
}

func TestSourcesHTTPSRepoDropsSSH(t *testing.T) {
	t.Setenv("CONTRACTS_REPO", "https://github.com/raylsnetwork/rayls-privacy-contracts.git")
	srcs := resolveIn(t, "")
	b := srcs.BuildSection("contracts", "contracts")
	if len(b.Ssh) != 0 {
		t.Errorf("https context should not request ssh forwarding, got %v", b.Ssh)
	}
	sshBuild := srcs.BuildSection("relayer", "kos")
	if len(sshBuild.Ssh) != 1 {
		t.Errorf("ssh context should request agent forwarding, got %v", sshBuild.Ssh)
	}
}

func TestSourcesBuildSectionUsesProductionDockerfile(t *testing.T) {
	srcs := resolveIn(t, "")
	for _, tc := range []struct{ key, prefix, dockerfile string }{
		{"relayer", "kos", "cts/Dockerfile"},
		{"relayer", "pubrelayer", "public-relayer/Dockerfile"},
		{"contracts", "contracts", "Dockerfile.dev"},
	} {
		b := srcs.BuildSection(tc.key, tc.prefix)
		if b == nil || b.Dockerfile != tc.dockerfile {
			t.Errorf("%s/%s dockerfile = %+v, want %q", tc.key, tc.prefix, b, tc.dockerfile)
		}
	}
	if srcs.BuildSection("relayer", "nope") != nil {
		t.Errorf("unknown service prefix should yield nil build section")
	}
}

func TestSourcesSrcFromEnvFile(t *testing.T) {
	srcs := resolveIn(t, "CONTRACTS_SRC=../rayls-privacy-contracts\n")
	if got := srcs.Src(*ComponentByKey("contracts")); got != "../rayls-privacy-contracts" {
		t.Errorf("Src = %q", got)
	}
}
