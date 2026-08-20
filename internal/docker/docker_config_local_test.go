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
	// Images without a source-build path are never short-named — full+--local
	// died with "No such image: rayls-proof-api:latest" when they were.
	for _, ecrImage := range []string{
		"rayls-nats:latest",
		"rayls-private-network-hub:latest",
		"rayls-proof-api:latest",
		"rayls-governance-api:latest",
		"rayls-governance-listener:latest",
		"rayls-governance-flagger:latest",
		"rayls-audit-explorer:latest",
	} {
		if !strings.Contains(s, "image: public.ecr.aws/w0k9o1t3/rayls-demo/"+ecrImage) {
			t.Errorf("%s should stay on ECR in --local", ecrImage)
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
	wantCtx := "${RELAYER_SRC:-git@github.com:raylsnetwork/rayls-privacy-relayer-api.git#v3.0.0}"
	if kosA.Build.Context != wantCtx {
		t.Errorf("kos-a context = %q, want %q", kosA.Build.Context, wantCtx)
	}
	if kosA.Build.Dockerfile != "cts/Dockerfile" {
		t.Errorf("kos-a dockerfile = %q, want cts/Dockerfile (production)", kosA.Build.Dockerfile)
	}
	if len(kosA.Build.Ssh) != 1 || kosA.Build.Ssh[0] != "default" {
		t.Errorf("kos-a should forward the ssh agent for the private git context, got %v", kosA.Build.Ssh)
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
	if got := compose.Services["contracts"].Build.Context; !strings.Contains(got, "#lean-no-pnh-3.0.0}") {
		t.Errorf("contracts context template should default to the registry's lean-no-pnh-3.0.0 ref (the .env pins override it at resolution time), got %q", got)
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
