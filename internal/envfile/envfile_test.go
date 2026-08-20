package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := `# a comment
FOO=bar
export EXPORTED=yes
QUOTED="hello world"
SINGLE='single'
EMPTY=

NOEQUALS
SPACED = padded
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vars, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FOO": "bar", "EXPORTED": "yes", "QUOTED": "hello world",
		"SINGLE": "single", "EMPTY": "", "SPACED": "padded",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("%s = %q, want %q", k, vars[k], v)
		}
	}
	if _, ok := vars["NOEQUALS"]; ok {
		t.Errorf("line without = should be skipped")
	}
}

func TestLoadMissingFile(t *testing.T) {
	vars, err := Load(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || len(vars) != 0 {
		t.Errorf("missing file should yield empty map, got %v, %v", vars, err)
	}
}

func TestSetUnsetRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# keep me\nEXISTING=1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Set(path, "DEMO_SRC", "/home/dev/demo"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "EXISTING", "2"); err != nil {
		t.Fatal(err)
	}
	vars, _ := Load(path)
	if vars["DEMO_SRC"] != "/home/dev/demo" || vars["EXISTING"] != "2" {
		t.Errorf("after Set: %v", vars)
	}
	data, _ := os.ReadFile(path)
	if string(data)[:9] != "# keep me" {
		t.Errorf("comments should be preserved, got:\n%s", data)
	}

	if err := Unset(path, "DEMO_SRC"); err != nil {
		t.Fatal(err)
	}
	vars, _ = Load(path)
	if _, ok := vars["DEMO_SRC"]; ok {
		t.Errorf("DEMO_SRC should be removed")
	}
	if vars["EXISTING"] != "2" {
		t.Errorf("EXISTING should survive Unset of another key")
	}

	// Unset of a missing key / file is not an error.
	if err := Unset(path, "NOT_THERE"); err != nil {
		t.Fatal(err)
	}
	if err := Unset(filepath.Join(t.TempDir(), "nope.env"), "X"); err != nil {
		t.Fatal(err)
	}
}

func TestLookupPrecedence(t *testing.T) {
	t.Setenv("PRECEDENCE_TEST_KEY", "process")
	t.Setenv("EMPTY_IN_ENV", "")
	vars := map[string]string{"PRECEDENCE_TEST_KEY": "file", "FILE_ONLY": "file", "EMPTY_IN_ENV": "file"}
	if got := Lookup(vars, "PRECEDENCE_TEST_KEY"); got != "process" {
		t.Errorf("process env should win, got %q", got)
	}
	if got := Lookup(vars, "FILE_ONLY"); got != "file" {
		t.Errorf("file value should apply, got %q", got)
	}
	// A set-but-empty process var means "not configured" for the CLI's
	// generation-time decisions: fall through to the file's intent rather than
	// silently discarding an explicit pin. (See the Lookup doc for how this
	// differs from compose's own interpolation in that edge.)
	if got := Lookup(vars, "EMPTY_IN_ENV"); got != "file" {
		t.Errorf("set-but-empty process env should fall through to the file, got %q", got)
	}
}

// Unquoted values end at inline comments, matching compose's .env semantics;
// quoted values keep their content verbatim.
func TestLoadInlineComments(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	content := "PLAIN=value # trailing note\nQUOTED=\"value # not a comment\"\nHASH_NO_SPACE=val#ue\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vars, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["PLAIN"] != "value" {
		t.Errorf("PLAIN = %q, want %q (inline comment stripped)", vars["PLAIN"], "value")
	}
	if vars["QUOTED"] != "value # not a comment" {
		t.Errorf("QUOTED = %q, want the quoted content verbatim", vars["QUOTED"])
	}
	if vars["HASH_NO_SPACE"] != "val#ue" {
		t.Errorf("HASH_NO_SPACE = %q, want %q (hash without preceding space is part of the value)", vars["HASH_NO_SPACE"], "val#ue")
	}
}
