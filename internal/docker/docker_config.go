package docker

import (
	"fmt"
	"strings"
)

type HealthCheck struct {
	Test        []string `yaml:"test,omitempty"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

type Build struct {
	Context    string            `yaml:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
	// Ssh forwards the host's ssh-agent into the build (`ssh: [default]`).
	// Needed when Context is a private git URL: BuildKit clones the repo
	// inside the daemon using the forwarded agent, so SSH_AUTH_SOCK must be
	// set in the invoking shell.
	Ssh []string `yaml:"ssh,omitempty"`
}

// type EnvFileEntry struct {
// 	Path     string `yaml:"path"`
// 	Required bool   `yaml:"required,omitempty"`
// }

type WatchAction struct {
	Action string `yaml:"action,omitempty"`
	Path   string `yaml:"path,omitempty"`
	Target string `yaml:"target,omitempty"`
}

type Develop struct {
	Watch []*WatchAction `yaml:"watch,omitempty"`
}

type NetworkConfig struct {
	Aliases []string `yaml:"aliases,omitempty"`
}

type Service struct {
	Image           string                    `yaml:"image,omitempty"`
	PullPolicy      string                    `yaml:"pull_policy,omitempty"`
	Command         string                    `yaml:"command,omitempty"`
	ExtraHosts      []string                  `yaml:"extra_hosts,omitempty"`
	Volumes         []string                  `yaml:"volumes,omitempty"`
	Networks        map[string]*NetworkConfig `yaml:"networks,omitempty"`
	Ports           []string                  `yaml:"ports,omitempty"`
	HealthCheck     interface{}               `yaml:"healthcheck,omitempty"`
	DependsOn       map[string]interface{}    `yaml:"depends_on,omitempty"`
	Build           *Build                    `yaml:"build,omitempty"`
	EntryPoint      []string                  `yaml:"entrypoint,omitempty"`
	Environment     interface{}               `yaml:"environment,omitempty"` // Can be map or array
	ContainerName   string                    `yaml:"container_name,omitempty"`
	Restart         string                    `yaml:"restart,omitempty"`
	User            string                    `yaml:"user,omitempty"`
	Platform        string                    `yaml:"platform,omitempty"`
	StopGracePeriod string                    `yaml:"stop_grace_period,omitempty"`
	//EnvFile       interface{}            `yaml:"env_file,omitempty"` // Can be []string or []EnvFileEntry
	ShmSize    string   `yaml:"shm_size,omitempty"`
	WorkingDir string   `yaml:"working_dir,omitempty"`
	Develop    *Develop `yaml:"develop,omitempty"`
}

type DockerCompose struct {
	Services map[string]*Service    `yaml:"services"`
	Volumes  map[string]interface{} `yaml:"volumes,omitempty"`
	Networks map[string]interface{} `yaml:"networks,omitempty"`
}

// raylsECRPrefix is the public ECR repository prefix used by every Rayls-owned
// image. When --local is passed we strip this prefix so docker uses a locally
// built tag of the same short name (e.g. `rayls-kos:latest`) and set
// pull_policy=never to guarantee docker won't fall back to the registry.
const raylsECRPrefix = "public.ecr.aws/w0k9o1t3/rayls-demo/"

// New-glossary config paths used by the rayls-contracts v3.0.0 image. The
// deploy writes per-participant relayer/governance env files here and the
// CTS, relayers and governance services read them from the same paths. Both
// lean and full mode use these now — the 3.0.0 image (`:latest` and
// `:lean-no-pnh`, same digest) only bakes config at these V3 paths.
const (
	relayerPathV3    = "/parfin/rayls-privacy-relayer-api"
	governancePathV3 = "/parfin/rayls-privacy-pnh-governance-api"
)

// PublicChain describes the blockchain the stack bridges to. When set,
// contracts deploy against this RPC and pubrelayer services are generated and
// wired to it. Local marks the `local` preset: the chain is an
// Axyl container the generator emits into the stack itself (RPC is its
// compose-network address), so the whole system runs on one host with no
// external dependency — the deploy still treats it exactly like an external
// chain (env-driven `public_chain` hardhat network), just one it can reach by
// service name.
type PublicChain struct {
	Name     string // preset identifier (e.g. "rayls-testnet"), surfaced in endpoints output
	RPC      string
	ChainID  int
	Faucet   string // optional, printed in access endpoints
	Explorer string // optional, block explorer URL printed in access endpoints
	Local    bool   // run the chain as a local axyl container (service `public-chain`)
}

// Local public-chain conventions, matching the relayer repo's dev stack
// (docker-compose.dev-local.yml runs its local PC at public-chain:8845 with
// chain id 7331, and the deploy image's hardhat config carries the same
// numbers in its localPC network) so tooling assumptions carry over.
const (
	LocalPublicChainPort    = 8845
	localPublicChainChainID = 7331
)

// FundingURL is where users request testnet RAYLS for their own deployer key
// (funding happens outside the CLI).
const FundingURL = "https://www.rayls.com/community"

// PublicChainKeyComposeEnv is the contracts env entry for external public
// chains: the user's key, compose-interpolated, NO embedded default. Shared
// with the key guard in internal/stacks, which matches this exact line.
const PublicChainKeyComposeEnv = "PUBLIC_CHAIN_PRIVATE_KEY=${DEMO_PUBLIC_CHAIN_PRIVATE_KEY:-${PUBLIC_CHAIN_PRIVATE_KEY:-}}"

// PublicChainPresets maps CLI-facing preset names to their chain config.
var PublicChainPresets = map[string]PublicChain{
	"rayls-testnet": {
		Name:     "rayls-testnet",
		RPC:      "https://testnet-rpc.rayls.com/",
		ChainID:  7295799,
		Faucet:   FundingURL,
		Explorer: "https://testnet-explorer.rayls.com/",
	},
	// Fully local public chain: an Axyl node inside the stack. The default for
	// --local inits (no external connectivity, no user-funded testnet key
	// needed); the deployer key is genesis-funded on it.
	"local": {
		Name:    "local",
		RPC:     fmt.Sprintf("http://public-chain:%d", LocalPublicChainPort),
		ChainID: localPublicChainChainID,
		Local:   true,
	},
}

// localImage rewrites a Rayls ECR image to its local short-name equivalent
// when local=true — but ONLY for the components the CLI can actually build
// from source (the Components registry: kos/pubrelayer from the relayer repo,
// contracts) plus the axyl node (pre-pulled and retagged locally by
// ensureAxylImage). Everything else — nats, the private-network-hub (stock
// Besu), the private relayer, proofs-api (gnark), the governance services and
// the audit explorer — has no source-build path in this repo set and must
// keep pulling from ECR: short-naming those with pull_policy=never makes `up`
// die with "No such image" (this exact failure shipped for full+--local until
// the blacklist here was inverted to a whitelist).
// Non-Rayls images (postgres, grafana, etc.) are returned unchanged. Returns
// (image, pullPolicy); pullPolicy is empty for registry-backed images.
func localImage(image string, local bool) (string, string) {
	if !local || !strings.HasPrefix(image, raylsECRPrefix) {
		return image, ""
	}
	shortName := strings.TrimPrefix(image, raylsECRPrefix)
	// Every image here must have a matching attachBuild call in --local, or the
	// short-name + pull_policy=never below leaves compose with "No such image".
	// kos/pubrelayer/relayer come from the relayer component; governance trio
	// from the governance component; proofs-api from gnark; audit-explorer from
	// auditor; contracts from contracts; axyl is retagged by ensureAxylImage.
	for _, buildable := range []string{
		"rayls-kos", "rayls-pubrelayer", "rayls-relayer", "rayls-contracts", "rayls-privacy-axyl",
		"rayls-governance-api", "rayls-governance-listener", "rayls-governance-flagger",
		"rayls-proof-api", "rayls-audit-explorer",
	} {
		if strings.HasPrefix(shortName, buildable) {
			return shortName, "never"
		}
	}
	return image, ""
}

// attachBuild wires a from-source build into svc: the build section (git
// context by default, local checkout via <PREFIX>_SRC) plus pull_policy=build
// so a plain `docker compose up` builds it. Only the FIRST participant's
// service per image carries the build — sibling services reuse the tag the
// build produces (compose builds before creating any containers), which
// avoids building the same image once per participant.
func attachBuild(svc *Service, srcs *Sources, componentKey, servicePrefix string) {
	if srcs == nil {
		return
	}
	if b := srcs.BuildSection(componentKey, servicePrefix); b != nil {
		svc.Build = b
		svc.PullPolicy = "build"
	}
}

// axylNodeImage is the single generic Axyl (rayls-network) node image. Unlike
// the legacy geth image, the participant letter is NOT baked into the image
// name — per-participant chain-id is applied at runtime by the init ceremony,
// so one image serves every participant. Built from the axyl repo's
// etc/docker-network/Dockerfile with:
//
//	docker build -f etc/docker-network/Dockerfile \
//	  --build-arg BUILD_FEATURES=dev-single-node-setup \
//	  --build-arg VERGEN_GIT_SHA=$(git rev-parse HEAD) \
//	  -t rayls-privacy-axyl:latest .
//
// The dev feature relaxes Axyl's committee-size assert to allow a single
// validator; VERGEN_GIT_SHA is required or reth's build.rs panics on an empty
// short-SHA slice. Uses a dedicated repo (rayls-privacy-axyl) rather than
// rayls-privacy-node, which collides with a stale unrelated image on ECR.
const axylNodeImage = raylsECRPrefix + "rayls-privacy-axyl:latest"

// axylDevFundedAccount is the deployer address (derived from PRIVATE_KEY_SYSTEM,
// the publicly known Anvil dev account #0) the contracts service transacts
// with; the Axyl genesis pre-funds it 1e9 RLS so the deploy has gas.
// TODO(milestone-3): also fund whatever address the plain-demo contracts
// image bakes when no --public-chain is set.
const axylDevFundedAccount = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

// axylInitScript is the one-shot external genesis ceremony for a single-validator
// dev chain, parameterized via RL_CHAIN_ID / RL_DEV_FUNDED_ACCOUNT / RL_DATADIR.
//
// The `touch .dev-bootstrap-complete` is CRITICAL: a dev-feature build force-
// enables --dev on every `rayls node` start and, absent this sentinel, would
// auto-bootstrap over our external genesis with a chain-id-2017 / Hardhat-funded
// dev chain. Writing the sentinel makes the node load OUR genesis instead.
//
// Every `$` is written as `$$`: docker compose interpolates ${VAR}/$VAR in the
// compose file before starting the container, so `$$` is the escape that passes
// a literal `$` through to the container's bash (which then expands these at
// runtime against the RL_* values set in `environment:`). Do NOT "simplify" the
// `$$` back to `$` — compose would blank the shell vars and keytool would get an
// empty --address.
const axylInitScript = `set -euo pipefail
DD="$${RL_DATADIR:-/home/nonroot/data}"
if [ -f "$$DD/.dev-bootstrap-complete" ]; then echo "[axyl-init] already initialized; skipping"; exit 0; fi
echo "[axyl-init] chain-id=$$RL_CHAIN_ID funded=$$RL_DEV_FUNDED_ACCOUNT datadir=$$DD"
rayls keytool generate validator --datadir "$$DD" --address "$$RL_DEV_FUNDED_ACCOUNT"
mkdir -p "$$DD/genesis/validators"
cp "$$DD/node-info.yaml" "$$DD/genesis/validators/validator.yaml"
rayls genesis --datadir "$$DD" --chain-id "$$RL_CHAIN_ID" --base-fee 0 --min-base-fee 0 --dev-funded-account "$$RL_DEV_FUNDED_ACCOUNT" --max-header-delay-ms 1000 --min-header-delay-ms 500 --epoch-duration-in-secs 60
touch "$$DD/.dev-bootstrap-complete"
chown -R 1101:1101 "$$DD"
echo "[axyl-init] ceremony complete"`

// axylNodeCommand is the long-running Axyl node invocation shared by the
// per-participant privacy nodes and the local public chain.
//
// Enlarged txpool limits: the Rayls contracts deploy fires large batches of
// txs from a single account (Promise.all over dozens of config/grantRole
// calls); reth's small defaults (max-account-slots = 16) reject them
// mid-deploy with "txpool is full". These values match axyl's reference
// etc/docker-network/compose.yaml. Gasless at runtime via
// minimal-protocol-fee/suggested-fee 0 (the genesis ceremony pins base-fee 0).
func axylNodeCommand(datadir string, httpPort int) string {
	return fmt.Sprintf("/usr/local/bin/rayls node --datadir %s "+
		"--http --http.addr 0.0.0.0 --http.port %d --http.api all "+
		"--full --storage.v2 "+
		"--txpool.pending-max-count 50000 --txpool.pending-max-size 62144000 "+
		"--txpool.basefee-max-count 50000 --txpool.basefee-max-size 1048556000 "+
		"--txpool.queued-max-count 50000 --txpool.queued-max-size 1048556000 "+
		"--txpool.max-pending-txns 50000 --txpool.max-new-txns 50000 "+
		"--txpool.max-account-slots 50000 "+
		"--txpool.gas-limit 999999999999 --txpool.max-tx-gas 999999999999 "+
		"--txpool.max-tx-input-bytes 999999999999 "+
		"--txpool.minimal-protocol-fee 0 --gpo.default-suggested-fee 0", datadir, httpPort)
}

// getAxylPrivacyNodeServices generates, per participant, a one-shot init
// container (runs the external genesis ceremony as root, then exits) plus a
// long-running Axyl node container (uid 1101). It preserves the same contract
// every consumer relies on as the legacy geth node: service name
// privacy-node-<p>, aliases pl-<p>/pn-<p>, internal RPC port 8545+i, chain-id
// 12345+i, and a pre-funded deployer. Axyl self-stores in its datadir — no
// external database. Healthcheck uses a bash /dev/tcp probe because the
// Axyl slim image ships no curl.
func getAxylPrivacyNodeServices(participants []string, local bool) map[string]*Service {
	services := make(map[string]*Service)
	image, pullPolicy := localImage(axylNodeImage, local)
	const datadir = "/home/nonroot/data"
	for i, p := range participants {
		httpPort := 8545 + i
		networkId := 12345 + i
		serviceName := "privacy-node-" + p
		initName := serviceName + "-init"
		dataVolume := fmt.Sprintf("privacy-node-%s-data:%s", p, datadir)

		// One-shot genesis ceremony (root; hands datadir to uid 1101).
		services[initName] = &Service{
			Image:      image,
			PullPolicy: pullPolicy,
			User:       "root",
			EntryPoint: []string{"bash", "-c", axylInitScript},
			Environment: []string{
				"RL_BLS_PASSPHRASE=local",
				"RL_DATADIR=" + datadir,
				fmt.Sprintf("RL_CHAIN_ID=%d", networkId),
				"RL_DEV_FUNDED_ACCOUNT=" + axylDevFundedAccount,
				"RL_EXTERNAL_PRIMARY_ADDR=/ip4/127.0.0.1/udp/49590/quic-v1",
				"RL_EXTERNAL_WORKER_ADDRS=/ip4/127.0.0.1/udp/49595/quic-v1",
			},
			Volumes: []string{dataVolume},
		}

		// Long-running node (uid 1101). Gasless: base-fee/min-base-fee 0 at
		// genesis (init) + minimal-protocol-fee/suggested-fee 0 at runtime.
		services[serviceName] = &Service{
			Image:      image,
			PullPolicy: pullPolicy,
			Restart:    "unless-stopped",
			User:       "1101:1101",
			Command:    axylNodeCommand(datadir, httpPort),
			Environment: []string{
				"RUST_LOG=info",
				"RL_BLS_PASSPHRASE=local",
				"RAYLS_NETWORK=local",
				"PRIMARY_LISTENER_MULTIADDR=/ip4/0.0.0.0/udp/49590/quic-v1",
				"WORKER_LISTENER_MULTIADDR=/ip4/0.0.0.0/udp/49595/quic-v1",
			},
			Networks: map[string]*NetworkConfig{
				"default": {Aliases: []string{"pl-" + p, "pn-" + p}},
			},
			Volumes: []string{dataVolume},
			Ports:   []string{fmt.Sprintf("127.0.0.1:%d:%d", httpPort, httpPort)},
			HealthCheck: &HealthCheck{
				Test:        []string{"CMD", "bash", "-c", fmt.Sprintf("echo > /dev/tcp/127.0.0.1/%d", httpPort)},
				Interval:    "10s",
				Timeout:     "10s",
				Retries:     10,
				StartPeriod: "30s",
			},
			DependsOn: map[string]interface{}{
				initName: map[string]string{"condition": "service_completed_successfully"},
			},
		}
	}
	return services
}

// getLocalPublicChainServices generates the `local` public-chain preset's
// containers: the same one-shot genesis ceremony + Axyl node pair the privacy
// nodes use, but serving as the PUBLIC chain — service/alias `public-chain`,
// chain id 7331, RPC port 8845 (the relayer repo's local-PC conventions). The
// genesis pre-funds the same deployer account as the privacy nodes, so the
// contracts deploy (which treats this chain like any external one, via the
// env-driven `public_chain` hardhat network) signs with the genesis-funded
// PRIVATE_KEY_SYSTEM; no external funding or user-supplied key involved.
func getLocalPublicChainServices(local bool) map[string]*Service {
	services := make(map[string]*Service)
	image, pullPolicy := localImage(axylNodeImage, local)
	const datadir = "/home/nonroot/data"
	const serviceName = "public-chain"
	const initName = serviceName + "-init"
	dataVolume := fmt.Sprintf("%s-data:%s", serviceName, datadir)

	services[initName] = &Service{
		Image:      image,
		PullPolicy: pullPolicy,
		User:       "root",
		EntryPoint: []string{"bash", "-c", axylInitScript},
		Environment: []string{
			"RL_BLS_PASSPHRASE=local",
			"RL_DATADIR=" + datadir,
			fmt.Sprintf("RL_CHAIN_ID=%d", localPublicChainChainID),
			"RL_DEV_FUNDED_ACCOUNT=" + axylDevFundedAccount,
			"RL_EXTERNAL_PRIMARY_ADDR=/ip4/127.0.0.1/udp/49590/quic-v1",
			"RL_EXTERNAL_WORKER_ADDRS=/ip4/127.0.0.1/udp/49595/quic-v1",
		},
		Volumes: []string{dataVolume},
	}

	services[serviceName] = &Service{
		Image:      image,
		PullPolicy: pullPolicy,
		Restart:    "unless-stopped",
		User:       "1101:1101",
		Command:    axylNodeCommand(datadir, LocalPublicChainPort),
		Environment: []string{
			"RUST_LOG=info",
			"RL_BLS_PASSPHRASE=local",
			"RAYLS_NETWORK=local",
			"PRIMARY_LISTENER_MULTIADDR=/ip4/0.0.0.0/udp/49590/quic-v1",
			"WORKER_LISTENER_MULTIADDR=/ip4/0.0.0.0/udp/49595/quic-v1",
		},
		Volumes: []string{dataVolume},
		Ports:   []string{fmt.Sprintf("127.0.0.1:%d:%d", LocalPublicChainPort, LocalPublicChainPort)},
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD", "bash", "-c", fmt.Sprintf("echo > /dev/tcp/127.0.0.1/%d", LocalPublicChainPort)},
			Interval:    "10s",
			Timeout:     "10s",
			Retries:     10,
			StartPeriod: "30s",
		},
		DependsOn: map[string]interface{}{
			initName: map[string]string{"condition": "service_completed_successfully"},
		},
	}
	return services
}

func getKosServices(participants []string, monitoring bool, local bool, lean bool, srcs *Sources) map[string]*Service {
	services := make(map[string]*Service)
	relayerPath := relayerPathV3
	otelSdkDisabled := "true"
	otelEndpoint := ""
	if monitoring {
		otelSdkDisabled = "true"
		otelEndpoint = "http://otel:4318"
	}
	for i, p := range participants {
		portsGRPC := 8080 + i // host -> container CTS_GRPC_PORT 8080
		portsHTTP := 8090 + i // host -> container CTS_HTTP_PORT 8090
		portsDebug := 4000 + i

		participantUpper := strings.ToUpper(p)
		serviceName := "kos-" + p
		envFile := fmt.Sprintf("%s/.%s.env", relayerPath, participantUpper)

		// The 3.0.0 CTS (rayls-cts) reads its config from the file given via
		// `run --env <path>` (the old KOS `ENV_FILE` env var is ignored), serves
		// gRPC on 8080 + HTTP on 8090, and runs mTLS unconditionally on both — so
		// it mounts the shared rayls-certs volume and waits on certs-init. Its DB
		// is cts<P> in the shared postgres. The image entrypoint is already
		// `rayls-cts run`, so Command supplies only the flags.
		env := map[string]string{
			"OTEL_SERVICE_NAME": serviceName,
			"OTEL_SDK_DISABLED": otelSdkDisabled,
		}

		if monitoring {
			env["OTEL_EXPORTER_OTLP_ENDPOINT"] = otelEndpoint
		}

		image, pullPolicy := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-kos:latest", local)
		services[serviceName] = &Service{
			Image:      image,
			PullPolicy: pullPolicy,
			Restart:    "unless-stopped",
			Command:    fmt.Sprintf("--env %s", envFile),
			Networks: map[string]*NetworkConfig{
				"default": {
					Aliases: []string{"cts-" + p},
				},
			},
			DependsOn: map[string]interface{}{
				"certs-init": map[string]string{"condition": "service_completed_successfully"},
				"postgres":   map[string]string{"condition": "service_healthy"},
				"contracts":  map[string]string{"condition": "service_healthy"},
			},
			Ports: []string{
				fmt.Sprintf("127.0.0.1:%d:8080", portsGRPC),
				fmt.Sprintf("127.0.0.1:%d:8090", portsHTTP),
				fmt.Sprintf("127.0.0.1:%d:%d", portsDebug, portsDebug),
			},
			Volumes: []string{
				"shared-config:/parfin",
				"rayls-certs:/certs",
			},
			Environment: env,
			// No healthcheck: the CTS prod image is FROM scratch (no shell/curl).
			// The contracts deploy gates readiness (curl cts-<p>:8090/health) and
			// dependents (pubrelayer) connect to the gRPC channel with retry.
		}
	}
	// From-source build (--local): attach to the first participant's service
	// only; siblings reuse the tag the build produces.
	if len(participants) > 0 {
		attachBuild(services["kos-"+participants[0]], srcs, "relayer", "kos")
	}
	return services
}

// relayerServicePrefix names the per-participant private-relayer services
// ("relayer-a", ...) — shared by the service map, the build attach, and the
// hub-less drop list.
const relayerServicePrefix = "relayer-"

func getRelayerServices(participants []string, monitoring bool, local bool, srcs *Sources) map[string]*Service {
	services := make(map[string]*Service)
	otelSdkDisabled := "true"
	otelEndpoint := ""
	if monitoring {
		otelSdkDisabled = "true"
		otelEndpoint = "http://otel:4318"
	}
	for i, p := range participants {
		portsHTTP := 9000 + i
		portsDebug := 4010 + i

		participantUpper := strings.ToUpper(p)
		serviceName := relayerServicePrefix + p
		envFile := fmt.Sprintf("%s/.%s.env", relayerPathV3, participantUpper)

		env := []string{
			fmt.Sprintf("GO_DEBUG_PORT=%d", portsDebug),
			fmt.Sprintf("OTEL_SERVICE_NAME=%s", serviceName),
			fmt.Sprintf("OTEL_SDK_DISABLED=%s", otelSdkDisabled),
			// mTLS cert paths as container env vars (viper AutomaticEnv). Same
			// rationale as the pubrelayer: the private relayer connects to the
			// now-TLS NATS and to the CTS gRPC channel, both mTLS. Point them at
			// the shared dev certs and this relayer's own leaf.
			"NATS_TLS_CA_FILE=/certs/ca.crt",
			"NATS_TLS_CERT_FILE=/certs/private-relayer.crt",
			"NATS_TLS_KEY_FILE=/certs/private-relayer.key",
			"CTS_CLIENT_TLS_CA_FILE=/certs/ca.crt",
			"CTS_CLIENT_TLS_CERT_FILE=/certs/private-relayer.crt",
			"CTS_CLIENT_TLS_KEY_FILE=/certs/private-relayer.key",
		}

		if monitoring {
			env = append(env, fmt.Sprintf("OTEL_EXPORTER_OTLP_ENDPOINT=%s", otelEndpoint))
		}

		image, pullPolicy := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-relayer:latest", local)
		services[serviceName] = &Service{
			Image:      image,
			PullPolicy: pullPolicy,
			Restart:    "unless-stopped",
			// The 3.0.0 private relayer is a prod FROM-scratch image whose
			// entrypoint is `/app/rayls-privacy-relayer-api run`, so it takes just
			// `--env <file>`. It reads the same per-participant V3 env file the CTS
			// uses. No healthcheck: scratch has no shell.
			Command: fmt.Sprintf("--env %s", envFile),
			DependsOn: map[string]interface{}{
				// kos-<p> (CTS) is a scratch image with no healthcheck, so wait for
				// started, not healthy. The relayer retries the CTS gRPC connection
				// until the CTS is ready.
				fmt.Sprintf("kos-%s", p):          map[string]string{"condition": "service_started"},
				fmt.Sprintf("privacy-node-%s", p): map[string]string{"condition": "service_healthy"},
				"nats":                            map[string]string{"condition": "service_healthy"},
				"contracts":                       map[string]string{"condition": "service_healthy"},
				"private-network-hub":             map[string]string{"condition": "service_healthy"},
			},
			Ports: []string{
				fmt.Sprintf("127.0.0.1:%d:%d", portsHTTP, portsHTTP),
				fmt.Sprintf("127.0.0.1:%d:%d", portsDebug, portsDebug),
			},
			Volumes: []string{
				"shared-config:/parfin",
				"rayls-certs:/certs",
			},
			Environment: env,
		}
	}
	// From-source build (--local): attach to the first participant's service
	// only; siblings reuse the tag the build produces (same pattern as kos).
	if len(participants) > 0 {
		attachBuild(services[relayerServicePrefix+participants[0]], srcs, "relayer", "relayer")
	}
	return services
}

// pubRelayerContainerPort is the port the pubrelayer healthcheck server binds
// to inside the container. It is hard-coded in public-relayer/cmd/run/run.go
// (":9000"), so every container listens on 9000 and host ports differ per
// participant via the port mapping.
const pubRelayerContainerPort = 9000

// getPubRelayerServices generates one pubrelayer per participant. The pubrelayer
// reads all chain, NATS, registry and KOS config from /parfin/rayls-privacy-relayer-api/.X.env,
// which is written by the contracts service after deployment against the external
// public chain. We only set ENV_FILE / GO_DEBUG_PORT / OTEL here — everything else
// lives in the env file.
//
// When a public chain preset is active and the live contracts image hasn't
// caught up with the rename (it may still write the local public-chain service
// URL), we wrap the entrypoint with a sed that overrides PUBLIC_CHAIN_RPC_URL
// to the preset's RPC before launching. Env vars alone can't override the
// env file because the relayer-api configinit loader has a viper wiring bug
// (local instance created with AutomaticEnv, but Unmarshal runs on the global
// viper which only sees the config file).
func getPubRelayerServices(participants []string, monitoring bool, local bool, pc *PublicChain, lean bool, srcs *Sources) map[string]*Service {
	relayerPath := relayerPathV3
	services := make(map[string]*Service)
	otelSdkDisabled := "true"
	otelEndpoint := ""
	if monitoring {
		otelEndpoint = "http://otel:4318"
	}
	for i, p := range participants {
		portsHTTP := 9050 + i
		portsDebug := 4050 + i
		participantUpper := strings.ToUpper(p)
		serviceName := "pubrelayer-" + p

		env := []string{
			fmt.Sprintf("ENV_FILE=%s/.%s.env", relayerPath, participantUpper),
			fmt.Sprintf("GO_DEBUG_PORT=%d", portsDebug),
			fmt.Sprintf("OTEL_SERVICE_NAME=%s", serviceName),
			fmt.Sprintf("OTEL_SDK_DISABLED=%s", otelSdkDisabled),
			// mTLS cert paths as container env vars (viper AutomaticEnv). The
			// pubrelayer loads its .A.env via air+ENV_FILE and does NOT reliably
			// pick up the NATS_TLS_*/CTS_CLIENT_TLS_* path overrides from the file,
			// so it falls back to system CAs and NATS mTLS fails with "certificate
			// signed by unknown authority". Set them explicitly here, pointing at
			// the shared dev certs (mounted at /certs) and this relayer's own leaf.
			"NATS_TLS_CA_FILE=/certs/ca.crt",
			"NATS_TLS_CERT_FILE=/certs/public-relayer.crt",
			"NATS_TLS_KEY_FILE=/certs/public-relayer.key",
			"CTS_CLIENT_TLS_CA_FILE=/certs/ca.crt",
			"CTS_CLIENT_TLS_CERT_FILE=/certs/public-relayer.crt",
			"CTS_CLIENT_TLS_KEY_FILE=/certs/public-relayer.key",
		}
		if monitoring {
			env = append(env, fmt.Sprintf("OTEL_EXPORTER_OTLP_ENDPOINT=%s", otelEndpoint))
		}

		image, pullPolicy := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-pubrelayer:latest", local)
		envFile := fmt.Sprintf("%s/.%s.env", relayerPath, participantUpper)
		svc := &Service{
			Image:      image,
			PullPolicy: pullPolicy,
			Restart:    "unless-stopped",
			// The 3.0.0 pubrelayer is a prod FROM-scratch image (entrypoint
			// `public-relayer run`, no air/shell). Load config explicitly with
			// `--env` — like the CTS — because the ENV_FILE+viper path doesn't
			// reliably honor the file's NATS_TLS_*/CTS_CLIENT_TLS_* overrides.
			// No sed for PUBLIC_CHAIN_RPC_URL: the contracts deploy writes it into
			// the env file. No healthcheck: scratch has no shell to run one.
			Command: fmt.Sprintf("--env %s", envFile),
			DependsOn: map[string]interface{}{
				// The private relayer is a scratch image with no healthcheck, so
				// wait for started, not healthy — compose refuses to start a
				// service whose service_healthy dependency has no healthcheck.
				// The pubrelayer retries its connections until peers are ready.
				fmt.Sprintf("relayer-%s", p): map[string]string{"condition": "service_started"},
				"nats":                       map[string]string{"condition": "service_healthy"},
				"contracts":                  map[string]string{"condition": "service_healthy"},
			},
			Ports: []string{
				fmt.Sprintf("127.0.0.1:%d:%d", portsHTTP, pubRelayerContainerPort),
				fmt.Sprintf("127.0.0.1:%d:%d", portsDebug, portsDebug),
			},
			// Mount the shared dev certs at /certs (the paths the .A.env + the
			// NATS_TLS_*/CTS_CLIENT_TLS_* container env vars point at). NOT at
			// /app/public-relayer/certs — in the prod scratch image /app/public-relayer
			// is the binary FILE, so a mount there fails ("not a directory").
			Volumes:     []string{"shared-config:/parfin", "rayls-certs:/certs"},
			Environment: env,
		}
		services[serviceName] = svc
	}
	if len(participants) > 0 {
		attachBuild(services["pubrelayer-"+participants[0]], srcs, "relayer", "pubrelayer")
	}
	return services
}

// getNatsService returns the single shared NATS service used by all participants
// and governance. As of v2.6.3, all services share one NATS instance instead of
// one per participant. Legacy per-participant aliases (nats-1..6, nats-a..f)
// are kept on the network so older config that still references them resolves
// to this shared instance.
// certsInitScript generates a self-signed dev CA and the leaf certs the 3.0.0
// CTS/relayer mTLS mesh needs, into the shared `rayls-certs` volume (mounted at
// /certs). The CTS gRPC channel and the NATS connection both run mTLS
// unconditionally (see cts/grpc/server.go and cts nats.Secure), so NATS, CTS,
// and the pubrelayer all authenticate with certs from this one CA.
//
// Files written to /certs: ca.crt, server.{crt,key} (CTS gRPC server),
// nats-server.{crt,key} (NATS server), cts.{crt,key} (CTS->NATS client),
// public-relayer.{crt,key} (pubrelayer->CTS gRPC + ->NATS client),
// private-relayer.{crt,key} (private relayer->NATS + ->CTS gRPC, full mode),
// governance.{crt,key} (governance api/listener/flagger->NATS, full mode).
//
// Idempotent: skips if /certs/ca.crt already exists. Every `$` is `$$` so
// docker compose passes it through to the container shell (see axylInitScript).
const certsInitScript = `set -eu
CERTS=/certs
if [ -f "$$CERTS/ca.crt" ]; then echo "[certs-init] certs already present; skipping"; exit 0; fi
apk add --no-cache openssl >/dev/null
cd "$$CERTS"
D=825
openssl genrsa -out ca.key 4096 >/dev/null 2>&1
openssl req -x509 -new -nodes -key ca.key -sha256 -days $$D -subj "/CN=Rayls Dev CA/O=Rayls" -out ca.crt >/dev/null 2>&1
gen() {
  n=$$1; cn=$$2; ext=$$3
  openssl genrsa -out $$n.key 4096 >/dev/null 2>&1
  openssl req -new -key $$n.key -subj "/CN=$$cn/O=Rayls" -out $$n.csr >/dev/null 2>&1
  openssl x509 -req -in $$n.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days $$D -sha256 -extfile $$ext -out $$n.crt >/dev/null 2>&1
  rm -f $$n.csr
}
printf '%s\n' 'basicConstraints=CA:FALSE' 'keyUsage=digitalSignature,keyEncipherment' 'extendedKeyUsage=serverAuth' 'subjectAltName=DNS:localhost,DNS:cts,DNS:cts-a,DNS:cts-b,DNS:cts-c,DNS:cts-d,DNS:cts-e,DNS:cts-f,IP:127.0.0.1' > server.ext
gen server cts server.ext
printf '%s\n' 'basicConstraints=CA:FALSE' 'keyUsage=digitalSignature,keyEncipherment' 'extendedKeyUsage=serverAuth' 'subjectAltName=DNS:localhost,DNS:nats,DNS:nats-a,DNS:nats-b,DNS:nats-c,DNS:nats-d,DNS:nats-e,DNS:nats-f,IP:127.0.0.1' > nats-server.ext
gen nats-server nats nats-server.ext
printf '%s\n' 'basicConstraints=CA:FALSE' 'keyUsage=digitalSignature,keyEncipherment' 'extendedKeyUsage=clientAuth' > client.ext
gen cts cts client.ext
gen public-relayer public-relayer client.ext
gen private-relayer private-relayer client.ext
gen governance governance client.ext
chmod 644 *.crt; chmod 600 *.key
# NATS server config: TLS with the generated server cert. verify:false keeps the
# handshake server-authenticated only, so clients must speak TLS (present the CA)
# but are not rejected for lacking a client cert — limits the client cascade.
printf '%s\n' 'max_payload: 8388608' 'tls {' '  cert_file: "/certs/nats-server.crt"' '  key_file: "/certs/nats-server.key"' '  ca_file: "/certs/ca.crt"' '  verify: false' '}' > nats.conf
echo "[certs-init] generated CA + CTS/NATS/relayer/governance certs + nats.conf in $$CERTS"`

// getCertsInitService returns the one-shot init container that populates the
// shared rayls-certs volume with the dev mTLS certs (see certsInitScript).
// CTS, NATS and the pubrelayer depend on it via service_completed_successfully.
func getCertsInitService() *Service {
	return &Service{
		Image:      "alpine:3.20",
		EntryPoint: []string{"sh", "-c", certsInitScript},
		Volumes:    []string{"rayls-certs:/certs"},
	}
}

func getNatsService(local bool) *Service {
	natsPort := 4222
	monitorPort := 8222
	image, pullPolicy := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-nats:latest", local)
	aliases := []string{
		"nats-1", "nats-2", "nats-3", "nats-4", "nats-5", "nats-6",
		"nats-a", "nats-b", "nats-c", "nats-d", "nats-e", "nats-f",
	}
	return &Service{
		Image:      image,
		PullPolicy: pullPolicy,
		Restart:    "unless-stopped",
		// Use the TLS-enabled nats.conf generated by certs-init into the shared
		// rayls-certs volume (the 3.0.0 CTS connects with nats.Secure). The baked
		// /etc/nats/nats.conf is plaintext-only and would reject the mTLS handshake.
		Command: fmt.Sprintf("-js -c /certs/nats.conf -p %d -m %d", natsPort, monitorPort),
		DependsOn: map[string]interface{}{
			"certs-init": map[string]string{"condition": "service_completed_successfully"},
		},
		Volumes: []string{"rayls-certs:/certs"},
		Networks: map[string]*NetworkConfig{
			"default": {Aliases: aliases},
		},
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:%d", natsPort, natsPort),
			fmt.Sprintf("127.0.0.1:%d:%d", monitorPort, monitorPort),
		},
		HealthCheck: &HealthCheck{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("wget -qO- http://localhost:%d/ || exit 1", monitorPort)},
			Interval: "5s",
			Timeout:  "2s",
			Retries:  10,
		},
	}
}

// contractsDeployCommand runs the image's deploy behind an already-deployed
// check. The deploy tasks refuse to redeploy over an existing OpenZeppelin
// manifest ("Cannot deploy: manifest already exists"), and those manifests live
// in .openzeppelin/ inside the shared-config volume, which survives `rayls stop`,
// `rayls down` and a re-run of `rayls init` — so a restarted or recreated
// contracts container would re-run a deploy that can only fail, taking every
// service that gates on `contracts: service_healthy` with it. When the manifests
// are there the contracts are already on-chain, so report ready instead: 1.0 in
// /tmp/deploy_status is what the image's healthcheck server answers 200 for.
// `rayls down -v` drops the volumes for a genuinely fresh deploy.
//
// Paths are relative to the image's working dir, and the script avoids `$` and
// single quotes — it rides inside a single-quoted compose `command:` that
// compose interpolates before the container sees it.
//
// The foundry.toml append switches off forge's lint-on-build (default-on since
// forge 1.x, and the image's foundry.toml has no [lint] section): the image's
// hardhat compile shells out to `forge build`, which otherwise floods every
// deploy step AND every later `docker exec npx hardhat` (rayls verify) with
// ~1500 lint-warning lines per compile, burying the real output.
const contractsDeployCommand = `if [ -f foundry.toml ] && ! grep -q lint_on_build foundry.toml; then printf "\n[lint]\nlint_on_build = false\n" >> foundry.toml; fi; ` +
	`if ls .openzeppelin/*.json >/dev/null 2>&1 && [ -f docker/dev/contracts_deploy_healthcheck.js ]; then ` +
	`echo "[rayls] Contracts are already deployed on this stack (.openzeppelin manifests present)."; ` +
	`echo "[rayls] Skipping the deploy: the deploy tasks refuse to redeploy over an existing manifest."; ` +
	`echo "[rayls] Run rayls down -v and then rayls init to wipe the stack volumes and deploy from scratch."; ` +
	`echo "1.0,Contracts already deployed (redeploy skipped)" > /tmp/deploy_status; ` +
	`exec node docker/dev/contracts_deploy_healthcheck.js; ` +
	`fi; ` +
	`exec docker/dev/deploy_contracts.sh`

func getContractsService(participants []string, local bool, pc *PublicChain, lean bool, noHub bool, srcs *Sources) *Service {
	dependsOn := map[string]interface{}{}
	// Lean keeps a MINIMAL PNH (the 3.0.0 CTS registers its sign keys against the
	// PNH's authoritative ParticipantStorageV1), so the contracts deploy waits on
	// private-network-hub in lean too. hub-less mode removes the PNH outright (the
	// 3.0.1 CTS runs hub-less when the deploy writes no PNH_* vars), so nothing
	// waits on it. proofs-api (gnark) is only used by Enygma confidential
	// transfers, which need the hub — standard PN->public-chain token bridges
	// never call it (verified: it only ever serves its own /healthcheck) — so
	// it's kept only in full hub mode.
	if !noHub {
		dependsOn["private-network-hub"] = map[string]string{"condition": "service_healthy"}
		if !lean {
			dependsOn["proofs-api"] = map[string]string{"condition": "service_healthy"}
		}
	}

	for _, p := range participants {
		serviceName := fmt.Sprintf("privacy-node-%s", p)
		dependsOn[serviceName] = map[string]string{"condition": "service_healthy"}
	}
	// The `local` public-chain preset runs inside the stack; the deploy dials
	// it immediately, so wait for it like the privacy nodes.
	if pc != nil && pc.Local {
		dependsOn["public-chain"] = map[string]string{"condition": "service_healthy"}
	}

	participantList := strings.ToUpper(strings.Join(participants, ","))

	// Both lean and full mode run the v3.0.0 contracts image (`:lean-no-pnh` and
	// `:latest`, same digest), which bakes the relayer/governance base config
	// only at the new-glossary V3 paths and defaults RELAYER_PATH/
	// GOVERNANCE_PATH to them. So point the deploy at the V3 paths in both
	// modes — the CTS, relayers and governance services all read from these
	// same paths. In lean the governance services aren't generated, so tell
	// the deploy not to deploy/configure governance either. Same hub-less:
	// governance is PNH governance — deploy_contracts.sh extracts its entire
	// .env from the PNH deploy output, inside the HUB_ENABLED gate — so a
	// hub-less stack can't configure it.
	governanceEnabled := "${GOVERNANCE_ENABLED:-true}"
	if lean || noHub {
		governanceEnabled = "false"
	}
	env := []string{
		fmt.Sprintf("RELAYER_PATH=%s", relayerPathV3),
		fmt.Sprintf("GOVERNANCE_PATH=%s", governancePathV3),
		fmt.Sprintf("PARTICIPANT_LIST=${PARTICIPANT_LIST:-%s}", participantList),
		"CUSTOM_UID=${CUSTOM_UID:-1000}",
		"CUSTOM_GID=${CUSTOM_GID:-1000}",
		"DEV_MODE=${DEV_MODE:-local}",
		fmt.Sprintf("GOVERNANCE_ENABLED=%s", governanceEnabled),
		// The 3.0.1 deploy defaults OPS_API_ENABLED=true and hard-fails (exit 10)
		// generating ops-api bindings into /parfin/rayls-privacy-ops-api, which
		// exists only in the relayer repo's bind-mounted dev stack — the CLI never
		// runs ops-api, so switch the step off in every mode (inert on the
		// 3.0.0-era images, which don't read it).
		"OPS_API_ENABLED=false",
	}
	// HUB_ENABLED is the 3.0.1 deploy's hub switch (deploy_contracts.sh gates the
	// whole PNH path on it: PNH deploy, business roles, PNH-side relayer auth,
	// template seeding, and — crucially — every PNH_* write into the per-
	// participant .X.env files, whose ABSENCE is what flips the 3.0.1 CTS into
	// hub-less mode). PNH_ENABLED is the equivalent switch of the 3.0.0-era
	// lean-no-pnh deploy image; 3.0.1 ignores it. Pass both explicitly so either
	// image generation reads its own knob.
	if noHub {
		// No PNH_ENABLED here on purpose: a 3.0.0-era deploy image doesn't know
		// HUB_ENABLED, and its PNH_ENABLED=false path aliases the PNH registry to
		// the privacy node in the .X.env — which a 3.0.1 CTS would read as "hub
		// enabled, at the PN" (silently wrong topology). Left defaulted (true),
		// such an image tries to deploy the PNH, dials the absent private-hub
		// host and fails loudly instead. (The aliasing is also insufficient for
		// a genuinely hub-less PULLED stack: the 3.0.0 CTS's ParticipantRegistrar
		// calls ParticipantStorageV1.getChainViewData on its "hub" at startup,
		// which the PN-side ParticipantStorageReplicaV1 doesn't implement —
		// verified empirically 2026-08-20 — so hub-less stays --local-only until
		// the CTS images are republished from >= version/3.0.1.)
		env = append(env, "HUB_ENABLED=false")
		// deployCoreContractsBatch ABI-encodes process.env.PNH_CHAIN_ID into
		// EndpointV1.initialize even hub-less (unset crashes ethers with "invalid
		// BigNumberish value null"); 0 is the no-hub sentinel the RayUp hub-less
		// deploy (deploy-rayup.sh) uses. No PNH_RPC_URL: every consumer of it in
		// the deploy sits inside the HUB_ENABLED gate, and there is no
		// private-hub host to point at.
		env = append(env, "PNH_CHAIN_ID=0")
	} else {
		// Both lean and full deploy the Private Network Hub: the 3.0.0 CTS
		// registers its sign keys against the PNH's authoritative
		// ParticipantStorageV1. In lean it's a MINIMAL PNH (governance skipped
		// via GOVERNANCE_ENABLED=false); in full the PNH plus governance are
		// deployed.
		env = append(env, "PNH_ENABLED=true", "HUB_ENABLED=true")
		// deploy:private-hub reads PNH_CHAIN_ID with no fallback (the Endpoint
		// initializer ABI-encodes it — unset crashes ethers with "invalid
		// BigNumberish value null"); Besu runs BESU_NETWORK_ID=1337 and hardhat's
		// localPNH network pins chainId 1337. activate-business-roles-pnh needs
		// PNH_RPC_URL (the deploy script doesn't pass --rpc-url).
		env = append(env,
			"PNH_CHAIN_ID=1337",
			"PNH_RPC_URL=http://private-hub:3445",
		)
	}
	// In every mode the per-PN deploy task needs PRIVACY_NODE_<P>_RPC_URL, and
	// add-authorized-relayers fetches CTS signing addresses from
	// CTS_SERVICE_<P>_URL once the CTS is healthy (the deploy's background auth
	// monitor waits on the same URL).
	for i, p := range participants {
		env = append(env,
			fmt.Sprintf("PRIVACY_NODE_%s_RPC_URL=http://pl-%s:%d", strings.ToUpper(p), p, 8545+i),
			fmt.Sprintf("CTS_SERVICE_%s_URL=http://cts-%s:8090", strings.ToUpper(p), p),
		)
	}
	// PRIVATE_KEY_SYSTEM drives the PNH/PN deployer (local chains we control)
	// and is needed in EVERY topology, public chain or not: hardhat.config.ts
	// builds the custom_pnh/custom_pn networks with
	// `accounts: [process.env['PRIVATE_KEY_SYSTEM']]` whenever
	// PNH_RPC_URL / PRIVACY_NODE_RPC_URL are set, and hardhat validates the
	// config on every invocation — a missing key fails with HH8 "Invalid
	// account: #0 ... Expected string, received undefined" before any task
	// runs. The default is the publicly known Anvil dev account #0 (never a
	// secret, only ever funded on in-stack genesis chains); override via the
	// shell env var to use your own key.
	env = append(env,
		"PRIVATE_KEY_SYSTEM=${PRIVATE_KEY_SYSTEM:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}",
	)
	if pc != nil {
		// PUBLIC_CHAIN_PRIVATE_KEY is a separately-funded key for the
		// external public chain: the USER's own, with deliberately NO
		// default; `rayls init` guarantees one exists (prompt + persist to
		// the stack .env, see stacks.ensurePublicChainKey).
		//
		// PUBLIC_RELAYER_FUND_AMOUNT_ETH is the amount the contracts deploy
		// seeds each per-participant public-relayer wallet with on the
		// public chain (enough for it to cover its own gas budget).
		env = append(env,
			"PUBLIC_CHAIN_ENABLED=true",
			fmt.Sprintf("PUBLIC_CHAIN_RPC_URL=%s", pc.RPC),
			fmt.Sprintf("PUBLIC_CHAIN_ID=%d", pc.ChainID),
		)
		if !pc.Local {
			// DEMO_PUBLIC_CHAIN_PRIVATE_KEY is a deprecated per-developer
			// alias that outranks the canonical var. The `local` preset
			// omits the entry entirely: its genesis funds PRIVATE_KEY_SYSTEM
			// and the deploy falls back to it; a testnet key would be
			// unfunded there.
			env = append(env, PublicChainKeyComposeEnv)
		}
		env = append(env,
			"PUBLIC_RELAYER_FUND_AMOUNT_ETH=0.5",
		)
	} else {
		env = append(env, "PUBLIC_CHAIN_ENABLED=${PUBLIC_CHAIN_ENABLED:-false}")
	}

	// Pulled modes use :latest — the 3.0.1 image with the HUB_ENABLED-aware
	// deploy; one image serves every topology (lean hub-less, lean with-hub,
	// full), HUB_ENABLED switching the deploy path. The :lean-no-pnh tag now
	// points at the same digest and is kept only for backward compatibility.
	// --local stacks build the same deploy from the pinned
	// rayls-sovereign-contracts main sources instead (see the stack .env pins).
	contractsRef := "public.ecr.aws/w0k9o1t3/rayls-demo/rayls-contracts:latest"
	image, pullPolicy := localImage(contractsRef, local)
	svc := &Service{
		Image:      image,
		PullPolicy: pullPolicy,
		Command:    "/bin/bash -c '" + contractsDeployCommand + "'",
		Ports: []string{
			"127.0.0.1:7000:7000",
		},
		Volumes: []string{
			"shared-config:/parfin",
		},
		Environment: env,
		DependsOn:   dependsOn,
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD-SHELL", "curl --fail http://127.0.0.1:7000"},
			Interval:    "10s",
			Timeout:     "30s",
			Retries:     100,
			StartPeriod: "60s",
		},
	}
	// From-source build. attachBuild no-ops when srcs is nil (every non---local
	// mode), so this covers exactly the --local stacks — lean (with or without
	// hub) and full alike, all building the HUB_ENABLED-aware deploy from the
	// pinned rayls-sovereign-contracts main sources. Previously full+--local skipped the
	// attach and silently depended on a rayls-contracts:latest image left
	// behind by an earlier lean/hub-less build.
	attachBuild(svc, srcs, "contracts", "contracts")
	return svc
}

// buildRelayerDBNames returns the comma-separated list of databases the
// postgres init script should create — four per participant:
// `relayer<P>`, `relayer<P>Kms`, `publicRelayer<P>`, `cts<P>`. Consumed via
// DB_NAMES env. `cts<P>` backs the 3.0.0 CTS (CTS_DATABASE_CONNECTIONSTRING).
func buildRelayerDBNames(participants []string) string {
	names := make([]string, 0, len(participants)*4)
	for _, p := range participants {
		pUpper := strings.ToUpper(p)
		names = append(names,
			fmt.Sprintf("relayer%s", pUpper),
			fmt.Sprintf("relayer%sKms", pUpper),
			fmt.Sprintf("publicRelayer%s", pUpper),
			fmt.Sprintf("cts%s", pUpper),
		)
	}
	return strings.Join(names, ",")
}

// GetPrivacyNodeOnlyConfig generates a minimal Docker Compose with a single
// Axyl privacy node (init ceremony + node). No contracts, relayer, KOS,
// governance, NATS, postgres, commit-chain, or proofs-api — Axyl self-stores
// in its datadir. Useful when external tooling only
// needs an EVM RPC endpoint to point at, and as the smallest smoke target for
// the Axyl node itself.
func GetPrivacyNodeOnlyConfig(local bool) *DockerCompose {
	compose := &DockerCompose{
		Volumes: map[string]interface{}{
			"privacy-node-a-data": map[string]interface{}{},
		},
		Services: map[string]*Service{},
	}
	for name, svc := range getAxylPrivacyNodeServices([]string{"a"}, local) {
		compose.Services[name] = svc
	}
	return compose
}

// GetDemoComposeConfig generates a complete Docker Compose configuration for a Rayls demo stack.
// It creates infrastructure services (commit-chain, governance, a shared NATS, postgres),
// contract deployment services, and per-participant services (privacy ledgers, KOS, relayers)
// for each participant in the provided list. The monitoring parameter controls whether to
// include observability services (Grafana LGTM stack). noHub removes the Private Network Hub
// and everything PNH-scoped from either mode (see applyNoHub). Returns a DockerCompose struct
// ready for YAML marshaling.
func GetDemoComposeConfig(participants []string, monitoring bool, blockscout []string, local bool, publicChain *PublicChain, lean bool, noHub bool, srcs *Sources) *DockerCompose {
	privateNetworkHubImage, privateNetworkHubPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-private-network-hub:latest", local)
	auditExplorerImage, auditExplorerPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-audit-explorer:latest", local)
	governanceAPIImage, governanceAPIPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-governance-api:latest", local)
	governanceListenerImage, governanceListenerPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-governance-listener:latest", local)
	governanceFlaggerImage, governanceFlaggerPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-governance-flagger:latest", local)
	proofsAPIImage, proofsAPIPull := localImage("public.ecr.aws/w0k9o1t3/rayls-demo/rayls-proof-api:latest", local)

	compose := &DockerCompose{
		Volumes: map[string]interface{}{
			"shared-config":     map[string]interface{}{},
			"postgres-data":     map[string]interface{}{},
			"commit-chain-data": map[string]interface{}{},
			"rayls-certs":       map[string]interface{}{},
		},
		Services: map[string]*Service{
			"private-network-hub": {
				Image:      privateNetworkHubImage,
				PullPolicy: privateNetworkHubPull,
				Restart:    "unless-stopped",
				// Besu runs as uid 1000 in this image and /opt/besu/database does
				// not exist in it, so the commit-chain-data named volume mounts
				// root-owned and besu can't write it ("DATABASE_METADATA.json
				// Permission denied"). Running the container as root lets besu
				// initialize the volume. Fine for a local demo stack.
				User: "0:0",
				Environment: []string{
					"BESU_NODE_PRIVATE_KEY_FILE=/app/besu_node_key",
					"BESU_RPC_HTTP_ENABLED=true",
					"BESU_RPC_HTTP_PORT=3445",
					"BESU_RPC_HTTP_HOST=0.0.0.0",
					"BESU_RPC_HTTP_API=ETH,NET,WEB3,CLIQUE,ADMIN,DEBUG,TXPOOL,TRACE",
					"BESU_HOST_ALLOWLIST=*",
					"BESU_P2P_ENABLED=false",
					"BESU_DISCOVERY_ENABLED=false",
					"BESU_MIN_GAS_PRICE=0",
					"BESU_DATA_STORAGE_FORMAT=FOREST",
					"BESU_REVERT_REASON_ENABLED=true",
					"BESU_TX_POOL=sequenced",
					"BESU_DATA_PATH=/opt/besu/database",
					"BESU_NETWORK_ID=1337",
					"BESU_GENESIS_FILE=/app/var/genesis.json",
					"BESU_TX_POOL_RETENTION_HOURS=999",
					"BESU_TX_POOL_LIMIT_BY_ACCOUNT_PERCENTAGE=1.0",
					"BESU_TX_POOL_MAX_SIZE=20000",
					"BESU_RPC_HTTP_MAX_REQUEST_CONTENT_LENGTH=20971520",
					"BESU_RPC_HTTP_MAX_BATCH_SIZE=-1",
				},
				Networks: map[string]*NetworkConfig{
					"default": {
						Aliases: []string{"commit-chain", "private-hub"},
					},
				},
				Volumes: []string{
					// Persist the Besu commit-chain DB (BESU_DATA_PATH) so the
					// commit chain keeps its state across `rayls down`.
					"commit-chain-data:/opt/besu/database",
				},
				Ports: []string{
					"127.0.0.1:3445:3445",
				},
				HealthCheck: &HealthCheck{
					Test:     []string{"CMD-SHELL", "curl -X POST -H 'Content-Type: application/json' --data '{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBlockByNumber\",\"params\":[\"latest\", true],\"id\":1}' 127.0.0.1:3445"},
					Interval: "10s",
					Timeout:  "10s",
					Retries:  5,
				},
			},
			// The stack's shared PostgreSQL: hosts the per-participant CTS and
			// relayer databases (see buildRelayerDBNames) and, in full mode,
			// the governance DB. (Renamed from the historical governance-postgres.)
			"postgres": {
				Image:   "postgres:17.3-bookworm",
				Restart: "unless-stopped",
				ShmSize: "128mb",
				Ports: []string{
					"127.0.0.1:5432:5432",
				},
				Environment: map[string]string{
					"POSTGRES_USER":     "governance",
					"POSTGRES_PASSWORD": "gov-super-pass841",
					"POSTGRES_DB":       "governance",
					"PGPORT":            "5432",
					// DB_NAMES is consumed by the init script bind-mounted below;
					// the script creates each database (owned by the admin user
					// it also creates) on first boot.
					"DB_NAMES": buildRelayerDBNames(participants),
				},
				Volumes: []string{
					"./docker-init/postgres-init.sh:/docker-entrypoint-initdb.d/postgres-init.sh:ro",
					"postgres-data:/var/lib/postgresql/data",
				},
				HealthCheck: &HealthCheck{
					Test:        []string{"CMD-SHELL", "pg_isready", "-d", "governance"},
					Interval:    "10s",
					Timeout:     "60s",
					Retries:     20,
					StartPeriod: "20s",
				},
			},
			"audit-explorer": {
				Image:      auditExplorerImage,
				PullPolicy: auditExplorerPull,
				Restart:    "unless-stopped",
				Ports: []string{
					"127.0.0.1:8181:80",
				},
				DependsOn: map[string]interface{}{
					// governance-api is a prod FROM-scratch image with no shell to
					// run a healthcheck, so wait for started (not healthy).
					"governance-api": map[string]string{"condition": "service_started"},
				},
				Environment: map[string]string{
					// The UI fetches assets/config.json at runtime and uses
					// RAYLS_API as its API base. The image's nginx proxies
					// /api/ -> governance-api:8080, so a relative /api avoids
					// CORS and host-port coupling.
					"RAYLS_API": "/api",
				},
			},
			"governance-api": {
				Image:      governanceAPIImage,
				PullPolicy: governanceAPIPull,
				Restart:    "unless-stopped",
				// 3.0.0 prod image (FROM scratch); default CMD is `./api run`. The
				// config path must come via the --config FLAG (viper BindPFlag) —
				// the old `air` wrapper passed it that way; the CONFIG_FILE env var
				// does NOT map to viper's `config` key, so without the flag the DB
				// string is empty and it dials localhost:5432. No healthcheck (no shell).
				Command: "./api run --config " + governancePathV3 + "/.env",
				Volumes: []string{
					"shared-config:/parfin",
					"rayls-certs:/certs",
				},
				Ports: []string{
					"127.0.0.1:9100:8080",
					"127.0.0.1:4030:4030",
				},
				Environment: map[string]string{
					"GO_DEBUG_PORT":     "4030",
					"OTEL_SERVICE_NAME": "governance-api",
					// NATS is mTLS now; governance requires all three when NATS_URL
					// is set (config.go). Point at the shared dev certs + the
					// governance leaf.
					"NATS_TLS_CA_FILE":   "/certs/ca.crt",
					"NATS_TLS_CERT_FILE": "/certs/governance.crt",
					"NATS_TLS_KEY_FILE":  "/certs/governance.key",
				},
				DependsOn: map[string]interface{}{
					"certs-init": map[string]string{"condition": "service_completed_successfully"},
					"contracts":  map[string]string{"condition": "service_healthy"},
					"postgres":   map[string]string{"condition": "service_healthy"},
					"nats":       map[string]string{"condition": "service_healthy"},
				},
			},
			"governance-listener": {
				Image:      governanceListenerImage,
				PullPolicy: governanceListenerPull,
				Restart:    "unless-stopped",
				// 3.0.0 prod image; default CMD is `./listener run`. Config path via
				// the --config flag (see governance-api note). No `air`, no
				// env-sourcing entrypoint.
				Command: "./listener run --config " + governancePathV3 + "/.env",
				Volumes: []string{
					"shared-config:/parfin",
					"rayls-certs:/certs",
				},
				Ports: []string{
					"127.0.0.1:9101:8081",
					"127.0.0.1:4031:4031",
				},
				Environment: map[string]string{
					"GO_DEBUG_PORT":      "4031",
					"OTEL_SERVICE_NAME":  "governance-listener",
					"NATS_TLS_CA_FILE":   "/certs/ca.crt",
					"NATS_TLS_CERT_FILE": "/certs/governance.crt",
					"NATS_TLS_KEY_FILE":  "/certs/governance.key",
				},
				DependsOn: map[string]interface{}{
					"certs-init": map[string]string{"condition": "service_completed_successfully"},
					"postgres":   map[string]string{"condition": "service_healthy"},
					"contracts":  map[string]string{"condition": "service_healthy"},
					"nats":       map[string]string{"condition": "service_healthy"},
				},
			},
			"governance-flagger": {
				Image:      governanceFlaggerImage,
				PullPolicy: governanceFlaggerPull,
				Restart:    "unless-stopped",
				// 3.0.0 prod image; default CMD is `./flagger run`. Config path via
				// the --config flag (see governance-api note).
				Command: "./flagger run --config " + governancePathV3 + "/.env",
				Volumes: []string{
					"shared-config:/parfin",
					"rayls-certs:/certs",
				},
				Ports: []string{
					"127.0.0.1:9102:8082",
					"127.0.0.1:4032:4032",
				},
				Environment: map[string]string{
					"GO_DEBUG_PORT":      "4032",
					"OTEL_SERVICE_NAME":  "governance-flagger",
					"NATS_TLS_CA_FILE":   "/certs/ca.crt",
					"NATS_TLS_CERT_FILE": "/certs/governance.crt",
					"NATS_TLS_KEY_FILE":  "/certs/governance.key",
				},
				DependsOn: map[string]interface{}{
					"certs-init": map[string]string{"condition": "service_completed_successfully"},
					"postgres":   map[string]string{"condition": "service_healthy"},
					"contracts":  map[string]string{"condition": "service_healthy"},
				},
			},
			"proofs-api": {
				Image:      proofsAPIImage,
				PullPolicy: proofsAPIPull,
				Restart:    "unless-stopped",
				Ports: []string{
					"127.0.0.1:3003:3003",
				},
				HealthCheck: &HealthCheck{
					Test:        []string{"CMD-SHELL", "curl --fail http://127.0.0.1:3003/healthcheck"},
					Interval:    "10s",
					Timeout:     "30s",
					Retries:     20,
					StartPeriod: "30s",
				},
			},
		},
	}

	// From-source builds (--local) for the components defined inline above.
	// attachBuild no-ops when srcs is nil (pulled mode); it flips the image to
	// the short-name local build (see localImage's whitelist) + pull_policy=build.
	// These services may be trimmed later by applyLeanNoPNH/applyNoHub, and the
	// build section goes with them: proofs-api survives lean-with-hub; the
	// governance trio and audit-explorer are --full only.
	attachBuild(compose.Services["governance-api"], srcs, "governance", "governance-api")
	attachBuild(compose.Services["governance-listener"], srcs, "governance", "governance-listener")
	attachBuild(compose.Services["governance-flagger"], srcs, "governance", "governance-flagger")
	attachBuild(compose.Services["proofs-api"], srcs, "gnark", "proofs-api")
	attachBuild(compose.Services["audit-explorer"], srcs, "auditor", "audit-explorer")

	if monitoring {
		compose.Services["otel"] = &Service{
			Image: "docker.io/grafana/otel-lgtm:latest",
			Ports: []string{
				"127.0.0.1:3300:3000", // grafana ui
				"127.0.0.1:3100:3100", // loki (logging)
				"127.0.0.1:3090:9090", // prometheus ui (metrics)
				"127.0.0.1:3040:4040", // pyroscope ui (profilling)
				"127.0.0.1:3200:3200", // tempo (tracing)
				"127.0.0.1:4317:4317", // gRPC (OTeL)
				"127.0.0.1:4318:4318", // http (OTeL)
			},
			Environment: map[string]string{
				"GF_PATHS_DATA": "/data/grafana",
			},
		}
	}

	// Add dynamic services (Axyl privacy node: one-shot init ceremony + node,
	// per participant). Each participant gets an isolated single-validator chain
	// with a distinct chain-id 12345+i.
	privacyNodeServices := getAxylPrivacyNodeServices(participants, local)
	for name, service := range privacyNodeServices {
		compose.Services[name] = service
	}
	// Register a named volume for each privacy node's Axyl datadir (mounted in
	// getAxylPrivacyNodeServices) so chain data persists across `rayls down`.
	for _, p := range participants {
		compose.Volumes[fmt.Sprintf("privacy-node-%s-data", p)] = map[string]interface{}{}
	}

	kosServices := getKosServices(participants, monitoring, local, lean, srcs)
	for name, service := range kosServices {
		compose.Services[name] = service
	}

	relayerServices := getRelayerServices(participants, monitoring, local, srcs)
	for name, service := range relayerServices {
		compose.Services[name] = service
	}

	compose.Services["certs-init"] = getCertsInitService()

	compose.Services["nats"] = getNatsService(local)

	compose.Services["contracts"] = getContractsService(participants, local, publicChain, lean, noHub, srcs)

	if publicChain != nil {
		if publicChain.Local {
			// The `local` preset runs the public chain inside the stack: an
			// Axyl node + its one-shot genesis ceremony, plus a named volume
			// for its chain data.
			for name, svc := range getLocalPublicChainServices(local) {
				compose.Services[name] = svc
			}
			compose.Volumes["public-chain-data"] = map[string]interface{}{}
		}
		for name, svc := range getPubRelayerServices(participants, monitoring, local, publicChain, lean, srcs) {
			compose.Services[name] = svc
		}
	}

	// Add Blockscout services if specified
	if len(blockscout) > 0 {
		blockscoutServices := getBlockscoutServices(participants, blockscout)
		for name, service := range blockscoutServices {
			compose.Services[name] = service
		}

		// Add Blockscout volumes
		for idx, node := range blockscout {
			_ = idx // unused but shows intent
			compose.Volumes[fmt.Sprintf("blockscout-db-data-%s", node)] = map[string]interface{}{}
			compose.Volumes[fmt.Sprintf("blockscout-logs-%s", node)] = map[string]interface{}{}
			compose.Volumes[fmt.Sprintf("blockscout-dets-%s", node)] = map[string]interface{}{}
			compose.Volumes[fmt.Sprintf("blockscout-nginx-config-%s", node)] = map[string]interface{}{}
			// compose.Volumes[fmt.Sprintf("blockscout-stats-db-data-%s", node)] = map[string]interface{}{}
		}
	}

	if lean {
		applyLeanNoPNH(compose, participants)
	}
	if noHub {
		applyNoHub(compose, participants)
	}

	return compose
}

// applyLeanNoPNH trims a public-chain compose down to the with-hub lean
// bridge: a FUNCTIONAL single-participant hub. The Private Network Hub,
// relayer-<p> (PN<->PNH message relaying) and proofs-api (Enygma) all stay —
// only governance and the audit explorer are --full territory. KOS is KEPT
// too: the pubrelayer fetches its ECDSA signing keys from KOS (reachable via
// the cts-<p> alias) at startup. The depends_on graphs of the surviving
// services are rewritten so they no longer wait on removed ones.
//
// The topology is uniform across image provenance: the published 3.0.1 ECR
// images and the --local source builds both carry the components this shape
// needs (the private relayer's config contract, the hub-less-capable CTS).
func applyLeanNoPNH(compose *DockerCompose, participants []string) {
	drop := []string{
		"governance-api", "governance-listener", "governance-flagger",
		"audit-explorer",
	}
	for _, name := range drop {
		delete(compose.Services, name)
	}

	// Strip depends_on entries that point at services we just removed, so
	// `up -d` doesn't block forever on a service that will never start.
	removed := make(map[string]bool, len(drop))
	for _, name := range drop {
		removed[name] = true
	}
	for _, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if removed[dep] {
				delete(svc.DependsOn, dep)
			}
		}
	}
}

// applyNoHub removes the Private Network Hub and everything PNH-scoped from
// the compose, in either mode (lean or full), mirroring `start_dev.sh --no-hub`
// in the relayer-api repo: the privacy nodes then intercommunicate only through
// the public chain. Dropped: private-network-hub (Besu), proofs-api/gnark (its
// only runtime consumer is the private relayer), the private relayers
// (relayer-<p> — their config hard-requires PNH_* vars, which the hub-less
// deploy never writes), and the governance services + audit explorer (PNH
// governance: the deploy extracts their config from the PNH deploy output).
//
// The depends_on graphs of the survivors are rewritten accordingly, and the
// pubrelayer waits on kos-<p> instead of the removed private relayer. postgres
// (the shared DB) stays: it hosts the CTS/relayer databases.
//
// The runtime hub-less switch is downstream of the contracts deploy: with
// HUB_ENABLED=false (set in getContractsService) the deploy writes no PNH_*
// vars into the per-participant .X.env files, and the 3.0.1 CTS keys hub-less
// mode off the absence of PNH_DEPLOYMENT_PROXY_REGISTRY. Requires the 3.0.1
// contracts + relayer components (rayls-sovereign-* on main; see
// GenerateDockerCompose's source pinning).
func applyNoHub(compose *DockerCompose, participants []string) {
	drop := []string{
		"private-network-hub",
		"proofs-api",
		"governance-api", "governance-listener", "governance-flagger",
		"audit-explorer",
	}
	for _, p := range participants {
		drop = append(drop, relayerServicePrefix+p)
	}
	for _, name := range drop {
		delete(compose.Services, name)
	}
	// The Besu commit-chain datadir volume belongs to the removed hub.
	delete(compose.Volumes, "commit-chain-data")

	// Strip depends_on entries that point at services we just removed, so
	// `up -d` doesn't block forever on a service that will never start.
	removed := make(map[string]bool, len(drop))
	for _, name := range drop {
		removed[name] = true
	}
	for _, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if removed[dep] {
				delete(svc.DependsOn, dep)
			}
		}
	}

	// The pubrelayer pulls signing keys from KOS at startup; wait for kos-<p>
	// to have *started* in place of the removed relayer-<p>. service_started,
	// not service_healthy — the CTS scratch image has no healthcheck (see
	// applyLeanNoPNH). Idempotent when applyLeanNoPNH already set it.
	for _, p := range participants {
		if pr, ok := compose.Services["pubrelayer-"+p]; ok {
			if pr.DependsOn == nil {
				pr.DependsOn = map[string]interface{}{}
			}
			pr.DependsOn["kos-"+p] = map[string]string{"condition": "service_started"}
		}
	}
}

// getBlockscoutServices generates all Blockscout services for specified nodes
func getBlockscoutServices(participants []string, blockscoutNodes []string) map[string]*Service {
	services := make(map[string]*Service)

	// Generate services for each blockscout-enabled node
	for idx, node := range blockscoutNodes {
		// Find participant index for chain ID
		participantIdx := -1
		for i, p := range participants {
			if p == node {
				participantIdx = i
				break
			}
		}
		if participantIdx == -1 {
			continue
		}

		// Port base for this node (10000, 10100, 10200, etc.)
		portBase := 10000 + (idx * 100)
		chainID := 12345 + participantIdx

		// Add all services for this node
		dbServices := getBlockscoutDBServices(node, portBase)
		for name, service := range dbServices {
			services[name] = service
		}

		services[fmt.Sprintf("blockscout-backend-%s", node)] = getBlockscoutBackendService(node, portBase, chainID, participantIdx)
		services[fmt.Sprintf("blockscout-frontend-%s", node)] = getBlockscoutFrontendService(node, portBase, chainID, participantIdx)
		// services[fmt.Sprintf("blockscout-stats-%s", node)] = getBlockscoutStatsService(node, portBase)
		//services[fmt.Sprintf("blockscout-visualizer-%s", node)] = getBlockscoutVisualizerService(node, portBase)
		services[fmt.Sprintf("blockscout-nginx-config-init-%s", node)] = getBlockscoutNginxConfigInitService(node)
		services[fmt.Sprintf("blockscout-proxy-%s", node)] = getBlockscoutProxyService(node, portBase)
	}

	return services
}

// getBlockscoutDBServices generates PostgreSQL DB services (main + stats) with init containers
func getBlockscoutDBServices(node string, portBase int) map[string]*Service {
	services := make(map[string]*Service)

	// Main DB init container
	services[fmt.Sprintf("blockscout-db-init-%s", node)] = &Service{
		Image: "postgres:17",
		Volumes: []string{
			fmt.Sprintf("blockscout-db-data-%s:/var/lib/postgresql/data", node),
		},
		EntryPoint: []string{"sh", "-c", "chown -R 2000:2000 /var/lib/postgresql/data"},
	}

	// Main DB
	services[fmt.Sprintf("blockscout-db-%s", node)] = &Service{
		Image:   "postgres:17",
		User:    "2000:2000",
		ShmSize: "256mb",
		Restart: "always",
		Command: "postgres -c 'max_connections=200' -c 'client_connection_check_interval=60000'",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:5432", portBase),
		},
		Volumes: []string{
			fmt.Sprintf("blockscout-db-data-%s:/var/lib/postgresql/data", node),
		},
		Environment: map[string]string{
			"POSTGRES_DB":       "blockscout",
			"POSTGRES_USER":     "blockscout",
			"POSTGRES_PASSWORD": "ceWb1MeLBEeOIfk65gU8EjF8",
		},
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD-SHELL", "pg_isready -U blockscout -d blockscout"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     5,
			StartPeriod: "10s",
		},
		DependsOn: map[string]interface{}{
			fmt.Sprintf("blockscout-db-init-%s", node): map[string]string{
				"condition": "service_completed_successfully",
			},
		},
	}

	// Stats DB init container
	// services[fmt.Sprintf("blockscout-stats-db-init-%s", node)] = &Service{
	// 	Image: "postgres:17",
	// 	Volumes: []string{
	// 		fmt.Sprintf("blockscout-stats-db-data-%s:/var/lib/postgresql/data", node),
	// 	},
	// 	EntryPoint: []string{"sh", "-c", "chown -R 2000:2000 /var/lib/postgresql/data"},
	// }

	// Stats DB
	// services[fmt.Sprintf("blockscout-stats-db-%s", node)] = &Service{
	// 	Image:   "postgres:17",
	// 	User:    "2000:2000",
	// 	ShmSize: "256mb",
	// 	Restart: "always",
	// 	Command: "postgres -c 'max_connections=200'",
	// 	Ports: []string{
	// 		fmt.Sprintf("127.0.0.1:%d:5432", portBase+1),
	// 	},
	// 	Volumes: []string{
	// 		fmt.Sprintf("blockscout-stats-db-data-%s:/var/lib/postgresql/data", node),
	// 	},
	// 	Environment: map[string]string{
	// 		"POSTGRES_DB":       "stats",
	// 		"POSTGRES_USER":     "stats",
	// 		"POSTGRES_PASSWORD": "n0uejXPl61ci6ldCuE2gQU5Y",
	// 	},
	// 	HealthCheck: &HealthCheck{
	// 		Test:        []string{"CMD-SHELL", "pg_isready -U stats -d stats"},
	// 		Interval:    "10s",
	// 		Timeout:     "5s",
	// 		Retries:     5,
	// 		StartPeriod: "10s",
	// 	},
	// 	DependsOn: map[string]interface{}{
	// 		fmt.Sprintf("blockscout-stats-db-init-%s", node): map[string]string{
	// 			"condition": "service_completed_successfully",
	// 		},
	// 	},
	// }

	return services
}

// getBlockscoutBackendService generates the Blockscout backend (Elixir) service
func getBlockscoutBackendService(node string, portBase, chainID, participantIdx int) *Service {
	rpcPort := 8545 + participantIdx
	privacyNode := fmt.Sprintf("privacy-node-%s", node)

	return &Service{
		// Backend and frontend must be a same-era matched pair: Docker Hub
		// `blockscout/blockscout:latest` is months stale while the ghcr frontend
		// moves, and the skew breaks the UI (CORS preflight rejects the
		// updated-gas-oracle header; /api/v2/search renamed address ->
		// address_hash, crashing the search bar). If either is bumped, bump both
		// and smoke-test those two endpoints.
		Image:           "ghcr.io/blockscout/blockscout:9.0.2",
		Restart:         "always",
		StopGracePeriod: "5m",
		Command:         `sh -c "bin/blockscout eval \"Elixir.Explorer.ReleaseTasks.create_and_migrate()\" && bin/blockscout start"`,
		ExtraHosts:      []string{"host.docker.internal:host-gateway"},
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:4000", portBase+2),
		},
		Volumes: []string{
			fmt.Sprintf("blockscout-logs-%s:/app/logs", node),
			fmt.Sprintf("blockscout-dets-%s:/app/dets", node),
		},
		Environment: map[string]string{
			"ETHEREUM_JSONRPC_VARIANT":   "geth",
			"ETHEREUM_JSONRPC_HTTP_URL":  fmt.Sprintf("http://%s:%d", privacyNode, rpcPort),
			"ETHEREUM_JSONRPC_TRACE_URL": fmt.Sprintf("http://%s:%d", privacyNode, rpcPort),
			"ETHEREUM_JSONRPC_WS_URL":    fmt.Sprintf("ws://%s:%d", privacyNode, rpcPort),
			"ETHEREUM_JSONRPC_TRANSPORT": "http",
			"DATABASE_URL":               fmt.Sprintf("postgresql://blockscout:ceWb1MeLBEeOIfk65gU8EjF8@blockscout-db-%s:5432/blockscout", node),
			"SECRET_KEY_BASE":            "56NtB48ear7+wMSf0IQuWDAAazhpb31qyc7GiyspBP2vh7t5zlCsF5QDv76chXeN",
			"PORT":                       "4000",
			"CHAIN_ID":                   fmt.Sprintf("%d", chainID),
			"COIN_NAME":                  "ETH",
			"INDEXER_DISABLE_PENDING_TRANSACTIONS_FETCHER": "true",
			"DISABLE_EXCHANGE_RATES":                       "true",
			"DISABLE_INDEXER":                              "false",
			"ECTO_USE_SSL":                                 "false",
			"MIX_ENV":                                      "prod",
			"BLOCKSCOUT_PROTOCOL":                          "http",
			"API_V2_ENABLED":                               "true",
		},
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD-SHELL", "curl -f http://localhost:4000/api/health/liveness || curl -f http://localhost:4000/api || exit 1"},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     10,
			StartPeriod: "180s",
		},
		DependsOn: map[string]interface{}{
			fmt.Sprintf("blockscout-db-%s", node): map[string]string{
				"condition": "service_healthy",
			},
			privacyNode: map[string]string{
				"condition": "service_healthy",
			},
		},
	}
}

// getBlockscoutFrontendService generates the Blockscout frontend (Next.js) service
func getBlockscoutFrontendService(node string, portBase, chainID, participantIdx int) *Service {
	nodeUpper := strings.ToUpper(node)

	return &Service{
		// Pinned as the matched pair of ghcr.io/blockscout/blockscout:9.0.2 —
		// see getBlockscoutBackendService before bumping.
		Image:   "ghcr.io/blockscout/frontend:v2.3.5",
		Restart: "always",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:3000", portBase+3),
		},
		Environment: map[string]string{
			"NEXT_PUBLIC_APP_HOST":               "localhost",
			"NEXT_PUBLIC_APP_PORT":               fmt.Sprintf("%d", portBase+4),
			"NEXT_PUBLIC_APP_PROTOCOL":           "http",
			"NEXT_PUBLIC_API_HOST":               "localhost",
			"NEXT_PUBLIC_API_PORT":               fmt.Sprintf("%d", portBase+2),
			"NEXT_PUBLIC_API_PROTOCOL":           "http",
			"NEXT_PUBLIC_API_WEBSOCKET_PROTOCOL": "ws",
			// "NEXT_PUBLIC_STATS_API_HOST":            fmt.Sprintf("http://localhost:%d", portBase+5),
			// "NEXT_PUBLIC_VISUALIZE_API_HOST":        fmt.Sprintf("http://localhost:%d", portBase+6),
			"NEXT_PUBLIC_NETWORK_NAME":              fmt.Sprintf("Rayls Privacy Node %s", nodeUpper),
			"NEXT_PUBLIC_NETWORK_SHORT_NAME":        fmt.Sprintf("Rayls-%s", nodeUpper),
			"NEXT_PUBLIC_NETWORK_ID":                fmt.Sprintf("%d", chainID),
			"NEXT_PUBLIC_NETWORK_CURRENCY_NAME":     "Ether",
			"NEXT_PUBLIC_NETWORK_CURRENCY_SYMBOL":   "ETH",
			"NEXT_PUBLIC_NETWORK_CURRENCY_DECIMALS": "18",
			"NEXT_PUBLIC_IS_TESTNET":                "false",
			"NEXT_PUBLIC_NETWORK_LOGO":              "https://i.postimg.cc/Qsy9Hytg/Rayls-Logo-Black.png",
			"NEXT_PUBLIC_NETWORK_LOGO_DARK":         "https://i.postimg.cc/NMXxXWrY/Rayls-Logo-Gradient.png",
			"NEXT_PUBLIC_NETWORK_ICON":              "https://i.postimg.cc/zBhkh9H6/Rayls-App-Gradient-BG.png",
			"NEXT_PUBLIC_NETWORK_ICON_DARK":         "https://i.postimg.cc/zBhkh9H6/Rayls-App-Gradient-BG.png",
			"NEXT_PUBLIC_API_BASE_PATH":             "/",
			"NEXT_PUBLIC_HOMEPAGE_CHARTS":           "['daily_txs']",
			"NEXT_PUBLIC_API_SPEC_URL":              "https://raw.githubusercontent.com/blockscout/blockscout-api-v2-swagger/main/swagger.yaml",
			"NEXT_PUBLIC_AD_BANNER_PROVIDER":        "none",
			"NEXT_PUBLIC_AD_TEXT_PROVIDER":          "none",
		},
		DependsOn: map[string]interface{}{
			fmt.Sprintf("blockscout-backend-%s", node): map[string]string{
				"condition": "service_healthy",
			},
		},
	}
}

// getBlockscoutStatsService generates the stats service
func getBlockscoutStatsService(node string, portBase int) *Service {
	return &Service{
		Image:    "ghcr.io/blockscout/stats:latest",
		Platform: "linux/amd64",
		Restart:  "always",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:8050", portBase+5),
		},
		Environment: map[string]string{
			"STATS__DB_URL":             fmt.Sprintf("postgresql://stats:n0uejXPl61ci6ldCuE2gQU5Y@blockscout-stats-db-%s:5432/stats", node),
			"STATS__BLOCKSCOUT_DB_URL":  fmt.Sprintf("postgresql://blockscout:ceWb1MeLBEeOIfk65gU8EjF8@blockscout-db-%s:5432/blockscout", node),
			"STATS__BLOCKSCOUT_API_URL": fmt.Sprintf("http://blockscout-backend-%s:4000", node),
			"STATS__CREATE_DATABASE":    "true",
			"STATS__RUN_MIGRATIONS":     "true",
			"STATS__SERVER__HTTP__ADDR": "0.0.0.0:8050",
		},
		DependsOn: map[string]interface{}{
			fmt.Sprintf("blockscout-stats-db-%s", node): map[string]string{
				"condition": "service_healthy",
			},
			fmt.Sprintf("blockscout-backend-%s", node): map[string]string{
				"condition": "service_healthy",
			},
		},
	}
}

// getBlockscoutVisualizerService generates the visualizer service
// func getBlockscoutVisualizerService(node string, portBase int) *Service {
// 	return &Service{
// 		Image:    "ghcr.io/blockscout/visualizer:latest",
// 		Platform: "linux/amd64",
// 		Restart:  "always",
// 		Ports: []string{
// 			fmt.Sprintf("127.0.0.1:%d:8081", portBase+6),
// 		},
// 		Environment: map[string]string{
// 			"VISUALIZER__SERVER__HTTP__ENABLED": "true",
// 			"VISUALIZER__SERVER__HTTP__ADDR":    "0.0.0.0:8081",
// 		},
// 	}
// }

// getBlockscoutNginxConfigInitService generates an init container that writes nginx config to volume
func getBlockscoutNginxConfigInitService(node string) *Service {
	nginxConfig := GetBlockscoutNginxConfig(node)
	// Escape single quotes and newlines for shell command
	escapedConfig := strings.ReplaceAll(nginxConfig, "'", "'\\''")

	return &Service{
		Image: "alpine:latest",
		Volumes: []string{
			fmt.Sprintf("blockscout-nginx-config-%s:/etc/nginx", node),
		},
		EntryPoint: []string{
			"sh",
			"-c",
			fmt.Sprintf("printf '%%s' '%s' > /etc/nginx/nginx.conf", escapedConfig),
		},
	}
}

// getBlockscoutProxyService generates the nginx proxy service
func getBlockscoutProxyService(node string, portBase int) *Service {
	return &Service{
		Image:   "nginx:alpine",
		Restart: "always",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:80", portBase+4),
		},
		Volumes: []string{
			fmt.Sprintf("blockscout-nginx-config-%s:/etc/nginx:ro", node),
		},
		DependsOn: map[string]interface{}{
			fmt.Sprintf("blockscout-nginx-config-init-%s", node): map[string]string{
				"condition": "service_completed_successfully",
			},
			fmt.Sprintf("blockscout-backend-%s", node): map[string]string{
				"condition": "service_started",
			},
			fmt.Sprintf("blockscout-frontend-%s", node): map[string]string{
				"condition": "service_started",
			},
		},
	}
}

// GetBlockscoutNginxConfig generates the nginx config for a Blockscout node
func GetBlockscoutNginxConfig(node string) string {
	return fmt.Sprintf(`events {
    worker_connections 1024;
}

http {
    upstream backend {
        server blockscout-backend-%s:4000;
    }

    upstream frontend {
        server blockscout-frontend-%s:3000;
    }

    # upstream stats {
    #     server blockscout-stats-%s:8050;
    # }

    # upstream visualizer {
    #    server blockscout-visualizer-%s:8081;
    #}

    server {
        listen 80;

        location /api {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $$http_upgrade;
            proxy_set_header Connection 'upgrade';
            proxy_set_header Host $$host;
            proxy_cache_bypass $$http_upgrade;
        }

        location /socket {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $$http_upgrade;
            proxy_set_header Connection "Upgrade";
            proxy_set_header Host $$host;
        }

        # location /stats-api {
        #     proxy_pass http://stats/;
        #     proxy_http_version 1.1;
        # }

        # location /visualizer-api {
        #    proxy_pass http://visualizer/;
        #    proxy_http_version 1.1;
        # }

        location / {
            proxy_pass http://frontend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $$http_upgrade;
            proxy_set_header Connection 'upgrade';
            proxy_set_header Host $$host;
            proxy_cache_bypass $$http_upgrade;
        }
    }
}
`, node, node, node, node)
}
