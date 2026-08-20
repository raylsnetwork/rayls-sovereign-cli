// Package envfile reads and updates the stack directory's .env file. Docker
// Compose interpolates ${VAR} references in docker-compose.yaml from this file
// automatically; the CLI reads it too so that generation-time decisions (git
// refs, source paths) honor the same values the compose run will see.
package envfile

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Load parses KEY=VALUE lines from path. Missing file returns an empty map.
// Lines starting with # and blank lines are ignored. Values may be quoted with
// single or double quotes, which are stripped (compose does the same).
func Load(path string) (map[string]string, error) {
	vars := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		} else if idx := strings.Index(value, " #"); idx >= 0 {
			// Unquoted values end at an inline comment (` #`), same as
			// compose's .env parsing — otherwise the CLI would read
			// `KEY=value # note` differently from the compose run.
			value = strings.TrimSpace(value[:idx])
		}
		vars[key] = value
	}
	return vars, nil
}

// Lookup returns the value for key: a NON-EMPTY process environment variable
// wins, otherwise the .env file value. A set-but-empty process var falls
// through to the file — "empty" means "not configured" for every
// generation-time decision the CLI makes with these values. (Compose's own
// interpolation is subtly different in that edge: an empty shell var shadows
// the .env file there, so `${VAR:-default}` resolves to the DEFAULT rather
// than the file value. Nobody should be exporting empty pins; when they do,
// the CLI reads the file's intent instead of silently discarding it.)
func Lookup(fileVars map[string]string, key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fileVars[key]
}

// Set upserts key=value in the .env file at path, preserving every other line
// (comments included). The file is created if missing.
func Set(path, key, value string) error {
	return rewrite(path, key, &value)
}

// Unset removes key from the .env file at path. A missing file or key is not
// an error.
func Unset(path, key string) error {
	return rewrite(path, key, nil)
}

func rewrite(path, key string, value *string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	out := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		trimmed := strings.TrimPrefix(strings.TrimSpace(line), "export ")
		k, _, found := strings.Cut(trimmed, "=")
		if found && strings.TrimSpace(k) == key && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			if value == nil {
				continue // drop the line
			}
			if !replaced {
				out = append(out, fmt.Sprintf("%s=%s", key, *value))
				replaced = true
			}
			continue
		}
		out = append(out, line)
	}
	if value != nil && !replaced {
		out = append(out, fmt.Sprintf("%s=%s", key, *value))
	}
	content := strings.Join(out, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// Keys returns the sorted keys of vars — handy for deterministic output.
func Keys(vars map[string]string) []string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
