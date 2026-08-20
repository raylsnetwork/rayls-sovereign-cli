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
| `contracts` | `rayls-privacy-contracts` | `lean-no-pnh-3.0.0` | `contracts` (deploy tooling) |
| `relayer` | `rayls-privacy-relayer-api` | `v3.0.0` | `kos` (CTS) + `pubrelayer` |
| `backend` | `rayls-privacy-backend` | `v3.0.0` | `backend` |

Infra images (`mongodb`, `nats`, `private-network-hub`, `axyl` privacy node) are
**not** source-built — they come from their normal images.

Because the repos are private, git contexts use SSH (`git@github.com:...`), so the
build forwards your **ssh-agent** (`build.ssh: [default]`). Make sure your key is
loaded (`ssh-add -l`). Once the repos are public over https this drops out
automatically.

### Overriding refs / repos (per stack, via `.env`)

The generated compose wraps each build context as
`${<PREFIX>_SRC:-<repo>#<ref>}`, and resolves ref/repo with this precedence:

```
ref:   <PREFIX>_REF   >   v<RAYLS_VERSION>   >   the component's DefaultRef
repo:  <PREFIX>_REPO  >   the registry default
src:   <PREFIX>_SRC   (a local checkout path or any git URL; managed by `rayls dev`)
```

`<PREFIX>` is `CONTRACTS`, `RELAYER`, or `BACKEND`. Examples (edit `.env` in the
stack dir — no regeneration needed, compose interpolates at build time):

```dotenv
RELAYER_REF=version/3.0.1          # build the relayer from a different branch
BACKEND_REPO=git@github.com:you/rayls-privacy-backend.git   # build from your fork
RAYLS_VERSION=3.0.0                # sets the default ref to v3.0.0 for all components
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
rayls dev backend                        # hack on the backend (clones if needed)
rayls dev relayer backend                # both at once
rayls dev backend --repo git@github.com:you/rayls-privacy-backend.git   # your fork
rayls dev relayer --src ~/code/rayls-privacy-relayer-api                 # existing checkout
rayls dev --status                       # which components are in dev mode?
rayls dev --off backend                  # back to the pinned from-source build
```

`--repo` / `--src` apply to a **single** component at a time.

### Which components hot-reload?

| Component | Hot reload (`Watch`) | Notes |
|---|---|---|
| `relayer` (kos + pubrelayer) | ✅ | air rebuilds on save |
| `backend` | ✅ | air rebuilds on save |
| `contracts` | ❌ | builds from your checkout, but **redeploys stay explicit** — a file watcher must not silently redeploy contracts |

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
rayls dev backend            # 2. clone + switch the backend to a local checkout
rayls watch                  # 3. edit backend source; saves hot-reload in ~seconds
# ...iterate...
rayls dev --off backend      # 4. done — back to the pinned build
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
