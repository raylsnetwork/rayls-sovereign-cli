package docker

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Hub-less pruning of the full service map: the PNH and everything PNH-scoped (proofs-api,
// private relayers, governance, audit explorer) disappears; CTS + pubrelayer
// stay and bridge the privacy nodes over the public chain only.
func TestGetDemoComposeConfigNoHubFull(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, &pc, false, true, nil)

	for _, name := range []string{
		"postgres", "nats", "certs-init",
		"privacy-node-a", "privacy-node-b", "contracts",
		"kos-a", "kos-b", "pubrelayer-a", "pubrelayer-b",
	} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("no-hub full: expected service %q to be present", name)
		}
	}
	for _, name := range []string{
		"private-network-hub", "proofs-api", "relayer-a", "relayer-b",
		"governance-api", "governance-listener", "governance-flagger", "audit-explorer",
	} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("no-hub full: service %q should be omitted", name)
		}
	}

	// The Besu commit-chain volume belongs to the removed hub.
	for _, vol := range []string{"commit-chain-data"} {
		if _, ok := compose.Volumes[vol]; ok {
			t.Errorf("no-hub: %s volume should be omitted", vol)
		}
	}
	for _, vol := range []string{"shared-config", "postgres-data", "privacy-node-a-data", "privacy-node-b-data"} {
		if _, ok := compose.Volumes[vol]; !ok {
			t.Errorf("no-hub: expected volume %q to be declared", vol)
		}
	}

	// The pubrelayer must wait on KOS (signing keys) in place of the removed
	// private relayer.
	for _, p := range []string{"a", "b"} {
		dep, ok := compose.Services["pubrelayer-"+p].DependsOn["kos-"+p].(map[string]string)
		if !ok || dep["condition"] != "service_started" {
			t.Errorf("no-hub: pubrelayer-%s should depend on kos-%s (service_started), got %v", p, p, dep)
		}
	}

	// Surviving services must not depend on any removed service.
	for svcName, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if _, ok := compose.Services[dep]; !ok {
				t.Errorf("no-hub: service %q depends on removed service %q", svcName, dep)
			}
		}
	}

	// No-hub uses the mainline deploy image (the lean-no-pnh tag predates
	// HUB_ENABLED support).
	if img := compose.Services["contracts"].Image; img != "public.ecr.aws/w0k9o1t3/rayls-demo/rayls-contracts:latest" {
		t.Errorf("no-hub: contracts image = %q, want the :latest mainline tag", img)
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "HUB_ENABLED=false") {
		t.Errorf("no-hub: expected HUB_ENABLED=false on contracts service")
	}
	if !strings.Contains(s, "GOVERNANCE_ENABLED=false") {
		t.Errorf("no-hub: expected GOVERNANCE_ENABLED=false on contracts service")
	}
	// The Endpoint initializer ABI-encodes PNH_CHAIN_ID even hub-less; 0 is the
	// no-hub sentinel (see deploy-rayup.sh).
	if !strings.Contains(s, "PNH_CHAIN_ID=0") {
		t.Errorf("no-hub: expected PNH_CHAIN_ID=0 sentinel on contracts service")
	}
	// PNH_ENABLED left unset so a pre-HUB_ENABLED deploy image fails loudly
	// instead of aliasing the PNH registry to the PN; no PNH_RPC_URL — there is
	// no private-hub host.
	if strings.Contains(s, "PNH_ENABLED=") {
		t.Errorf("no-hub: PNH_ENABLED should not be set")
	}
	if strings.Contains(s, "PNH_RPC_URL=") {
		t.Errorf("no-hub: PNH_RPC_URL should not be set")
	}
	if !strings.Contains(s, "PUBLIC_CHAIN_ENABLED=true") {
		t.Errorf("no-hub: expected PUBLIC_CHAIN_ENABLED=true (the PC is the only interconnection path)")
	}
	// The 3.0.1 deploy defaults OPS_API_ENABLED=true and exits 10 when the
	// ops-api bindings dir is missing — the CLI never runs ops-api.
	if !strings.Contains(s, "OPS_API_ENABLED=false") {
		t.Errorf("no-hub: expected OPS_API_ENABLED=false on contracts service")
	}
}

// Hub-less pruning of the lean stack removes the minimal PNH that lean otherwise keeps
// for the 3.0.0 CTS, leaving a genuinely hub-less single-PN bridge.
func TestGetDemoComposeConfigNoHubLean(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, false, &pc, true, true, nil)

	for _, name := range []string{"postgres", "nats", "privacy-node-a", "contracts", "kos-a", "pubrelayer-a"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("no-hub lean: expected service %q to be present", name)
		}
	}
	for _, name := range []string{
		"private-network-hub", "proofs-api", "relayer-a",
		"governance-api", "governance-listener", "governance-flagger", "audit-explorer",
	} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("no-hub lean: service %q should be omitted", name)
		}
	}
	for _, vol := range []string{"commit-chain-data"} {
		if _, ok := compose.Volumes[vol]; ok {
			t.Errorf("no-hub lean: %s volume should be omitted", vol)
		}
	}

	// Lean no-hub needs the mainline HUB_ENABLED-aware deploy, not the
	// lean-no-pnh tag (whose PNH_ENABLED=false path aliases the PNH registry to
	// the PN — wrong topology for a 3.0.1 CTS).
	if img := compose.Services["contracts"].Image; img != "public.ecr.aws/w0k9o1t3/rayls-demo/rayls-contracts:latest" {
		t.Errorf("no-hub lean: contracts image = %q, want the :latest mainline tag", img)
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "HUB_ENABLED=false") {
		t.Errorf("no-hub lean: expected HUB_ENABLED=false on contracts service")
	}
	if strings.Contains(s, "PNH_ENABLED=") {
		t.Errorf("no-hub lean: PNH_ENABLED should not be set")
	}

	for svcName, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if _, ok := compose.Services[dep]; !ok {
				t.Errorf("no-hub lean: service %q depends on removed service %q", svcName, dep)
			}
		}
	}
}

// Hub-less --local stacks build the contracts deploy from source in BOTH modes (no
// published image carries the HUB_ENABLED-gated deploy), from the ref pinned in
// the stack's .env by the stacks layer.
func TestGetDemoComposeConfigNoHubLocalFromSource(t *testing.T) {
	t.Chdir(t.TempDir())
	// Simulate the pin GenerateDockerCompose writes for hub-less --local stacks.
	if err := os.WriteFile(".env", []byte("CONTRACTS_REF=main\nRELAYER_REF=main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, &pc, false, true, srcs)

	contracts := compose.Services["contracts"]
	if contracts.Build == nil {
		t.Fatalf("no-hub --local: contracts should build from source (no published image has HUB_ENABLED support)")
	}
	if contracts.Build.Dockerfile != "Dockerfile.dev" {
		t.Errorf("contracts dockerfile = %q, want Dockerfile.dev", contracts.Build.Dockerfile)
	}
	if want := "${CONTRACTS_SRC:-git@github.com:raylsnetwork/rayls-sovereign-contracts.git#main}"; contracts.Build.Context != want {
		t.Errorf("contracts context = %q, want %q", contracts.Build.Context, want)
	}
	kosA := compose.Services["kos-a"]
	if kosA.Build == nil {
		t.Fatalf("kos-a should have a build section in --local")
	}
	if want := "${RELAYER_SRC:-git@github.com:raylsnetwork/rayls-sovereign-relayer.git#main}"; kosA.Build.Context != want {
		t.Errorf("kos-a context = %q, want %q", kosA.Build.Context, want)
	}
}

// Every depends_on edge must point at an existing service, and service_healthy
// edges must point at services that actually define a healthcheck — compose
// refuses to start a service whose service_healthy dependency has none. This
// invariant must hold in every topology the generator can emit.
func TestDependsOnConditionsSatisfiable(t *testing.T) {
	pc := PublicChainPresets["rayls-testnet"]
	localPC := PublicChainPresets["local"]
	cases := map[string]*DockerCompose{
		"full":              GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, nil, false, false, nil),
		"full+pc":           GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, &pc, false, false, nil),
		"full+pc+bs":        GetDemoComposeConfig([]string{"a", "b"}, true, []string{"a"}, false, &pc, false, false, nil),
		"lean+pc":           GetDemoComposeConfig([]string{"a"}, false, nil, false, &pc, true, false, nil),
		"full+pc+no-hub":    GetDemoComposeConfig([]string{"a", "b"}, false, nil, false, &pc, false, true, nil),
		"lean+pc+no-hub":    GetDemoComposeConfig([]string{"a"}, false, nil, false, &pc, true, true, nil),
		"lean+localpc":      GetDemoComposeConfig([]string{"a"}, false, nil, true, &localPC, true, false, nil),
		"nohub+localpc":     GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, &localPC, false, true, nil),
		"privacy-node-only": GetPrivacyNodeOnlyConfig(false),
	}
	for mode, compose := range cases {
		for svcName, svc := range compose.Services {
			for dep, raw := range svc.DependsOn {
				target, ok := compose.Services[dep]
				if !ok {
					t.Errorf("%s: %q depends on missing service %q", mode, svcName, dep)
					continue
				}
				cond, ok := raw.(map[string]string)
				if !ok {
					t.Errorf("%s: %q has unexpected depends_on shape for %q: %T", mode, svcName, dep, raw)
					continue
				}
				if cond["condition"] == "service_healthy" && target.HealthCheck == nil {
					t.Errorf("%s: %q requires service_healthy from %q, which has no healthcheck", mode, svcName, dep)
				}
			}
		}
	}
}

// Hub-less mode with the `local` public-chain preset is the fully self-contained
// hub-less system: the in-stack public chain must survive the no-hub pruning —
// it IS the interconnection path.
func TestGetDemoComposeConfigNoHubLocalPublicChain(t *testing.T) {
	pc := PublicChainPresets["local"]
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, &pc, false, true, nil)

	for _, name := range []string{"public-chain", "public-chain-init", "kos-a", "kos-b", "pubrelayer-a", "pubrelayer-b", "contracts"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("no-hub local-pc: expected service %q to be present", name)
		}
	}
	for _, name := range []string{"private-network-hub", "proofs-api", "relayer-a"} {
		if _, ok := compose.Services[name]; ok {
			t.Errorf("no-hub local-pc: service %q should be omitted", name)
		}
	}
	for svcName, svc := range compose.Services {
		for dep := range svc.DependsOn {
			if _, ok := compose.Services[dep]; !ok {
				t.Errorf("no-hub local-pc: service %q depends on removed service %q", svcName, dep)
			}
		}
	}
}
