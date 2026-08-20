package stacks

import (
	"os"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/envfile"
)

// Keeping an existing docker-compose.yaml must still record the --local build
// pins: missing pins silently revert source builds to 3.0.0-era git refs
// (regression: .env deleted to reset the key + "keep" at the prompt).
func TestGenerateDockerComposeKeepPathRecordsPins(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("docker-compose.yaml", []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Feed "n" (keep) to the overwrite prompt.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	generated, err := GenerateDockerCompose([]string{"a"}, false, nil, true, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("expected the existing compose to be kept")
	}

	vars, err := envfile.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	// No sibling checkouts exist next to the temp stack dir, so the fallback
	// REF pins must have been recorded for the buildable components.
	if vars["CONTRACTS_REF"] == "" && vars["CONTRACTS_SRC"] == "" {
		t.Errorf("keep path did not record a contracts build pin; .env: %v", vars)
	}
	if vars["RELAYER_REF"] == "" && vars["RELAYER_SRC"] == "" {
		t.Errorf("keep path did not record a relayer build pin; .env: %v", vars)
	}
}
