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
	Key        string // CLI-facing name used by `rayls dev` (e.g. "relayer")
	EnvPrefix  string // prefix for the _SRC/_REF/_REPO override vars (e.g. "RELAYER")
	Repo       string // default git URL
	DirName    string // directory name used for local checkouts
	DefaultRef string // ref the 3.0.0 stack images are built from
	Watch      bool   // participates in hot reload (`rayls dev` + `docker compose watch`)

	// Services maps compose service-name prefixes built from this repo to
	// their build configuration.
	Services map[string]ServiceBuild
}

// Components is the registry of Rayls components buildable from source in
// --local mode, pinned to the same refs the published 3.0.0 ECR images were
// built from. Infra images (nats, private-network-hub, the private relayer,
// proofs-api, axyl) are not source-built — see localImage/ensureAxylImage.
var Components = []Component{
	{
		Key:        "contracts",
		EnvPrefix:  "CONTRACTS",
		Repo:       "git@github.com:raylsnetwork/rayls-privacy-contracts.git",
		DirName:    "rayls-privacy-contracts",
		DefaultRef: "lean-no-pnh-3.0.0",
		Watch:      false, // contract changes need an explicit redeploy, not a file watcher
		Services: map[string]ServiceBuild{
			// Dockerfile.dev is what the published lean-no-pnh image is built
			// from (the deploy tooling image; "dev" is historical naming).
			"contracts": {Dockerfile: "Dockerfile.dev"},
		},
	},
	{
		Key:        "relayer",
		EnvPrefix:  "RELAYER",
		Repo:       "git@github.com:raylsnetwork/rayls-privacy-relayer-api.git",
		DirName:    "rayls-privacy-relayer-api",
		// v3.0.0 is an immutable tag (== version/3.0.0 head); version/3.0.1
		// is still in development. Override per stack via RELAYER_REF.
		DefaultRef: "v3.0.0",
		Watch:      true,
		Services: map[string]ServiceBuild{
			"kos":        {Dockerfile: "cts/Dockerfile", DevDockerfile: "cts/Dockerfile.dev", AirConfig: "cts/air.toml"},
			"pubrelayer": {Dockerfile: "public-relayer/Dockerfile", DevDockerfile: "public-relayer/Dockerfile.dev", AirConfig: "public-relayer/air.toml"},
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
