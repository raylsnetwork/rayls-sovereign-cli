# Local dev mode (`--local`, `rayls dev`, `rayls watch`)

This is the "hack on Rayls source" workflow: run the stack from source instead
of the published ECR images, edit a component in a local checkout, and get
hot reload. Three pieces work together:

| Command | What it does |
|---|---|
| `rayls init --local` | Build every Rayls component **from source** (a pinned git ref) instead of pulling ECR images. |
| `rayls dev <component>` | Flip one component from the pinned build to a **local checkout you can edit** (clones it if needed). |
| `rayls watch` | Hot reload — sync your file saves into the running container; `air` rebuilds & restarts in seconds. |

---

## 1. `rayls init --local` — build from source, no clone needed

`--local` builds each Rayls component from a **git build context** — Docker/BuildKit
fetches the repo at a pinned ref and builds it directly (`context: <repo>#<ref>`).
**Nothing is cloned to disk**; the build happens straight from the git ref, and
the result is byte-equivalent to the published ECR image (it uses the same
production Dockerfile).

Components and the refs they build from (`internal/docker/sources.go`):

| Component (`Key`) | Repo | Default ref | Images built |
|---|---|---|---|
| `contracts` | `rayls-sovereign-contracts` | `main` | `contracts` (deploy tooling) |
| `relayer` | `rayls-sovereign-relayer` | `main` | `kos` (CTS) + `pubrelayer` + `relayer` (private) |
| `governance` | `rayls-sovereign-pnh-governance` | `main` | `governance-api` + `-listener` + `-flagger` |
| `gnark` | `rayls-sovereign-gnark-api` | `main` | `proofs-api` (Enygma proofs) |
| `auditor` | `rayls-sovereign-pnh-auditor-ui` | `main` | `audit-explorer` (Angular → nginx) |

The `rayls-sovereign-*` repos hold the **3.0.1** code as a single `main` branch
(no version tags yet), so `main` is the default build ref. `governance`, `gnark`
and the private `relayer` only appear in hub topologies (`--with-hub` / `--full`);
`auditor` is `--full` only. The only Rayls images **not** source-built are the
infra ones — `nats`, `private-network-hub` (Besu) — plus the `axyl` privacy node
(pulled + retagged); everything else builds from the repos above.

> ⚠️ **gnark uses Git-LFS.** Its proving/verifying keys under `last_build/` are
> Git-LFS blobs. A pinned **git-context** build (the default `--local`) does *not*
> smudge LFS, so `proofs-api` would ship pointer files and fail to load keys. For
> a working Enygma stack, either `rayls dev gnark` (clones with `git lfs pull` — a
> local checkout, so LFS is smudged) or use the pulled ECR image. Non-Enygma
> stacks (the hub-less default) never build `proofs-api`, so this doesn't apply.

> **ops-api excluded on purpose.** The new `rayls-sovereign-ops-api` (the former
> backend) is not run by the CLI and is not in the registry.

The sovereign repos are **public**, so the default git contexts use **https** —
no ssh-agent needed. If you override a component to a private fork with a
`git@github.com:…` / `ssh://…` URL (via `<PREFIX>_REPO`), the build forwards your
**ssh-agent** (`build.ssh: [default]`) automatically; make sure your key is loaded
(`ssh-add -l`).

### Overriding refs / repos (per stack, via `.env`)

The generated compose wraps each build context as
`${<PREFIX>_SRC:-<repo>#<ref>}`, and resolves ref/repo with this precedence:

```
ref:   <PREFIX>_REF   >   v<RAYLS_VERSION>   >   the component's DefaultRef
repo:  <PREFIX>_REPO  >   the registry default
src:   <PREFIX>_SRC   (a local checkout path or any git URL; managed by `rayls dev`)
```

`<PREFIX>` is `CONTRACTS`, `RELAYER`, `GOVERNANCE`, `GNARK`, or `AUDITOR`.
Examples (edit `.env` in the stack dir — no regeneration needed, compose
interpolates at build time):

```dotenv
RELAYER_REF=my-feature-branch      # build the relayer from a different branch
CONTRACTS_REPO=git@github.com:you/rayls-sovereign-contracts.git   # build from your fork
RAYLS_VERSION=3.0.2                # default ref -> v3.0.2 for all components (needs matching vX.Y.Z tags; the sovereign repos have none yet)
```

---

## 2. `rayls dev <component>` — edit a component locally

`rayls dev` switches a component from the pinned git-context build to a **local
checkout you can edit**. It:

1. **Clones the repo** next to the stack directory if you don't already have a
   checkout. Location precedence: `--src <path>` (use an existing checkout) →
   `RAYLS_SRC_DIR` (parent dir for checkouts) → the stack directory's parent.
   Use `--repo <git-url>` to clone your fork instead of the default.
2. Records the checkout path as **`<PREFIX>_SRC`** in `.env` and regenerates
   **`docker-compose.override.yaml`** so the service builds from your checkout,
   using the component's **air-based dev Dockerfile** (hot reload) where
   supported.
3. **Rebuilds just that component's services.** The rest of the stack (Mongo,
   chain, NATS state) keeps running.

```bash
rayls dev relayer                        # hack on the relayer (clones if needed)
rayls dev relayer contracts              # both at once
rayls dev relayer --repo git@github.com:you/rayls-sovereign-relayer.git  # your fork
rayls dev relayer --src ~/code/rayls-sovereign-relayer                   # existing checkout
rayls dev --status                       # which components are in dev mode?
rayls dev --off relayer                  # back to the pinned from-source build
```

`--repo` / `--src` apply to a **single** component at a time.

### Which components hot-reload?

| Component | Hot reload (`Watch`) | Notes |
|---|---|---|
| `relayer` (kos + pubrelayer + private relayer) | ✅ | air rebuilds on save |
| `contracts` | ❌ | builds from your checkout, but **redeploys stay explicit** — a file watcher must not silently redeploy contracts |
| `governance` | ❌ | builds from your checkout (production Dockerfiles); rebuild to apply |
| `gnark` | ❌ | builds from your checkout; needs Git-LFS (see the ⚠️ note above) |
| `auditor` | ❌ | Angular → nginx build from your checkout; rebuild to apply |

---

## 3. `rayls watch` — hot reload loop

```bash
rayls watch
```

Runs `docker compose watch` in the foreground. Every file save in a dev-mode
component's checkout syncs into its container, where `air` rebuilds and restarts
the service — typically a few seconds. Stack state (Mongo, chain, NATS) survives
the restarts. Press **Ctrl-C** to stop watching; the stack keeps running.

Requires **Docker Compose v2** (`docker compose watch` needs ≥ 2.22; the dev
override's `build: !override` needs ≥ 2.24). Compose v1 (`docker-compose`) is not
supported for watch.

---

## Typical workflow

```bash
rayls init --local           # 1. build the whole stack from pinned source
rayls dev relayer            # 2. clone + switch the relayer to a local checkout
rayls watch                  # 3. edit relayer source; saves hot-reload in ~seconds
# ...iterate...
rayls dev --off relayer      # 4. done — back to the pinned build
```

For **contracts**, step 3 is different: edit your checkout, then trigger a
redeploy explicitly (there's no file watcher for contracts by design).

---

## How it's wired (for maintainers)

- Component registry + build resolution: `internal/docker/sources.go`
  (`Components`, `Sources`, `BuildContext`, `BuildSection`).
- `rayls dev` / `rayls watch` orchestration: `internal/stacks/dev.go`
  (`EnableDev`, `DisableDev`, `DevStatus`, `RegenerateDevOverride`, `Watch`) and
  `cmd/dev.go` / `cmd/watch.go`.
- `.env` read/write for the `_SRC`/`_REF`/`_REPO` overrides: `internal/envfile/`.
- The generated `docker-compose.override.yaml` carries a marker line; the CLI
  refuses to clobber a hand-written override.
