package stacks

import (
	"os"
	"strings"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

func TestNormalizePrivateKey(t *testing.T) {
	bare := strings.Repeat("ab", 32) // 64 hex chars
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string // substring the error must contain; empty = no error
	}{
		{"bare hex", bare, bare, ""},
		{"0x prefix stripped", "0x" + bare, bare, ""},
		{"0X prefix stripped", "0X" + bare, bare, ""},
		{"uppercase lowered", "0x" + strings.ToUpper(bare), bare, ""},
		{"surrounding whitespace trimmed", "  " + bare + "\n", bare, ""},
		{"too short", bare[:62], "", "got 62"},
		{"too long", bare + "ab", "", "got 66"},
		{"right length, non-hex", strings.Repeat("zg", 32), "", "only hex characters"},
		{"empty", "", "", "got 0"},
		{"0x alone", "0x", "", "got 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePrivateKey(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
				// The input may be a mistyped REAL key: the error must
				// never echo it back. (Only meaningful for key-sized
				// inputs; "0x" trivially appears in the format hint.)
				if len(tc.in) > 10 && strings.Contains(err.Error(), strings.TrimSpace(tc.in)) {
					t.Errorf("error message echoes the key material: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// clearKeyEnv makes both key variables truly UNSET (t.Setenv registers the
// restore; the Unsetenv gives resolvePublicChainKey's os.LookupEnv the
// not-present case rather than set-but-empty).
func clearKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv(publicChainKeyVar, "")
	t.Setenv(demoPublicChainKeyVar, "")
	os.Unsetenv(publicChainKeyVar)
	os.Unsetenv(demoPublicChainKeyVar)
}

func writeCompose(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile("docker-compose.yaml", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeEnvFile(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePublicChainKey(t *testing.T) {
	key := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)

	t.Run("nothing set resolves empty", func(t *testing.T) {
		clearKeyEnv(t)
		v, _, err := resolvePublicChainKey(map[string]string{})
		if err != nil || v != "" {
			t.Errorf("got (%q, %v), want empty, nil", v, err)
		}
	})

	t.Run("env var wins over .env file", func(t *testing.T) {
		clearKeyEnv(t)
		t.Setenv(publicChainKeyVar, key)
		v, source, err := resolvePublicChainKey(map[string]string{publicChainKeyVar: other})
		if err != nil || v != key {
			t.Errorf("got (%q, %v), want env value", v, err)
		}
		if !strings.Contains(source, "environment") {
			t.Errorf("source = %q, want environment", source)
		}
	})

	t.Run("DEMO alias wins over canonical var, from env or .env", func(t *testing.T) {
		clearKeyEnv(t)
		t.Setenv(demoPublicChainKeyVar, key)
		if v, _, _ := resolvePublicChainKey(map[string]string{publicChainKeyVar: other}); v != key {
			t.Errorf("env DEMO should win, got %q", v)
		}
		clearKeyEnv(t)
		// Compose interpolates ${DEMO_...} from the stack .env too.
		if v, _, _ := resolvePublicChainKey(map[string]string{demoPublicChainKeyVar: key, publicChainKeyVar: other}); v != key {
			t.Errorf(".env DEMO should win, got %q", v)
		}
	})

	t.Run("set-but-empty canonical var is a hard error (shadows .env in compose)", func(t *testing.T) {
		clearKeyEnv(t)
		t.Setenv(publicChainKeyVar, "")
		_, _, err := resolvePublicChainKey(map[string]string{publicChainKeyVar: key})
		if err == nil || !strings.Contains(err.Error(), "EMPTY") {
			t.Errorf("expected set-but-empty error, got %v", err)
		}
	})

	t.Run("set-but-empty DEMO alias falls through harmlessly", func(t *testing.T) {
		clearKeyEnv(t)
		t.Setenv(demoPublicChainKeyVar, "")
		// Compose's `:-` treats the empty alias as unset AND shadows any
		// .env DEMO value; the canonical .env key must still resolve.
		v, _, err := resolvePublicChainKey(map[string]string{demoPublicChainKeyVar: other, publicChainKeyVar: key})
		if err != nil || v != key {
			t.Errorf("got (%q, %v), want canonical .env key", v, err)
		}
	})
}

// keyCheckCase is one CheckPublicChainKey scenario: the on-disk stack state
// (compose + .env), the process env, and the expected outcome.
type keyCheckCase struct {
	name    string
	compose string            // compose file content; "" = no docker-compose.yaml
	envFile string            // stack .env content; "" = no .env
	env     map[string]string // process env to set (empty value = set-but-empty)
	wantErr string            // substring the error must contain; "" = expect success
}

func runKeyCheckCase(t *testing.T, tc keyCheckCase) {
	t.Chdir(t.TempDir())
	clearKeyEnv(t)
	if tc.compose != "" {
		writeCompose(t, tc.compose)
	}
	if tc.envFile != "" {
		writeEnvFile(t, tc.envFile)
	}
	for k, v := range tc.env {
		t.Setenv(k, v)
	}
	err := CheckPublicChainKey()
	if tc.wantErr == "" {
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
		t.Errorf("expected error containing %q, got %v", tc.wantErr, err)
	}
}

func TestCheckPublicChainKey(t *testing.T) {
	key := strings.Repeat("ab", 32)
	keyLine := "  - " + docker.PublicChainKeyComposeEnv + "\n"

	tests := []keyCheckCase{
		{name: "no compose file passes"},
		{name: "local stack compose passes without key",
			compose: "services: {}\n# no public chain key line\n"},
		{name: "testnet compose without any key fails with funding pointer",
			compose: "environment:\n" + keyLine, wantErr: docker.FundingURL},
		{name: "key in env passes",
			compose: keyLine, env: map[string]string{publicChainKeyVar: key}},
		{name: "key in stack .env passes",
			compose: keyLine, envFile: publicChainKeyVar + "=" + key + "\n"},
		{name: "DEMO alias in stack .env passes (compose reads it too)",
			compose: keyLine, envFile: demoPublicChainKeyVar + "=" + key + "\n"},
		{name: "set-but-empty env var fails even with .env key",
			compose: keyLine, envFile: publicChainKeyVar + "=" + key + "\n",
			env: map[string]string{publicChainKeyVar: ""}, wantErr: "EMPTY"},
		{name: "malformed key in .env fails with format error",
			compose: keyLine, envFile: publicChainKeyVar + "=nothex\n", wantErr: "invalid"},
		{name: "pre-OSS compose with embedded default is left alone",
			compose: "  - " + legacyKeyEnvPrefix + "somelegacydefault}}\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runKeyCheckCase(t, tc) })
	}
}

func TestPublicChainKeyGuardApplies(t *testing.T) {
	if !publicChainKeyGuardApplies(nil) {
		t.Error("full-stack up must be guarded")
	}
	if !publicChainKeyGuardApplies([]string{"postgres", "contracts"}) {
		t.Error("subset including contracts must be guarded")
	}
	if publicChainKeyGuardApplies([]string{"postgres", "nats"}) {
		t.Error("subset without contracts must not be guarded")
	}
}

func TestPersistPublicChainKey(t *testing.T) {
	t.Chdir(t.TempDir())

	// Pre-existing .env content (build pins) must survive the upsert, and the
	// file must be tightened to 0600 even though it started 0644.
	if err := os.WriteFile(".env", []byte("# pins\nCONTRACTS_REF=main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	key := strings.Repeat("cd", 32)
	if err := persistPublicChainKey(key); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), publicChainKeyVar+"="+key) {
		t.Errorf(".env does not contain the persisted key line:\n%s", data)
	}
	if !strings.Contains(string(data), "CONTRACTS_REF=main") {
		t.Errorf("persisting the key clobbered existing .env content:\n%s", data)
	}
	info, err := os.Stat(".env")
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf(".env permissions = %o, want 600", perm)
	}
}

func TestPersistPublicChainKeyCreatesFile0600(t *testing.T) {
	t.Chdir(t.TempDir())
	// Fresh stack dir: the file must be BORN 0600, since the secret is in it
	// from the first write.
	if err := persistPublicChainKey(strings.Repeat("ef", 32)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(".env")
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf(".env created with permissions %o, want 600", perm)
	}
}
