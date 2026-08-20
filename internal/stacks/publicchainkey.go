package stacks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/fatih/color"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/envfile"
)

// External public chains deploy from the USER's funded key; there is no
// shared default. Compose interpolates docker.PublicChainKeyComposeEnv from the
// shell env / stack .env on every run; init persists the prompted key to
// ./.env so later runs reuse it.
const (
	publicChainKeyVar     = "PUBLIC_CHAIN_PRIVATE_KEY"
	demoPublicChainKeyVar = "DEMO_PUBLIC_CHAIN_PRIVATE_KEY"
	promptAttempts        = 3
)

// docker.PublicChainKeyComposeEnv minus its empty "}}" tail: this prefix with
// a non-empty innermost fallback marks a pre-OSS compose with the shared
// testnet key embedded (see warnLegacyEmbeddedKey).
const legacyKeyEnvPrefix = "PUBLIC_CHAIN_PRIVATE_KEY=${DEMO_PUBLIC_CHAIN_PRIVATE_KEY:-${PUBLIC_CHAIN_PRIVATE_KEY:-"

var privateKeyPattern = regexp.MustCompile(`^(?:0[xX])?[0-9a-fA-F]{64}$`)

// normalizePrivateKey validates a 32-byte hex key and returns it lowercased
// without the 0x prefix. Errors never echo the input, which may be a
// mistyped real key.
func normalizePrivateKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if !privateKeyPattern.MatchString(key) {
		stripped := strings.TrimPrefix(strings.TrimPrefix(key, "0x"), "0X")
		if len(stripped) != 64 {
			return "", fmt.Errorf("expected 64 hex characters after the optional 0x prefix, got %d", len(stripped))
		}
		return "", errors.New("expected only hex characters (0-9, a-f) after the optional 0x prefix")
	}
	if len(key) == 66 {
		key = key[2:]
	}
	return strings.ToLower(key), nil
}

// resolvePublicChainKey mirrors compose's interpolation of
// ${DEMO_...:-${PUBLIC_...:-}}: per variable the process env shadows the
// stack .env even when SET-BUT-EMPTY (see envfile.Lookup), and the DEMO_
// alias wins when non-empty. A set-but-empty canonical var is a hard error:
// compose would shadow any .env key (including a freshly persisted one) into
// an empty container key.
func resolvePublicChainKey(fileVars map[string]string) (value, source string, err error) {
	if v, ok := os.LookupEnv(demoPublicChainKeyVar); ok {
		if v != "" {
			return v, demoPublicChainKeyVar + " in the environment", nil
		}
		// empty alias: compose's `:-` falls through to the inner variable
	} else if v := fileVars[demoPublicChainKeyVar]; v != "" {
		return v, demoPublicChainKeyVar + " in ./.env", nil
	}
	if v, ok := os.LookupEnv(publicChainKeyVar); ok {
		if v != "" {
			return v, "the environment", nil
		}
		return "", "", fmt.Errorf("%s is exported EMPTY in this shell. Compose lets a set-but-empty variable shadow the stack .env, so the deploy would run with no key even after one is saved there; unset it first (`unset %s`)", publicChainKeyVar, publicChainKeyVar)
	}
	if v := fileVars[publicChainKeyVar]; v != "" {
		return v, "./.env", nil
	}
	return "", "", nil
}

// ensureStackPublicChainKey guards init. Runs AFTER generation so it acts on
// the stack's actual compose file (the user may have kept an existing one):
// no-default marker → require + balance-check the user key; legacy embedded
// default → warn; no key reference (local preset, PC-less) → nothing.
func ensureStackPublicChainKey(pc *docker.PublicChain, participants int) error {
	data, err := os.ReadFile("docker-compose.yaml")
	if err != nil {
		return fmt.Errorf("reading docker-compose.yaml: %w", err)
	}
	compose := string(data)
	if strings.Contains(compose, docker.PublicChainKeyComposeEnv) {
		key, err := ensurePublicChainKey(pc)
		if err != nil {
			return err
		}
		return checkDeployerBalance(pc, key, participants)
	}
	warnLegacyEmbeddedKey(compose)
	return nil
}

// ensurePublicChainKey resolves the deployer key (env > ./.env), prompting
// with hidden input and persisting to ./.env on a TTY, failing with
// instructions otherwise. Returns the key as normalized bare hex.
func ensurePublicChainKey(pc *docker.PublicChain) (string, error) {
	yellow := color.New(color.FgYellow)

	fileVars, err := envfile.Load(".env")
	if err != nil {
		return "", fmt.Errorf("reading .env: %w", err)
	}
	value, source, err := resolvePublicChainKey(fileVars)
	if err != nil {
		return "", err
	}
	if value != "" {
		normalized, err := normalizePrivateKey(value)
		if err != nil {
			return "", fmt.Errorf("the public-chain deployer key from %s is invalid: %v", source, err)
		}
		if strings.HasPrefix(source, demoPublicChainKeyVar) {
			yellow.Printf("Using the public-chain deployer key from %s (deprecated alias; prefer %s, the alias overrides it).\n", source, publicChainKeyVar)
		} else {
			fmt.Printf("Using the public-chain deployer key from %s.\n", source)
		}
		if source == "the environment" {
			yellow.Println("Note: keys from the environment are not saved to ./.env; later runs need the variable exported again (or add it to ./.env yourself).")
		}
		return normalized, nil
	}

	if !term.IsTerminal(os.Stdin.Fd()) {
		return "", missingPublicChainKeyError(pc.Name)
	}

	fmt.Printf(`
A deployer key funded on %s is required: the deploy pays public-chain gas and
seeds each participant's public-relayer wallets from it. There is no shared
default key; fund your own key first via %s
(roughly 5 RAYLS per participant), then enter it below. It will be stored in
this directory's .env (permissions 0600) and reused by every later run.

`, pc.Name, pc.Faucet)

	for attempt := 1; attempt <= promptAttempts; attempt++ {
		fmt.Printf("Private key for %s (64 hex chars, 0x optional; input hidden): ", pc.Name)
		raw, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("could not read the key from the terminal: %w", err)
		}
		key, err := normalizePrivateKey(string(raw))
		if err != nil {
			yellow.Printf("Invalid key: %v\n", err)
			continue
		}
		return key, persistPublicChainKey(key)
	}
	return "", missingPublicChainKeyError(pc.Name)
}

// persistPublicChainKey writes the key to ./.env, created/tightened to 0600
// BEFORE the secret lands in it (os.WriteFile keeps an existing file's mode).
func persistPublicChainKey(key string) error {
	if f, err := os.OpenFile(".env", os.O_CREATE, 0o600); err == nil {
		f.Close()
	}
	chmodErr := os.Chmod(".env", 0o600)
	if err := envfile.Set(".env", publicChainKeyVar, key); err != nil {
		return fmt.Errorf("saving the key to ./.env: %w", err)
	}
	warnIfEnvFileCommittable()
	if chmodErr != nil {
		color.New(color.FgYellow).Printf("✓ Key saved to ./.env, but its permissions could NOT be tightened to 0600 (%v); do it manually, the file now contains your private key.\n", chmodErr)
	} else {
		fmt.Println("✓ Key saved to ./.env (mode 0600); every later rayls / docker compose run reuses it.")
	}
	return nil
}

// warnIfEnvFileCommittable: init writes no .gitignore into stack dirs.
// check-ignore exits 0 = ignored, 1 = not ignored (warn), 128 = no repo/git.
func warnIfEnvFileCommittable() {
	err := exec.Command("git", "check-ignore", "-q", ".env").Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		color.New(color.FgYellow).Println("This directory is a git repository and .env is NOT git-ignored; add '.env' to .gitignore before committing, it now contains your private key.")
	}
}

func missingPublicChainKeyError(chainName string) error {
	return fmt.Errorf(`no funded deployer key for %s.

Deploying to %s needs your own private key, funded with testnet RAYLS; there
is no shared default key. Get funds at %s, then provide
the key one of these ways:

  rayls init                                     interactive prompt (key is saved to ./.env)
  PUBLIC_CHAIN_PRIVATE_KEY=<hex> rayls init      environment variable (CI-friendly, overrides ./.env)
  echo 'PUBLIC_CHAIN_PRIVATE_KEY=<hex>' >> .env  stack .env directly (then: chmod 600 .env)

64 hex characters, 0x prefix optional. Fully local stacks need no key: rayls init --local`, chainName, chainName, docker.FundingURL)
}

// warnLegacyEmbeddedKey flags pre-OSS compose files (shared key embedded as
// the innermost fallback): still runnable, but that account is drained.
func warnLegacyEmbeddedKey(compose string) {
	idx := strings.Index(compose, legacyKeyEnvPrefix)
	if idx < 0 {
		return
	}
	if strings.HasPrefix(compose[idx+len(legacyKeyEnvPrefix):], "}}") {
		return // current no-default form, not a legacy embed
	}
	color.New(color.FgYellow).Printf("This stack's docker-compose.yaml embeds a legacy shared testnet deployer key, which is no longer funded, so a redeploy will fail. Re-run `rayls init` to switch to your own funded key (get funds at %s).\n", docker.FundingURL)
}

// publicChainKeyGuardApplies: only the contracts service consumes the key, so
// explicit service subsets without it are not blocked.
func publicChainKeyGuardApplies(services []string) bool {
	if len(services) == 0 {
		return true
	}
	for _, s := range services {
		if s == "contracts" {
			return true
		}
	}
	return false
}

// CheckPublicChainKey guards up-paths on existing stacks: with the no-default
// interpolation and no resolvable key, the contracts deploy would fall back
// to PRIVATE_KEY_SYSTEM (unfunded on external chains) and die mid-deploy
// with an error naming neither variable. Legacy composes only warn; stacks
// that don't reference the key pass.
func CheckPublicChainKey() error {
	data, err := os.ReadFile("docker-compose.yaml")
	if err != nil {
		return nil // no compose file; the caller produces its own guidance
	}
	compose := string(data)
	if !strings.Contains(compose, docker.PublicChainKeyComposeEnv) {
		warnLegacyEmbeddedKey(compose)
		return nil
	}
	fileVars, err := envfile.Load(".env")
	if err != nil {
		fileVars = map[string]string{}
	}
	value, source, err := resolvePublicChainKey(fileVars)
	if err != nil {
		return err
	}
	if value == "" {
		return missingPublicChainKeyError("the public chain")
	}
	if _, err := normalizePrivateKey(value); err != nil {
		return fmt.Errorf("the public-chain deployer key from %s is invalid: %v", source, err)
	}
	return nil
}
