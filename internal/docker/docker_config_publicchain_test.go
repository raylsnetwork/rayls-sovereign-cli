package docker

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetDemoComposeConfigPublicChain(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, &pc, false, false, nil)

	for _, name := range []string{"pubrelayer-a", "pubrelayer-b"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("expected service %q to be generated", name)
		}
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "PUBLIC_CHAIN_ENABLED=true") {
		t.Errorf("expected PUBLIC_CHAIN_ENABLED=true on contracts service")
	}
	if !strings.Contains(s, "PUBLIC_CHAIN_RPC_URL=https://testnet-rpc.rayls.com/") {
		t.Errorf("expected rayls testnet RPC URL")
	}
	if !strings.Contains(s, "PUBLIC_CHAIN_ID=7295799") {
		t.Errorf("expected chain id 7295799")
	}
	// No embedded default: a hardcoded fallback would be a shared secret
	// shipped in every generated compose file.
	if !strings.Contains(s, "PUBLIC_CHAIN_PRIVATE_KEY=${DEMO_PUBLIC_CHAIN_PRIVATE_KEY:-${PUBLIC_CHAIN_PRIVATE_KEY:-}}") {
		t.Errorf("expected public chain private key passthrough (DEMO_ override, canonical var, NO embedded default)")
	}
	// Unconditional in every mode: the CLI never runs ops-api and the 3.0.1
	// deploy hard-fails its ops-api bindings step without this.
	if !strings.Contains(s, "OPS_API_ENABLED=false") {
		t.Errorf("expected OPS_API_ENABLED=false on contracts service")
	}
}

func TestGetDemoComposeConfigPublicChainOmitted(t *testing.T) {
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, nil, false, false, nil)
	for _, name := range []string{"pubrelayer-a", "pubrelayer-b"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("service %q should not be present when public chain is disabled", name)
		}
	}
}

// Regression: `rayls init --full` without --public-chain (pc == nil) must
// still pass PRIVATE_KEY_SYSTEM to the contracts deploy — hardhat.config.ts
// declares the custom_pnh/custom_pn networks with
// accounts: [PRIVATE_KEY_SYSTEM] whenever PNH_RPC_URL/PRIVACY_NODE_RPC_URL
// are set (always), and hardhat validates the config on EVERY invocation, so
// a missing key aborts the deploy with HH8 "Invalid account: #0 ... received
// undefined" before any task runs.
func TestGetDemoComposeConfigContractsAlwaysHaveSystemKey(t *testing.T) {
	pc := PublicChainPresets["local"]
	cases := map[string]*DockerCompose{
		"full no public chain":    GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, nil, false, false, nil),
		"full local public chain": GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, &pc, false, false, nil),
		"lean hub-less":           GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, true, nil),
	}
	for name, compose := range cases {
		contracts, ok := compose.Services["contracts"]
		if !ok {
			t.Fatalf("%s: contracts service missing", name)
		}
		envList, ok := contracts.Environment.([]string)
		if !ok {
			t.Fatalf("%s: contracts environment is not []string", name)
		}
		found := false
		for _, e := range envList {
			if strings.HasPrefix(e, "PRIVATE_KEY_SYSTEM=") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: contracts environment must include PRIVATE_KEY_SYSTEM (needed by hardhat custom_pnh/custom_pn network validation)", name)
		}
	}
}

// --with-hub on a --local (source-built, >= 3.0.1) stack is a FUNCTIONAL hub:
// the private relayer and proofs-api are kept so PN<->PNH messaging and Enygma
// actually work. The contracts image is the shared :latest local build
// (HUB_ENABLED-aware mainline deploy), not the pulled-mode lean-no-pnh hybrid
// tag.
func TestGetDemoComposeConfigLeanWithHubLocal(t *testing.T) {
	pc := PublicChainPresets["local"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, false, nil)

	for _, name := range []string{"private-network-hub", "relayer-a", "proofs-api", "kos-a", "pubrelayer-a", "contracts", "privacy-node-a", "public-chain", "postgres", "nats"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("with-hub local: expected service %q to be present", name)
		}
	}
	for _, name := range []string{"governance-api", "governance-listener", "governance-flagger", "audit-explorer"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("with-hub local: service %q should be omitted", name)
		}
	}
	if img := compose.Services["contracts"].Image; img != "rayls-contracts:latest" {
		t.Errorf("with-hub local: contracts image should be the shared local build rayls-contracts:latest, got %q", img)
	}
}

func TestGetDemoComposeConfigLean(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, false, &pc, true, false, nil)

	// The lean with-hub bridge keeps the functional hub set — PNH, private
	// relayer, proofs-api — regardless of image provenance (the topology is
	// not forked on pulled-vs-local; see applyLeanNoPNH). KOS stays because
	// the pubrelayer fetches its signing keys from it (cts-a alias).
	for _, name := range []string{"postgres", "nats", "privacy-node-a", "contracts", "kos-a", "pubrelayer-a", "private-network-hub", "relayer-a", "proofs-api"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("lean mode: expected service %q to be present", name)
		}
	}
	// ...and drops governance and the audit explorer (--full territory).
	for _, name := range []string{"governance-api", "governance-listener", "governance-flagger", "audit-explorer"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("lean mode: service %q should be omitted", name)
		}
	}

	// Surviving services must not depend on any removed service.
	for svcName, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if _, ok := compose.Services[dep]; !ok {
				t.Errorf("lean mode: service %q depends on removed service %q", svcName, dep)
			}
		}
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "GOVERNANCE_ENABLED=false") {
		t.Errorf("lean mode: expected GOVERNANCE_ENABLED=false on contracts service")
	}
	if !strings.Contains(string(out), "PNH_ENABLED=true") {
		t.Errorf("lean mode: expected PNH_ENABLED=true on contracts service (minimal PNH stays for CTS)")
	}
}

func TestGetDemoComposeConfigPersistenceVolumes(t *testing.T) {
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, nil, false, false, nil)
	for _, vol := range []string{"postgres-data", "commit-chain-data", "privacy-node-a-data", "privacy-node-b-data"} {
		if _, ok := compose.Volumes[vol]; !ok {
			t.Errorf("expected named volume %q to be declared", vol)
		}
	}
}

// The `local` preset runs the public chain inside the stack: the generator
// must emit the axyl public-chain pair, wire contracts to wait on it, and use
// the genesis-funded system key (NOT a user-supplied testnet key, which would
// be unfunded on the local chain).
func TestGetDemoComposeConfigLocalPublicChain(t *testing.T) {
	pc := PublicChainPresets["local"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, false, nil)

	for _, name := range []string{"public-chain", "public-chain-init", "pubrelayer-a", "relayer-a", "proofs-api"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("local-pc: expected service %q to be present", name)
		}
	}
	if _, ok := compose.Volumes["public-chain-data"]; !ok {
		t.Errorf("local-pc: expected public-chain-data volume")
	}
	dep, ok := compose.Services["contracts"].DependsOn["public-chain"].(map[string]string)
	if !ok || dep["condition"] != "service_healthy" {
		t.Errorf("local-pc: contracts should wait on public-chain (service_healthy), got %v", dep)
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "PUBLIC_CHAIN_RPC_URL=http://public-chain:8845") {
		t.Errorf("local-pc: expected the in-stack public chain RPC on the contracts service")
	}
	if !strings.Contains(s, "PUBLIC_CHAIN_ID=7331") {
		t.Errorf("local-pc: expected chain id 7331")
	}
	if strings.Contains(s, "PUBLIC_CHAIN_PRIVATE_KEY") {
		t.Errorf("local-pc: PUBLIC_CHAIN_PRIVATE_KEY must not be set — the deploy must fall back to the genesis-funded PRIVATE_KEY_SYSTEM")
	}
}

// The external testnet preset must not spawn a local public-chain container.
func TestGetDemoComposeConfigTestnetHasNoLocalPublicChain(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, false, nil)
	for _, name := range []string{"public-chain", "public-chain-init"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("testnet: service %q should not be generated", name)
		}
	}
	if _, ok := compose.Volumes["public-chain-data"]; ok {
		t.Errorf("testnet: public-chain-data volume should not be declared")
	}
}
