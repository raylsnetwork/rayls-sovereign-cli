package docker

import (
	"fmt"
	"strings"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/envfile"
)

// ServiceBuild describes how one compose service builds from a component's
// source tree: the production Dockerfile (used by --local git-context builds;
// byte-equivalent to the published ECR images) and the air-based dev variant
// plus its air config (used by `rayls dev` hot reload).
type ServiceBuild struct {
	Dockerfile    string // production build (what the ECR image is built from)
	DevDockerfile string // air-based dev build ("" = component has no hot reload)
	AirConfig     string // air config path inside the repo, e.g. "cts/air.toml"
}

// Component describes one buildable Rayls source component: the repo it lives
// in, the env-var prefix that overrides it, and how its services are built.
// A single component may produce several images (the relayer-api repo builds
// both the CTS/kos and the public-relayer).
type Component struct {
	Key       string // CLI-facing name used by `rayls dev` (e.g. "relayer")
	EnvPrefix string // prefix for the _SRC/_REF/_REPO override vars (e.g. "RELAYER")
	Repo      string // default git URL
	DirName   string // directory name used for local checkouts
	// GlossaryPath is the /parfin/<component> directory the contracts deploy
	// writes this component's per-participant env files to and the service
	// reads via --env / ENV_FILE. It is the container-side config path and is
	// deliberately decoupled from the source repo/checkout name (DirName): the
	// GitHub repos were renamed to rayls-sovereign-*, but the deploy still
	// writes the historical rayls-privacy-* glossary paths. Empty for
	// components that have no per-participant env files (e.g. contracts).
	GlossaryPath string
	DefaultRef   string // git ref to build from (the sovereign repos' main == 3.0.1 code)
	Watch        bool   // participates in hot reload (`rayls dev` + `docker compose watch`)

	// Services maps compose service-name prefixes built from this repo to
	// their build configuration.
	Services map[string]ServiceBuild
}

// Components is the registry of Rayls components buildable from source in
// --local mode. Each points at its GitHub repo on `main`: the rayls-sovereign-*
// repos hold the 3.0.1 code copied over as a single `main` branch (no version
// tags yet), so `main` is the build ref.
//
// TODO(sovereign-public): the rayls-sovereign-* repos are currently PRIVATE, so
// the default context uses SSH (git@...). BuildSection adds build.ssh:[default]
// for git@/ssh:// URLs, so `docker compose up --build` forwards your ssh-agent
// — run `ssh-add -l` and make sure a key with raylsnetwork access is loaded.
// Once the admin makes the repos public, switch every Repo below back to
// https://github.com/raylsnetwork/<name>.git (anonymous clone, no ssh-agent);
// BuildSection then drops the ssh forwarding automatically. See also the
// matching notes in README.md / DEV_MODE.md.
//
// Infra images (nats, private-network-hub, axyl) are not source-built — see
// localImage/ensureAxylImage.
var Components = []Component{
	{
		Key:        "contracts",
		EnvPrefix:  "CONTRACTS",
		Repo:       "git@github.com:raylsnetwork/rayls-sovereign-contracts.git",
		DirName:    "rayls-sovereign-contracts",
		DefaultRef: "main",
		Watch:      false, // contract changes need an explicit redeploy, not a file watcher
		Services: map[string]ServiceBuild{
			// Dockerfile.dev is the deploy-tooling image (the "dev" naming is
			// historical). On sovereign-contracts main it bakes the /parfin env
			// templates and honors HUB_ENABLED/OPS_API_ENABLED/GOVERNANCE_ENABLED.
			"contracts": {Dockerfile: "Dockerfile.dev"},
		},
	},
	{
		Key:          "relayer",
		EnvPrefix:    "RELAYER",
		Repo:         "git@github.com:raylsnetwork/rayls-sovereign-relayer.git",
		DirName:      "rayls-sovereign-relayer",
		GlossaryPath: relayerPathV3, // deploy still writes /parfin/rayls-privacy-relayer-api
		// The sovereign repos carry only a single `main` branch (the copied-over
		// 3.0.1 code); no version tags exist yet. Override per stack via
		// RELAYER_REF once tags land.
		DefaultRef: "main",
		Watch:      true,
		Services: map[string]ServiceBuild{
			"kos":        {Dockerfile: "cts/Dockerfile", DevDockerfile: "cts/Dockerfile.dev", AirConfig: "cts/air.toml"},
			"pubrelayer": {Dockerfile: "public-relayer/Dockerfile", DevDockerfile: "public-relayer/Dockerfile.dev", AirConfig: "public-relayer/air.toml"},
			// The private relayer (compose service relayer-<p>) now lives in this
			// same repo under private-relayer/ (it was the repo root pre-3.0.1).
			"relayer": {Dockerfile: "private-relayer/Dockerfile", DevDockerfile: "private-relayer/Dockerfile.dev", AirConfig: "private-relayer/air.toml"},
		},
	},
	{
		Key:          "governance",
		EnvPrefix:    "GOVERNANCE",
		Repo:         "git@github.com:raylsnetwork/rayls-sovereign-pnh-governance.git",
		DirName:      "rayls-sovereign-pnh-governance",
		GlossaryPath: governancePathV3, // deploy writes /parfin/rayls-privacy-pnh-governance-api
		DefaultRef:   "main",
		Watch:        false, // no air wiring here; governance changes need an explicit rebuild
		Services: map[string]ServiceBuild{
			"governance-api":      {Dockerfile: "Dockerfile.api"},
			"governance-listener": {Dockerfile: "Dockerfile.listener"},
			"governance-flagger":  {Dockerfile: "Dockerfile.flagger"},
		},
	},
	{
		Key:        "gnark",
		EnvPrefix:  "GNARK",
		Repo:       "git@github.com:raylsnetwork/rayls-sovereign-gnark-api.git",
		DirName:    "rayls-sovereign-gnark-api",
		DefaultRef: "main",
		Watch:      false,
		Services: map[string]ServiceBuild{
			// gnark's proving/verifying keys under last_build/ are Git-LFS blobs.
			// BuildKit's git-context clone does NOT smudge LFS, so a pinned
			// git-context build ships pointer files and the server fails to load
			// keys. Use `rayls dev gnark` (clones + `git lfs pull`, see
			// ensureCheckout) or the pulled ECR image for a working proofs-api.
			"proofs-api": {Dockerfile: "Dockerfile"},
		},
	},
	{
		Key:        "auditor",
		EnvPrefix:  "AUDITOR",
		Repo:       "git@github.com:raylsnetwork/rayls-sovereign-pnh-auditor-ui.git",
		DirName:    "rayls-sovereign-pnh-auditor-ui",
		DefaultRef: "main",
		Watch:      false, // Angular -> nginx; no hot reload
		Services: map[string]ServiceBuild{
			"audit-explorer": {Dockerfile: "Dockerfile"},
		},
	},
}

// ComponentByKey returns the component registered under key, or nil.
func ComponentByKey(key string) *Component {
	for i := range Components {
		if Components[i].Key == key {
			return &Components[i]
		}
	}
	return nil
}

// Sources resolves where each component's source comes from. Resolution order
// per component (process env wins over .env, matching compose):
//
//	ref:  <PREFIX>_REF  >  v<RAYLS_VERSION>  >  the component's DefaultRef
//	repo: <PREFIX>_REPO >  the registry default
//
// The generated compose additionally wraps every build context in
// ${<PREFIX>_SRC:-...} so a local checkout path (managed by `rayls dev`, or
// set by hand in .env) overrides the git context without regenerating.
type Sources struct {
	fileVars map[string]string
}

// ResolveSources loads ./.env (compose's interpolation file in the stack
// directory) and returns a Sources resolver over it plus the process env.
func ResolveSources() (*Sources, error) {
	vars, err := envfile.Load(".env")
	if err != nil {
		return nil, fmt.Errorf("reading .env: %w", err)
	}
	return &Sources{fileVars: vars}, nil
}

// lookup applies the process-env-over-.env precedence.
func (s *Sources) lookup(key string) string {
	return envfile.Lookup(s.fileVars, key)
}

// Ref returns the git ref component c builds from.
func (s *Sources) Ref(c Component) string {
	if ref := s.lookup(c.EnvPrefix + "_REF"); ref != "" {
		return ref
	}
	if v := s.lookup("RAYLS_VERSION"); v != "" {
		return "v" + v
	}
	return c.DefaultRef
}

// Repo returns the git URL component c is fetched from.
func (s *Sources) Repo(c Component) string {
	if repo := s.lookup(c.EnvPrefix + "_REPO"); repo != "" {
		return repo
	}
	return c.Repo
}

// Src returns the local source override for component c ("" when the
// component builds from its git context).
func (s *Sources) Src(c Component) string {
	return s.lookup(c.EnvPrefix + "_SRC")
}

// BuildContext returns the compose build context for component c: the
// <PREFIX>_SRC variable (a local checkout path or any git URL) with the
// pinned git context as its default. Compose interpolates the variable at
// run time, so flipping a component to a local checkout is a .env edit — no
// regeneration needed.
func (s *Sources) BuildContext(c Component) string {
	return fmt.Sprintf("${%s_SRC:-%s#%s}", c.EnvPrefix, s.Repo(c), s.Ref(c))
}

// BuildSection returns the compose build section for the service prefix
// (e.g. "kos") of the component registered under key, using the PRODUCTION
// dockerfile (the dev variant is applied by the `rayls dev` override).
// ssh-agent forwarding is only requested when the default context is an SSH
// git URL — once the repos are public over https it drops out automatically.
func (s *Sources) BuildSection(key, servicePrefix string) *Build {
	c := ComponentByKey(key)
	if c == nil {
		return nil
	}
	sb, ok := c.Services[servicePrefix]
	if !ok {
		return nil
	}
	b := &Build{
		Context:    s.BuildContext(*c),
		Dockerfile: sb.Dockerfile,
	}
	repo := s.Repo(*c)
	if strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://") {
		b.Ssh = []string{"default"}
	}
	return b
}
