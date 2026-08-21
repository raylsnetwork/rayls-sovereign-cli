package docker

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetDemoComposeConfigLocal(t *testing.T) {
	composeConfig := GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, nil, false, false, nil)

	out, err := yaml.Marshal(composeConfig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "image: rayls-kos:latest") {
		t.Errorf("expected short-name rayls-kos image, got:\n%s", s)
	}
	if !strings.Contains(s, "pull_policy: never") {
		t.Errorf("expected pull_policy: never for local images")
	}
	if !strings.Contains(s, "image: public.ecr.aws/w0k9o1t3/rayls-demo/rayls-nats:latest") {
		t.Errorf("nats should still pull from ECR, got:\n%s", s)
	}
	if strings.Contains(s, "image: public.ecr.aws/w0k9o1t3/rayls-demo/rayls-kos:latest") {
		t.Errorf("kos should not reference ECR when local=true")
	}
	// Images without a source-build path are never short-named — they keep the
	// full ECR ref so `up` can pull them (full+--local died with "No such
	// image" when a non-buildable image was short-named).
	for _, ecrImage := range []string{
		"rayls-nats:latest",
		"rayls-private-network-hub:latest",
	} {
		if !strings.Contains(s, "image: public.ecr.aws/w0k9o1t3/rayls-demo/"+ecrImage) {
			t.Errorf("%s should stay on ECR in --local", ecrImage)
		}
	}
	// Source-buildable images ARE short-named in --local (a local build/tag
	// provides them): the governance trio, proofs-api, audit-explorer and the
	// private relayer, alongside kos/pubrelayer/contracts.
	for _, shortImage := range []string{
		"rayls-relayer:latest",
		"rayls-governance-api:latest",
		"rayls-governance-listener:latest",
		"rayls-governance-flagger:latest",
		"rayls-proof-api:latest",
		"rayls-audit-explorer:latest",
	} {
		if !strings.Contains(s, "image: "+shortImage) {
			t.Errorf("%s should be short-named (source-buildable) in --local", shortImage)
		}
		if strings.Contains(s, "image: public.ecr.aws/w0k9o1t3/rayls-demo/"+shortImage) {
			t.Errorf("%s should not reference ECR when local=true", shortImage)
		}
	}
}

// --local with Sources: kos/pubrelayer/contracts carry build sections
// (pinned git contexts, overridable via <PREFIX>_SRC) on the first
// participant's service only; infra images (nats, PNH, axyl) are
// not source-built.
func TestGetDemoComposeConfigLocalFromSource(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate from any real .env
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a"}, false, nil, true, &pc, true, false, srcs)

	kosA := compose.Services["kos-a"]
	if kosA.Build == nil {
		t.Fatalf("kos-a should have a build section")
	}
	wantCtx := "${RELAYER_SRC:-git@github.com:raylsnetwork/rayls-sovereign-relayer.git#main}"
	if kosA.Build.Context != wantCtx {
		t.Errorf("kos-a context = %q, want %q", kosA.Build.Context, wantCtx)
	}
	if kosA.Build.Dockerfile != "cts/Dockerfile" {
		t.Errorf("kos-a dockerfile = %q, want cts/Dockerfile (production)", kosA.Build.Dockerfile)
	}
	// The sovereign repos are currently private git@ URLs, so the build forwards
	// the ssh agent. (Flip to len==0 once they go public and the URLs become https.)
	if len(kosA.Build.Ssh) != 1 || kosA.Build.Ssh[0] != "default" {
		t.Errorf("kos-a should forward the ssh agent for the private git@ context, got %v", kosA.Build.Ssh)
	}
	if kosA.PullPolicy != "build" {
		t.Errorf("kos-a pull_policy = %q, want build", kosA.PullPolicy)
	}

	for service, dockerfile := range map[string]string{
		"pubrelayer-a": "public-relayer/Dockerfile",
		"contracts":    "Dockerfile.dev",
	} {
		svc := compose.Services[service]
		if svc == nil || svc.Build == nil {
			t.Errorf("%s should have a build section", service)
			continue
		}
		if svc.Build.Dockerfile != dockerfile {
			t.Errorf("%s dockerfile = %q, want %q", service, svc.Build.Dockerfile, dockerfile)
		}
	}
	if got := compose.Services["contracts"].Build.Context; !strings.Contains(got, "#main}") {
		t.Errorf("contracts context template should default to the sovereign-contracts main ref (the .env pins override it at resolution time), got %q", got)
	}

	// Infra images stay pulled from ECR.
	for _, svc := range []string{"nats", "private-network-hub"} {
		s := compose.Services[svc]
		if s == nil {
			t.Fatalf("service %s missing", svc)
		}
		if s.Build != nil || !strings.HasPrefix(s.Image, "public.ecr.aws/") {
			t.Errorf("%s should stay a pulled ECR image, got image=%q build=%v", svc, s.Image, s.Build)
		}
	}
	if compose.Services["privacy-node-a"].Build != nil {
		t.Errorf("privacy-node-a should not build from source")
	}
}

// --local --full source-builds the remaining components too: the private
// relayer (from the relayer repo), the governance trio, proofs-api (gnark) and
// the audit explorer — each from its rayls-sovereign-* repo on main.
func TestGetDemoComposeConfigFullLocalSourceBuilds(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate from any real .env
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	pc := PublicChainPresets["rayls-testnet"]
	// full (lean=false), with hub (noHub=false), --local, from source.
	compose := GetDemoComposeConfig([]string{"a", "b"}, false, nil, true, &pc, false, false, srcs)

	cases := map[string]struct{ dockerfile, repo string }{
		"relayer-a":           {"private-relayer/Dockerfile", "rayls-sovereign-relayer"},
		"governance-api":      {"Dockerfile.api", "rayls-sovereign-pnh-governance"},
		"governance-listener": {"Dockerfile.listener", "rayls-sovereign-pnh-governance"},
		"governance-flagger":  {"Dockerfile.flagger", "rayls-sovereign-pnh-governance"},
		"proofs-api":          {"Dockerfile", "rayls-sovereign-gnark-api"},
		"audit-explorer":      {"Dockerfile", "rayls-sovereign-pnh-auditor-ui"},
	}
	for svcName, want := range cases {
		svc := compose.Services[svcName]
		if svc == nil {
			t.Errorf("%s missing in --local --full", svcName)
			continue
		}
		if svc.Build == nil {
			t.Errorf("%s should build from source in --local --full", svcName)
			continue
		}
		if svc.Build.Dockerfile != want.dockerfile {
			t.Errorf("%s dockerfile = %q, want %q", svcName, svc.Build.Dockerfile, want.dockerfile)
		}
		if !strings.Contains(svc.Build.Context, want.repo) || !strings.Contains(svc.Build.Context, "#main}") {
			t.Errorf("%s context = %q, want it to reference %s on main", svcName, svc.Build.Context, want.repo)
		}
		if svc.PullPolicy != "build" {
			t.Errorf("%s pull_policy = %q, want build", svcName, svc.PullPolicy)
		}
		if strings.HasPrefix(svc.Image, "public.ecr.aws/") {
			t.Errorf("%s should be short-named when source-built, got %q", svcName, svc.Image)
		}
	}
	// Only the first participant's private relayer carries the build; siblings
	// reuse the produced tag.
	if b := compose.Services["relayer-b"].Build; b != nil {
		t.Errorf("relayer-b should reuse relayer-a's build, not carry its own: %+v", b)
	}
}
