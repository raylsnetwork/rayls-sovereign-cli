// Package stacks provides functionality for initializing and managing Rayls stack deployments.
// It handles Docker Compose file generation, stack initialization, and service orchestration.
package stacks

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/envfile"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

// postgresInitScript is the script dropped into /docker-entrypoint-initdb.d/
// on postgres. It runs once on first boot and creates the admin
// superuser plus all relayer/KOS/public-relayer databases listed in DB_NAMES.
//
//go:embed postgres-init.sh
var postgresInitScript []byte

// writePostgresInitScript materialises the embedded init script to
// ./docker-init/postgres-init.sh relative to the working directory. The
// compose file bind-mounts this path into postgres.
func writePostgresInitScript() error {
	dir := "docker-init"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, "postgres-init.sh")
	if err := os.WriteFile(path, postgresInitScript, 0755); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// GenerateDockerCompose creates a docker-compose.yaml file for the specified participants.
// It prompts for confirmation if the file already exists and generates a complete Docker
// Compose configuration with all required services. The monitoring parameter controls whether
// to include observability services (Grafana/LGTM stack). The blockscout parameter specifies
// which nodes should have Blockscout explorer enabled. When local is true the
// source-buildable images are used locally instead of pulled from ECR (see localImage).
// Returns (generated bool, error) where generated indicates if a new file was created.
func GenerateDockerCompose(participants []string, monitoring bool, blockscout []string, local bool, publicChain *docker.PublicChain, lean bool, noHub bool) (bool, error) {
	if _, err := os.Stat("docker-compose.yaml"); err == nil {
		// File exists
		fmt.Print("\ndocker-compose.yaml already exists. Overwrite? (Yy/Nn) [n]: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			response := scanner.Text()
			response = strings.ToLower(strings.TrimSpace(response))
			if response != "y" && response != "yes" {
				fmt.Println("Using existing docker-compose.yaml")
				// Re-record the pins even when keeping the file: the kept
				// compose still interpolates ${<X>_SRC:-<git ref>} from .env
				// on every `up`, and missing pins (e.g. .env deleted to
				// reset the key) silently revert builds to 3.0.0-era refs.
				if local {
					if err := pinLocalSources(); err != nil {
						return false, err
					}
				}
				return false, nil // User chose not to overwrite
			}
		}
	} else if !os.IsNotExist(err) {
		// Another error occurred
		return false, err
	}

	// Runs BEFORE any .env mutation (pinLocalSources) so a refused init leaves no
	// trace. Only the new SERVICE SET matters here, and that is independent of
	// build sources — hence the nil-srcs probe generate.
	if err := guardTopologySwitch(docker.GetDemoComposeConfig(participants, monitoring, blockscout, local, publicChain, lean, noHub, nil)); err != nil {
		return false, err
	}

	var srcs *docker.Sources
	if local {
		// Record the pins BEFORE resolving them so this generation, `rayls dev`
		// clones and later regenerations all build from the same sources.
		if err := pinLocalSources(); err != nil {
			return false, err
		}
		var err error
		if srcs, err = docker.ResolveSources(); err != nil {
			return false, err
		}
	}

	composeFile := docker.GetDemoComposeConfig(participants, monitoring, blockscout, local, publicChain, lean, noHub, srcs)

	outputBytes, err := yaml.Marshal(&composeFile)
	if err != nil {
		return false, err
	}

	if err := os.WriteFile("docker-compose.yaml", outputBytes, 0644); err != nil {
		return false, err
	}

	if err := writePostgresInitScript(); err != nil {
		return false, err
	}

	return true, nil
}

// localSourcesRef is the ref --local builds contracts/relayer from when no
// sibling checkout exists: DefaultRefs pin the 3.0.0 images' refs, predating the
// HUB_ENABLED deploy and the hub-less CTS. Enough for the relayer; the contracts
// CLI deploy support (baked /parfin env templates, external-PC targeting) is NOT
// on this ref, so a build from the pin fails its deploy — hence the
// sibling-checkout preference and the warning below.
const localSourcesRef = "version/3.0.1"

const localSourcesMarker = "# --local build sources recorded by `rayls init --local` — override or remove these pins to build from different sources."

// legacyNoHubMarker is what older CLI builds wrote when only hub-less inits
// pinned sources; still recognized so a stack .env gets no second marker.
const legacyNoHubMarker = "# Hub-less build sources recorded by a hub-less `rayls init` — remove these pins when switching this stack back to a hub topology."

func pinLocalSources() error {
	yellow := color.New(color.FgYellow).SprintFunc()
	fileVars, err := envfile.Load(".env")
	if err != nil {
		return err
	}
	pinned := []string{}
	for _, key := range []string{"contracts", "relayer"} {
		comp := docker.ComponentByKey(key)
		srcKey, refKey := comp.EnvPrefix+"_SRC", comp.EnvPrefix+"_REF"
		if src := envfile.Lookup(fileVars, srcKey); src != "" {
			if _, statErr := os.Stat(src); statErr != nil {
				fmt.Printf("%s\n", yellow(fmt.Sprintf("Warning: %s=%s does not exist — the %s build will fail. Fix or remove it from .env.", srcKey, src, comp.Key)))
			}
			continue
		}
		if envfile.Lookup(fileVars, refKey) != "" || envfile.Lookup(fileVars, "RAYLS_VERSION") != "" {
			continue // explicit user pin wins
		}

		checkout := filepath.Join(srcRoot(fileVars), comp.DirName)
		if _, statErr := os.Stat(filepath.Join(checkout, ".git")); statErr == nil {
			if err := envfile.Set(".env", srcKey, checkout); err != nil {
				return err
			}
			pinned = append(pinned, fmt.Sprintf("%s=%s", srcKey, checkout))
			continue
		}
		if err := envfile.Set(".env", refKey, localSourcesRef); err != nil {
			return err
		}
		pinned = append(pinned, fmt.Sprintf("%s=%s", refKey, localSourcesRef))
		if key == "contracts" {
			fmt.Printf("%s\n", yellow("Warning: no ../rayls-privacy-contracts checkout found — falling back to the remote\nversion/3.0.1 git context, which does not yet include the CLI deploy support.\nClone the contracts repo next to this stack (or set CONTRACTS_SRC in .env)\nuntil that support lands upstream."))
		}
	}
	if len(pinned) > 0 {
		if err := ensureLocalSourcesMarker(); err != nil {
			return err
		}
		fmt.Printf("%s\n", yellow(fmt.Sprintf("--local: recorded build sources in .env (the registry's 3.0.0-era refs predate the\nHUB_ENABLED-aware deploy and the hub-less CTS). Override there if you want different\nsources.\n  %s", strings.Join(pinned, "\n  "))))
	}
	return nil
}

func ensureLocalSourcesMarker() error {
	data, err := os.ReadFile(".env")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), localSourcesMarker) || strings.Contains(string(data), legacyNoHubMarker) {
		return nil
	}
	return os.WriteFile(".env", []byte(localSourcesMarker+"\n"+string(data)), 0644)
}

// topologyNeutral reports whether a service can be added or removed without a
// fresh contracts deploy: observability and explorer UIs, unlike bridged services.
func topologyNeutral(name string) bool {
	return name == "otel" || strings.HasPrefix(name, "blockscout-")
}

// renamedServices maps historical service names to their current ones: a pure
// rename is not a topology change (same container role, same volumes), so the
// guard must not force `down -v` on stacks generated before the rename.
var renamedServices = map[string]string{
	"governance-postgres": "postgres",
}

func canonicalServiceName(name string) string {
	if current, ok := renamedServices[name]; ok {
		return current
	}
	return name
}

// topologyDiff returns the topology-defining services added or removed between
// the old and new service sets.
func topologyDiff(oldServices, newServices []string) []string {
	in := func(list []string, name string) bool {
		for _, n := range list {
			if canonicalServiceName(n) == name {
				return true
			}
		}
		return false
	}
	var diff []string
	for _, name := range oldServices {
		if !topologyNeutral(name) && !in(newServices, canonicalServiceName(name)) {
			diff = append(diff, name)
		}
	}
	for _, name := range newServices {
		if !topologyNeutral(name) && !in(oldServices, canonicalServiceName(name)) {
			diff = append(diff, name)
		}
	}
	sort.Strings(diff)
	return diff
}

// guardTopologySwitch refuses to overwrite docker-compose.yaml with a different
// topology while the stack's volumes survive: the one-shot contracts deploy would
// be skipped (see contractsDeployCommand), leaving new services undeployed and env
// files describing the old topology. No old compose or no surviving volumes → no
// guard.
func guardTopologySwitch(newCompose *docker.DockerCompose) error {
	oldServices, err := composeServiceNames()
	if err != nil {
		return nil // no old compose (or unreadable) — nothing to switch from
	}
	newServices := make([]string, 0, len(newCompose.Services))
	for name := range newCompose.Services {
		newServices = append(newServices, name)
	}
	diff := topologyDiff(oldServices, newServices)
	if len(diff) == 0 {
		return nil
	}
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return err
	}
	if len(existingStackVolumes(version)) == 0 {
		return nil // fresh ground — any topology is fine
	}
	return fmt.Errorf("this stack's topology would change (services added/removed: %s) but its volumes\nstill hold the previous deployment — the contracts deploy is one-shot per stack, so the\nresult would be a broken hybrid. Run `rayls down -v` first, then re-run this init",
		strings.Join(diff, ", "))
}

// generatePrivacyNodeOnlyCompose writes a minimal docker-compose.yaml with just
// a single Axyl privacy-node-a. Same overwrite-prompt behaviour as GenerateDockerCompose.
func generatePrivacyNodeOnlyCompose(local bool) (bool, error) {
	if _, err := os.Stat("docker-compose.yaml"); err == nil {
		fmt.Print("\ndocker-compose.yaml already exists. Overwrite? (Yy/Nn) [n]: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			response := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if response != "y" && response != "yes" {
				fmt.Println("Using existing docker-compose.yaml")
				return false, nil
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	composeFile := docker.GetPrivacyNodeOnlyConfig(local)
	if err := guardTopologySwitch(composeFile); err != nil {
		return false, err
	}
	outputBytes, err := yaml.Marshal(&composeFile)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile("docker-compose.yaml", outputBytes, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// InitPrivacyNodeOnly initializes a minimal stack: a single Axyl privacy-node-a
// with no contract deployment or surrounding Rayls services.
// Other CLI commands (start/stop/down/ps/logs) work against the generated
// compose file as usual; verify is unavailable since the contracts container
// isn't present.
func InitPrivacyNodeOnly(local bool) error {
	fmt.Println("Generating docker-compose.yaml for a single privacy node (Axyl)...")
	generated, err := generatePrivacyNodeOnlyCompose(local)
	if err != nil {
		return err
	}
	if generated {
		fmt.Println("✓ docker-compose.yaml generated successfully")
	} else {
		fmt.Println("✓ Using existing docker-compose.yaml")
	}

	fmt.Println("\nChecking Docker environment...")
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return err
	}
	if version == docker.ComposeV2 {
		fmt.Println("✓ Using Docker Compose V2")
	} else {
		fmt.Println("✓ Using Docker Compose V1")
	}

	if local {
		// Axyl is this stack's only image and --local marks it pull_policy=never,
		// so there is nothing for the compose pull step to do.
		if err := ensureAxylImage(); err != nil {
			return err
		}
	} else {
		fmt.Println("\nPulling container images sequentially (this may take a few minutes)...")
		if err := pullImagesSequentially(version); err != nil {
			return fmt.Errorf("failed to pull images: %w", err)
		}
		fmt.Println("✓ All images pulled successfully")
	}

	fmt.Println("\nStarting services...")
	if err := startStack(version, false); err != nil {
		return err
	}

	fmt.Println("\n✓ Privacy node started!")
	fmt.Println("\nAccess Endpoints:")
	fmt.Println("-----------------")
	fmt.Println("- Privacy Node RPC: http://localhost:8545")
	printDemoCommands(nil)
	return nil
}

// ensureAxylImage guarantees rayls-privacy-axyl:latest exists locally in
// --local mode. The Axyl node is a 30-90 minute Rust build, so unlike the Go
// services it is not built from a git context: the published multi-arch image
// is pulled and retagged instead. Developers changing node code build the
// image themselves and this function then leaves theirs alone. Override the
// pulled image with RAYLS_AXYL_IMAGE (env or stack .env).
func ensureAxylImage() error {
	if exec.Command("docker", "image", "inspect", "rayls-privacy-axyl:latest").Run() == nil {
		return nil
	}
	fileVars, err := envfile.Load(".env")
	if err != nil {
		return err
	}
	source := envfile.Lookup(fileVars, "RAYLS_AXYL_IMAGE")
	if source == "" {
		source = "public.ecr.aws/w0k9o1t3/rayls-demo/rayls-privacy-axyl:latest"
	}
	fmt.Printf("rayls-privacy-axyl:latest not found locally — pulling %s...\n", source)
	if err := dockerPull(source); err != nil {
		return fmt.Errorf("pulling axyl node image: %w", err)
	}
	if err := exec.Command("docker", "tag", source, "rayls-privacy-axyl:latest").Run(); err != nil {
		return fmt.Errorf("tagging axyl node image: %w", err)
	}
	return nil
}

// ECR Public throttles unauthenticated pulls to ~1/s per source IP (Docker Hub
// caps them per 6h window): registry steps pull serially and cool off when throttled.
const (
	pullMaxAttempts     = 4
	pullBackoffInterval = 10 * time.Second
)

func pullBackoff(attempt int) time.Duration {
	return pullBackoffInterval << (attempt - 1)
}

var rateLimitSignatures = []string{
	"toomanyrequests",
	"rate exceeded",
	"pull rate limit",
	"429 too many requests",
}

func isRateLimited(output string) bool {
	return containsAny(output, rateLimitSignatures)
}

// permanentFailureSignatures are rejections another attempt can't fix (a stale
// ECR login answers 403, an unpublished tag "manifest unknown"); everything
// else — throttling, resets, timeouts, 5xx — is retried.
var permanentFailureSignatures = []string{
	"403 forbidden",
	"pull access denied",
	"unauthorized",
	"manifest unknown",
	"repository does not exist",
	"no such host",
	"invalid reference format",
}

func isPermanentFailure(output string) bool {
	return containsAny(output, permanentFailureSignatures)
}

// worthRetrying: throttling is always retried, even when the same run also logged
// a permanent rejection for some other image.
func worthRetrying(output string) bool {
	return isRateLimited(output) || !isPermanentFailure(output)
}

func containsAny(output string, signatures []string) bool {
	lower := strings.ToLower(output)
	for _, sig := range signatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

const rateLimitHint = "The registry is rate-limiting this IP. ECR Public allows ~1 unauthenticated image\n" +
	"pull per second per IP — a limit shared by everyone behind the same NAT/VPN.\n" +
	"Options:\n" +
	"  • re-run the command in a minute — fetched layers are cached, so it resumes\n" +
	"  • authenticate to raise the limit to 10 pulls/s (the images stay public):\n" +
	"      aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws\n" +
	"  • pass --no-pull to run with the images you already have locally"

// staleLoginHint: an expired `docker login public.ecr.aws` token (~12h) keeps being
// sent instead of falling back to anonymous access, so pulling these *public*
// images answers `403 Forbidden`.
const staleLoginHint = "If you saw a `403 Forbidden` on a public.ecr.aws image, it's almost always a\n" +
	"stale ECR Public login token. The Rayls images are public — fix it with either:\n" +
	"  docker logout public.ecr.aws    # pull anonymously (simplest)\n" +
	"  aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws    # refresh the token"

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// composeArgs builds the argv for a compose invocation. serial adds compose v2's
// `--parallel 1`, a global flag that must precede the subcommand; v1 has no
// equivalent and relies on COMPOSE_PARALLEL_LIMIT (set in runCompose).
func composeArgs(version docker.DockerComposeVersion, serial bool, args ...string) (string, []string) {
	if version != docker.ComposeV2 {
		return "docker-compose", args
	}
	argv := []string{"compose"}
	if serial {
		argv = append(argv, "--parallel", "1")
	}
	return "docker", append(argv, args...)
}

// runCompose runs a compose subcommand with stderr both shown and captured — that's
// where compose writes progress and errors, and callers scan it for rate limits.
// Teeing through a pipe drops compose to plain progress output, hence
// COMPOSE_PROGRESS=tty when there's a terminal: an env var rather than `--progress`
// because older compose builds ignore an unknown variable but reject an unknown flag.
func runCompose(version docker.DockerComposeVersion, serial bool, args ...string) (string, error) {
	name, argv := composeArgs(version, serial, args...)
	cmd := exec.Command(name, argv...)
	cmd.Env = os.Environ()
	if serial {
		cmd.Env = append(cmd.Env, "COMPOSE_PARALLEL_LIMIT=1")
	}
	if isCharDevice(os.Stderr) && os.Getenv("COMPOSE_PROGRESS") == "" {
		cmd.Env = append(cmd.Env, "COMPOSE_PROGRESS=tty")
	}

	var captured bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &captured)

	err := cmd.Run()
	return captured.String(), err
}

// dockerPull pulls one image with cool-off retries. Unlike compose, docker sends
// pull progress to stdout and errors to stderr, so only stderr is captured.
func dockerPull(image string) error {
	yellow := color.New(color.FgYellow).SprintFunc()

	var lastOutput string
	var lastErr error
	for attempt := 1; attempt <= pullMaxAttempts; attempt++ {
		var captured bytes.Buffer
		cmd := exec.Command("docker", "pull", image)
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &captured)

		err := cmd.Run()
		if err == nil {
			return nil
		}
		lastOutput, lastErr = captured.String(), err
		if attempt == pullMaxAttempts || !worthRetrying(lastOutput) {
			break
		}
		wait := pullBackoff(attempt)
		reason := "Pull of " + image + " failed"
		if isRateLimited(lastOutput) {
			reason = "Registry rate limit pulling " + image
		}
		fmt.Printf("\n%s\n", yellow(fmt.Sprintf("⚠ %s (attempt %d/%d) — retrying in %s...", reason, attempt, pullMaxAttempts, wait)))
		time.Sleep(wait)
	}

	if isRateLimited(lastOutput) {
		return fmt.Errorf("%w\n\n%s", lastErr, rateLimitHint)
	}
	return lastErr
}

// pullImagesSequentially pulls the stack's images one at a time to stay under ECR
// Public's per-IP limit. Still needed in --local: compose pull skips services that
// build from source or set pull_policy=never, leaving exactly the infra images.
func pullImagesSequentially(version docker.DockerComposeVersion) error {
	yellow := color.New(color.FgYellow).SprintFunc()

	var lastOutput string
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= pullMaxAttempts; attempt++ {
		output, err := runCompose(version, true, "pull")
		if err == nil {
			return nil
		}
		attempts, lastOutput, lastErr = attempt, output, err
		if attempt == pullMaxAttempts || !worthRetrying(output) {
			break
		}
		wait := pullBackoff(attempt)
		reason := "Pull failed"
		if isRateLimited(output) {
			reason = "Registry rate limit"
		}
		fmt.Printf("\n%s\n", yellow(fmt.Sprintf("⚠ %s (attempt %d/%d) — retrying in %s; layers already fetched are kept.", reason, attempt, pullMaxAttempts, wait)))
		time.Sleep(wait)
	}

	hint := staleLoginHint
	if isRateLimited(lastOutput) {
		hint = rateLimitHint
	}
	return fmt.Errorf("pull failed after %d attempt(s): %w\n\n%s", attempts, lastErr, hint)
}

// StartServices runs `docker compose up -d [service...]` and returns the captured
// output so callers can map failures to friendlier messages. `up` fetches missing
// images in parallel, so callers that skipped the pull step pass serial=true; a
// throttled run is retried serialized (up is idempotent — containers already
// created are left alone).
func StartServices(version docker.DockerComposeVersion, serial bool, services ...string) (string, error) {
	yellow := color.New(color.FgYellow).SprintFunc()

	// Choke point for every up-path: refuse a missing user key (e.g. stack
	// .env deleted since init) instead of letting the contracts deploy die
	// mid-run on an unfunded fallback. Skipped when the named services
	// exclude contracts, the key's only consumer.
	if publicChainKeyGuardApplies(services) {
		if err := CheckPublicChainKey(); err != nil {
			return "", err
		}
	}

	// --remove-orphans: the compose here is regenerated per init, so a service no
	// longer in it is stale — and left running it pins the old volumes, defeating
	// a later `down -v`.
	args := append([]string{"up", "-d", "--remove-orphans"}, services...)

	output, err := runCompose(version, serial, args...)
	for attempt := 1; err != nil && isRateLimited(output) && attempt < pullMaxAttempts; attempt++ {
		wait := pullBackoff(attempt)
		fmt.Printf("\n%s\n", yellow(fmt.Sprintf("⚠ Registry rate limit while starting (attempt %d/%d) — waiting %s, then retrying one image at a time.", attempt, pullMaxAttempts, wait)))
		time.Sleep(wait)
		output, err = runCompose(version, true, args...)
	}
	return output, err
}

// existingStackVolumes lists the compose project volumes docker already has, so init
// can warn it is resuming a stack: the contracts deploy is one-shot per stack (see
// contractsDeployCommand) and everything else is keyed to those addresses.
// Compose v1 has no `config --format json`, so there it returns nothing.
func existingStackVolumes(version docker.DockerComposeVersion) []string {
	if version != docker.ComposeV2 {
		return nil
	}
	out, err := exec.Command("docker", "compose", "config", "--format", "json").Output()
	if err != nil {
		return nil
	}
	var project struct {
		Name    string `json:"name"`
		Volumes map[string]struct {
			Name string `json:"name"`
		} `json:"volumes"`
	}
	if err := json.Unmarshal(out, &project); err != nil {
		return nil
	}

	var existing []string
	for key, vol := range project.Volumes {
		name := vol.Name
		if name == "" {
			name = project.Name + "_" + key
		}
		if exec.Command("docker", "volume", "inspect", name).Run() == nil {
			existing = append(existing, name)
		}
	}
	sort.Strings(existing)
	return existing
}

func startStack(version docker.DockerComposeVersion, serial bool) error {
	output, err := StartServices(version, serial)
	if err == nil {
		return nil
	}
	if isRateLimited(output) {
		return fmt.Errorf("failed to start stack: %w\n\n%s", err, rateLimitHint)
	}
	return fmt.Errorf("failed to start stack: %w", err)
}

// InitStack initializes a complete Rayls stack with the specified number of participants.
// It generates the docker-compose.yaml file, checks the Docker environment, and starts
// all services using docker compose up. Displays a security warning about hardcoded keys
// and prints access endpoints after successful initialization. The blockscout parameter
// specifies which nodes should have Blockscout explorer enabled.
func InitStack(participants []string, monitoring bool, blockscout []string, local bool, publicChain *docker.PublicChain, lean bool, noHub bool, noPull bool) error {
	red := color.New(color.FgRed, color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Printf("\n%s\n", red("!!! SECURITY WARNING !!!"))
	fmt.Printf("%s\n\n", yellow("This is a DEMO stack. The in-stack chains and databases use well-known development\nkeys and passwords identical for every install. Do NOT use in a real environment.\nOnly for testing purposes and presentations."))

	if publicChain != nil {
		if publicChain.Local {
			fmt.Printf("Public chain: local Axyl node inside the stack (service public-chain, chain id %d) — no external connectivity needed.\n", publicChain.ChainID)
		} else {
			fmt.Printf("Public chain: %s (%s, chain id %d)\n", publicChain.Name, publicChain.RPC, publicChain.ChainID)
		}
	}

	fmt.Printf("Generating docker-compose.yaml for %d participants...\n", len(participants))
	if len(blockscout) > 0 {
		fmt.Printf("Blockscout enabled for nodes: %s\n", strings.Join(blockscout, ", "))
	}
	generated, err := GenerateDockerCompose(participants, monitoring, blockscout, local, publicChain, lean, noHub)
	if err != nil {
		return err
	}
	if generated {
		fmt.Println("✓ docker-compose.yaml generated successfully")
	} else {
		fmt.Println("✓ Using existing docker-compose.yaml")
	}

	// External chains deploy from the USER's funded key: resolve it (env >
	// stack .env), prompt + persist when absent, preflight the balance.
	// After generation (acts on the kept-or-new compose file), before the
	// slow docker steps.
	if publicChain != nil && !publicChain.Local {
		if err := ensureStackPublicChainKey(publicChain, len(participants)); err != nil {
			return err
		}
	}

	fmt.Println("\nChecking Docker environment...")
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return err
	}

	if version == docker.ComposeV2 {
		fmt.Println("✓ Using Docker Compose V2")
	} else {
		fmt.Println("✓ Using Docker Compose V1")
	}

	if volumes := existingStackVolumes(version); len(volumes) > 0 {
		fmt.Printf("\n%s\n", yellow(fmt.Sprintf("Found state from a previous stack (%d volumes). Its deployed contracts, chain data and\ndatabases are reused — the contracts deploy is one-shot per stack. Run `rayls down -v`\nfirst to wipe them and deploy from scratch (required to pick up rebuilt contracts).", len(volumes))))
		if noHub {
			// Reused .X.env files still carry PNH_DEPLOYMENT_PROXY_REGISTRY, so the
			// CTS boots hub-ENABLED against an absent private-hub (see applyNoHub).
			fmt.Printf("%s\n", yellow("hub-less init with reused volumes: if the previous stack had a hub, its env files still\ncarry the PNH config and the CTS will try to reach the now-absent private-hub.\nSwitching topology requires `rayls down -v` first."))
		}
	}

	if local {
		fmt.Printf("\n%s\n", yellow("--local set: kos/pubrelayer/contracts build from source (pinned git contexts;\noverride per component via <COMPONENT>_SRC in .env or `rayls dev`). First build may take several minutes."))
		if err := ensureAxylImage(); err != nil {
			return err
		}
	}

	if noPull {
		fmt.Printf("\n%s\n", yellow("--no-pull set: skipping the pull step. `up` will fetch only images missing locally, so a locally-built image (e.g. a custom contracts build) is used as-is."))
	} else {
		// One image at a time to stay under AWS ECR Public's per-IP rate limit.
		// --local needs it too: only the source-built components are skipped, the
		// infra images still come from the registry and `up` would fetch them at once.
		if local {
			fmt.Println("\nPulling infra images sequentially (source-built components are skipped)...")
		} else {
			fmt.Println("\nPulling container images sequentially (this may take a few minutes)...")
		}
		if err := pullImagesSequentially(version); err != nil {
			return fmt.Errorf("failed to pull images: %w", err)
		}
		fmt.Println("✓ All images pulled successfully")
	}

	// Without the pull step, `up` is what fetches images — keep it serialized.
	fmt.Println("\nStarting services...")
	if err := startStack(version, noPull); err != nil {
		return err
	}

	fmt.Println("\n✓ Stack initialized successfully!")
	PrintAccessEndpoints(participants, monitoring, blockscout, publicChain, lean, noHub)
	printDemoCommands(publicChain)
	return nil
}

// PrintAccessEndpoints displays a formatted list of service URLs for accessing the stack.
// Shows shared services and per-participant services (privacy nodes, relayers, KOS) with
// their respective ports. Includes Grafana URL if monitoring is enabled. Shows Blockscout
// URLs for enabled nodes. In lean mode the full-stack-only services (audit explorer,
// governance API) are omitted; the minimal PNH stays — the 3.0.0 CTS
// stack deploys it even in lean. With noHub the PNH and the endpoints riding on it
// are omitted entirely.
func PrintAccessEndpoints(participants []string, monitoring bool, blockscout []string, publicChain *docker.PublicChain, lean bool, noHub bool) {
	fmt.Println("\nAccess Endpoints:")
	fmt.Println("-----------------")

	// Shared Services
	fmt.Println("Shared Services:")
	if !lean && !noHub {
		fmt.Println("- Block Explorer:      http://localhost:8181")
	}
	if !noHub {
		fmt.Println("- Private Network Hub: http://localhost:3445")
	}
	if !lean && !noHub {
		fmt.Println("- Governance API:      http://localhost:9100")
	}
	if monitoring {
		fmt.Println("- Grafana:             http://localhost:3300")
	}
	if publicChain != nil {
		if publicChain.Local {
			fmt.Printf("- Public Chain RPC:    http://localhost:%d (chain id %d, local Axyl)\n", docker.LocalPublicChainPort, publicChain.ChainID)
		} else {
			fmt.Printf("- Public Chain RPC:    %s (chain id %d)\n", publicChain.RPC, publicChain.ChainID)
		}
		if publicChain.Faucet != "" {
			fmt.Printf("- Public Chain Faucet: %s\n", publicChain.Faucet)
		}
		if publicChain.Explorer != "" {
			fmt.Printf("- Public Chain Explorer: %s\n", publicChain.Explorer)
		}
	}
	fmt.Println("")

	// Per Participant Services
	fmt.Println("Participant Services:")
	for i, p := range participants {
		pUpper := strings.ToUpper(p)
		fmt.Printf("[%s]\n", pUpper)
		fmt.Printf("- Privacy Node:        http://localhost:%d\n", 8545+i)
		// Kept in lean too — applyLeanNoPNH drops governance/explorer but
		// keeps relayer-<p>; only hub-less stacks lose it.
		if !noHub {
			fmt.Printf("- Relayer:             http://localhost:%d\n", 9000+i)
		}
		fmt.Printf("- KOS (CTS):           http://localhost:%d\n", 8080+i)
		if publicChain != nil {
			fmt.Printf("- PubRelayer:          http://localhost:%d\n", 9050+i)
		}

		// Check if this node has Blockscout enabled
		for bsIdx, bsNode := range blockscout {
			if bsNode == p {
				blockscoutPort := 10004 + (bsIdx * 100)
				fmt.Printf("- Blockscout:          http://localhost:%d\n", blockscoutPort)
				break
			}
		}

		if i < len(participants)-1 {
			fmt.Println("")
		}
	}

	// Blockscout Summary if any enabled
	if len(blockscout) > 0 {
		fmt.Println("")
		fmt.Println("Blockscout Explorers:")
		for i, node := range blockscout {
			port := 10004 + (i * 100)
			fmt.Printf("- Node %s: http://localhost:%d\n", strings.ToUpper(node), port)
		}
	}
}

// printDemoCommands prints a short cheat-sheet of useful commands to run once
// the stack is up, so users exploring the demo know what to do next. The
// public-chain-only entries are shown when a public chain is configured.
func printDemoCommands(publicChain *docker.PublicChain) {
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Println("\nUseful commands:")
	fmt.Println("----------------")
	fmt.Printf("  %s   # Show service status\n", cyan("rayls ps"))
	fmt.Printf("  %s   # Follow logs from all services\n", cyan("rayls logs -f"))
	fmt.Printf("  %s   # Follow logs from one service (e.g. kos-a)\n", cyan("rayls logs -f <service>"))
	fmt.Printf("  %s   # Run the contracts E2E test suite\n", cyan("rayls verify contracts"))
	if publicChain != nil {
		fmt.Printf("  %s   # Smoke-test the private -> public chain bridge\n", cyan("rayls verify public-chain"))
	}
	fmt.Println()
	fmt.Printf("  %s   # Stop services (data is preserved)\n", cyan("rayls stop"))
	fmt.Printf("  %s   # Start services again (honors dependency ordering; reuses the deployed contracts)\n", cyan("rayls start"))
	fmt.Printf("  %s   # Remove containers/networks, keep volumes (data preserved)\n", cyan("rayls down"))
	fmt.Printf("  %s   # Remove everything including volumes (wipes all data; needed to redeploy contracts)\n", cyan("rayls down -v"))
}
