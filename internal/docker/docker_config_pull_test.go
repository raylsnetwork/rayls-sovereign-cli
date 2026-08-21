package docker

import (
	"strings"
	"testing"
)

// pullPolicySkipped mirrors what `docker compose pull` skips: services that
// build from source and services told never to pull. Everything else is fetched
// from a registry — by the CLI's sequential pull step if it runs, by `up` in
// parallel if it doesn't.
func pullPolicySkipped(s *Service) bool {
	return s.Image == "" || s.PullPolicy == "never" || s.PullPolicy == "build"
}

// --local still needs a pull step: the source-built components are skipped, but
// the infra images come from a registry and are exactly what tripped ECR
// Public's per-IP limit when `up` fetched them in parallel.
func TestLocalStackStillHasRegistryImages(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate from any real .env
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	pc := PublicChainPresets["rayls-testnet"]
	compose := GetDemoComposeConfig([]string{"a"}, false, []string{"a"}, true, &pc, true, false, srcs)

	pulled := map[string]bool{}
	for name, svc := range compose.Services {
		if pullPolicySkipped(svc) {
			continue
		}
		pulled[svc.Image] = true
		if strings.HasPrefix(svc.Image, raylsECRPrefix) {
			continue
		}
		if strings.Contains(svc.Image, "/") || strings.Contains(svc.Image, ":") {
			continue // public registry image (postgres, nginx, blockscout, ...)
		}
		t.Errorf("service %s pulls a short-name image %q — nothing would provide it", name, svc.Image)
	}

	// ECR images --local deliberately keeps pulling (no source-build path).
	// Only the genuinely non-source-built infra remains: NATS and the Besu
	// private-network-hub. The relayer, proofs-api (gnark), governance trio and
	// audit-explorer are now source-built (see the Components registry).
	for _, image := range []string{
		raylsECRPrefix + "rayls-nats:latest",
		raylsECRPrefix + "rayls-private-network-hub:latest",
	} {
		if !pulled[image] {
			t.Errorf("%s should still be pulled in --local mode", image)
		}
	}
	// Source-built components must not be in the pull set.
	for image := range pulled {
		for _, short := range []string{
			"rayls-kos:", "rayls-pubrelayer:", "rayls-relayer:", "rayls-contracts:",
			"rayls-privacy-axyl:", "rayls-proof-api:",
			"rayls-governance-api:", "rayls-governance-listener:", "rayls-governance-flagger:",
			"rayls-audit-explorer:",
		} {
			if strings.HasPrefix(image, short) {
				t.Errorf("%s should be built/never-pulled in --local mode, not pulled", image)
			}
		}
	}
}

// Build contexts must never be an absolute path from whoever generated the
// file: compose then treats the service as buildable on every machine, and a
// registry failure surfaces as "unable to prepare context: path not found".
func TestNoHostSpecificBuildContexts(t *testing.T) {
	t.Chdir(t.TempDir())
	srcs, err := ResolveSources()
	if err != nil {
		t.Fatalf("ResolveSources: %v", err)
	}
	pc := PublicChainPresets["rayls-testnet"]

	for _, tc := range []struct {
		name  string
		local bool
		srcs  *Sources
	}{
		{"published images", false, nil},
		{"source builds", true, srcs},
	} {
		compose := GetDemoComposeConfig([]string{"a"}, true, []string{"a"}, tc.local, &pc, false, false, tc.srcs)
		for name, svc := range compose.Services {
			if svc.Build == nil {
				continue
			}
			ctx := svc.Build.Context
			if strings.HasPrefix(ctx, "/") || strings.HasPrefix(ctx, "~") {
				t.Errorf("%s: service %s builds from host path %q", tc.name, name, ctx)
			}
			if !tc.local {
				t.Errorf("%s: service %s should not carry a build section (context %q)", tc.name, name, ctx)
			}
		}
	}
}

// The blockscout explorer runs the published image; nothing local builds it.
func TestBlockscoutBackendPullsPublishedImage(t *testing.T) {
	compose := GetDemoComposeConfig([]string{"a"}, false, []string{"a"}, false, nil, false, false, nil)
	svc := compose.Services["blockscout-backend-a"]
	if svc == nil {
		t.Fatal("blockscout-backend-a missing")
	}
	if svc.Build != nil {
		t.Errorf("blockscout-backend-a should have no build section, got context %q", svc.Build.Context)
	}
	// Backend + frontend are a pinned matched pair (Docker Hub :latest is
	// stale and skews against the ghcr frontend, breaking CORS and search).
	if svc.Image != "ghcr.io/blockscout/blockscout:9.0.2" {
		t.Errorf("blockscout-backend-a image = %q, want ghcr.io/blockscout/blockscout:9.0.2", svc.Image)
	}
}
