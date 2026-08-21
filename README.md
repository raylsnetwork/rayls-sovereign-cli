<div align="center">

# Rayls CLI

**Provision and manage a local Rayls blockchain stack on a single host — a privacy node bridged to a public chain by default, or the full multi-participant demo.**

[![License: Apache 2.0][license-badge]][license-url]
[![Go][go-badge]][go-url]

[![Discord][discord-badge]][discord-url]
[![X][x-badge]][x-url]
[![LinkedIn][linkedin-badge]][linkedin-url]
[![YouTube][youtube-badge]][youtube-url]

[Features](#features) | [Installation](#installation) | [Usage](#usage) | [Architecture](#architecture) | [License](#license)

</div>

The **Rayls CLI** is a developer tool for provisioning and managing the [Rayls](https://rayls.com/) blockchain stack.

Currently, this tool focuses on deploying a **local demo environment** on a single host, making it ideal for sales demonstrations, proof-of-concept exploration, and local development. It automates the generation of Docker Compose configurations and manages the lifecycle of the Rayls components.

> **v2.0.0** — the privacy node is now the **Axyl** (`rayls-network`) node, running gaslessly in its local dev-mode profile, and replacing the previous Geth-based ledger. The **default** `rayls init` now spins up a single local privacy node bridged to a public chain (the primary use case); the full multi-participant demo stack moved behind `--full`.

For a deeper understanding of the Rayls architecture and ecosystem, please visit the [Official Rayls Documentation](https://docs.rayls.com/docs/a-warm-introduction-to-rayls).

## Features

*   **Local privacy node → public chain by default:** A bare `rayls init` runs a single Axyl privacy node bridged to a public chain (Rayls testnet).
*   **Gasless Axyl node:** The privacy node runs the Axyl `local` hardfork profile (EIP-1559 active from block 0 with a zero base-fee floor), so relayer/pubrelayer transactions on the node need no gas funding.
*   **Full demo stack on demand:** `--full` brings up the multi-participant stack with a local Private Network Hub (commit chain), governance, and proofs API — 2 to 6 participants.
*   **Automated Setup:** Generates a dynamic `docker-compose.yaml` tailored to your specifications.
*   **Sequential Image Pulling:** Automatically pulls container images one-by-one — in every mode, `--local` included — to stay under ECR Public's per-IP pull rate limit, cooling off and retrying if a registry throttles anyway.
*   **Lifecycle Management:** Specialized commands to start, stop, and tear down the stack.
*   **Monitoring & Observability:** Optional OpenTelemetry stack with eBPF auto-instrumentation, Grafana, Loki, Prometheus, and Tempo.
*   **Per-Node Block Explorers:** Blockscout deployment per privacy node (default-on).
*   **Version Management:** Built-in version checking and update notifications.
*   **Environment Verification:** Tools to verify the integrity of the setup, including an end-to-end public-chain bridge smoke test.

## Prerequisites

Before using the Rayls CLI, ensure you have the following installed on your system:

*   **Go** (1.25.4+ recommended) - for building the CLI.
*   **Docker** - The engine to run the containerized stack.
*   **Docker Compose** (**v2.24 or newer**) - Required for orchestration. The generated stack uses `build: !override` and `develop.watch`, which older versions cannot parse.

## Funding

Stacks that bridge to the **Rayls testnet** (what a bare `rayls init` does) deploy the public-chain contracts and seed each participant's relayer wallets **from your own deployer key**. Funding happens outside the CLI:

1. Create a fresh key with any wallet tool (e.g. `cast wallet new`).
2. Request testnet RAYLS for its address through the [Rayls community](https://www.rayls.com/community). Budget roughly **5 RAYLS per participant** (each participant's public-relayer wallets are seeded with 2.5 RAYLS, plus deploy gas).
3. Run `./rayls init`; it prompts for the key (input hidden, `0x` prefix optional) and stores it in the stack directory's `.env` (created with `0600` permissions), where every later `rayls` / `docker compose` run picks it up automatically. For CI or scripting, set `PUBLIC_CHAIN_PRIVATE_KEY=<hex>` in the environment instead because it overrides `.env` and is never written to disk.

`rayls init` **preflights the balance**: it derives your key's address, queries the chain, and refuses immediately (naming the account, balance, and shortfall) if it can't cover a fresh deploy (~2 RAYLS gas + 2.5 per participant). This replaces the opaque mid-deploy failures an underfunded key used to cause. Note that every fresh init (after `rayls down -v`) spends that amount again, and each restart of the contracts container re-seeds the relayer wallets with 2.5, so prefer `rayls stop`/`start` over wipe-and-redeploy while iterating.

Use a **testnet-only key** and never reuse a mainnet key: like any compose environment value it is visible in `docker inspect` on your machine. Fully local stacks (`rayls init --local`, `--privacy-node-only`, or `--full` without a public chain) need no funding at all since their chains run in-stack and are genesis-funded.

## Installation

### Option 1: Download Pre-built Binary (Recommended)

Download the latest binary for your platform:

**macOS (Apple Silicon):**
```bash
curl -fL https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64 -o rayls
chmod +x rayls
sudo mv rayls /usr/local/bin/
```

**macOS (Intel):**
```bash
curl -fL https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-amd64 -o rayls
chmod +x rayls
sudo mv rayls /usr/local/bin/
```

**Linux (x86_64):**
```bash
curl -fL https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-amd64 -o rayls
chmod +x rayls
sudo mv rayls /usr/local/bin/
```

**Linux (ARM64):**
```bash
curl -fL https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-arm64 -o rayls
chmod +x rayls
sudo mv rayls /usr/local/bin/
```

### Option 2: Build from Source

**1. Clone the Repository:**
```bash
git clone <repository-url>
cd rayls-sovereign-cli
```

**2. Build the CLI:**
```bash
go build -o rayls .
```

*Optionally, move the `rayls` binary to a directory in your system's `$PATH` for global access.*

## Usage

The CLI follows a standard verb-based command structure.

### Initialization

To set up a new environment, run the `init` command. This will:
1. Generate the necessary `docker-compose.yaml` file
2. Pull all container images sequentially (to avoid registry rate limits)
3. Start all Docker containers

```bash
./rayls init
```

By default this spins up **Axyl privacy node(s) bridged to a public chain**, with **hub-less as the default topology**: the nodes intercommunicate via the public chain only. With `--local` everything runs on your machine (source builds + an in-stack Axyl public chain). Pulled-image inits (no `--local`) bridge to the Rayls testnet and keep a minimal Private Network Hub for now (the published images predate hub-less support), deploying from **your own funded testnet key**: `rayls init` prompts for it and saves it to the stack `.env` (see [Funding](#funding)). Every participant also gets a Blockscout explorer by default (`--no-blockscout` disables). `--with-hub` opts a lean stack into the minimal hub explicitly; `--full` brings the complete hub demo stack.

If a `docker-compose.yaml` already exists, you'll be prompted to overwrite or use the existing file. If you choose to keep the existing file, the CLI will proceed with pulling images and starting containers.

**Options:**
*   `--full`: Bring up the full multi-participant demo stack (local Private Network Hub / commit chain, governance, proofs API, multiple privacy nodes). Combine with `--members` and/or `--public-chain`.
*   `--with-hub`: Include the **Private Network Hub** in the default (lean) stack — a **functional hub**: PNH plus the private relayer and proofs-api, so PN↔PNH messaging and Enygma work. Without it, **hub-less is the default**: no PNH, no private relayer, no proofs API — the privacy nodes intercommunicate via the public chain only. Hub-less currently applies to `--local` stacks only: it needs the 3.0.1 `rayls-sovereign-contracts` / `rayls-sovereign-relayer` sources (on `main`), which the published ECR images predate, so pulled-image inits keep the hub until the images catch up. Every `--local` init records its build sources in the stack `.env` — the local sibling checkouts (`../rayls-sovereign-contracts`, `../rayls-sovereign-relayer`, recorded as relative paths) via `CONTRACTS_SRC`/`RELAYER_SRC` when present (whatever branch is checked out there is what builds), else the `main` git contexts via `CONTRACTS_REF`/`RELAYER_REF`. `--full` always includes the full hub.
*   `--members <int>`: Number of privacy node participants. With `--full`: 2–6 (default **2**). For the hub-less default topology: 1–6 (default **1**) — the nodes intercommunicate via the public chain, so any count is meaningful. Ignored on hub-carrying lean stacks (`--with-hub` runs a single participant; use `--full` for the multi-participant hub).
*   `--public-chain <preset>`: Public chain preset to bridge to — `local` (an Axyl public chain running **inside the stack**: service `public-chain`, RPC `localhost:8845`, chain id `7331`, deployer genesis-funded, no external connectivity) or `rayls-testnet` (the external testnet). Applied by default for the default (lean) stack — in the hub-less default the public chain is the privacy nodes' only interconnection path: **`local` with `--local`, `rayls-testnet` otherwise**. `--full --local` also defaults to `local` (the 3.0.1 source deploy requires a public chain); only `--full` with pulled images runs without one. Adds per-participant `pubrelayer` services.
*   `--privacy-node-only`: Run just a single Axyl privacy node, with no bridge or surrounding services. Ignores all other flags.
*   `--monitoring`: Enable the observability stack (Grafana, Loki, Prometheus, Tempo) with eBPF auto-instrumentation. Default: off.
*   `--blockscout <list>`: Comma-separated participant letters that should get a Blockscout explorer (e.g. `a,b`). Defaults to **every participant**; use this to narrow the set.
*   `--no-blockscout`: Disable the per-node Blockscout explorers entirely (overrides `--blockscout`).
*   `--local`: Dev mode. Build the Rayls app components (kos/CTS, pubrelayer, private relayer, contracts — plus governance, proofs-api and audit-explorer in hub topologies) from source (short names, `pull_policy=build`/`never`) instead of pulling them from ECR. Also defaults the topology to **hub-less** and `--public-chain` to `local`, so a `--local` init runs **everything on your machine with no hub** — pair with `--public-chain rayls-testnet` to keep bridging to the testnet instead. The infra images (NATS, the Private Network Hub, Postgres, nginx, Blockscout) still come from their registries and are pre-pulled one at a time, same as the published stack.
*   `--no-pull`: Skip the image-pull step; `up` then fetches only the images missing locally, one at a time. Use to keep a locally-built image (e.g. a custom node/contracts build) instead of overwriting it from ECR.
*   `--lean`: **Deprecated** — the lean privacy-node → public-chain bridge is now the default, so this flag is a no-op. Use `--full` for the multi-participant stack.

Example:
```bash
./rayls init                                  # default: 1 Axyl node -> rayls-testnet + explorer (http://localhost:10004)
./rayls init --local                          # fully local: source builds + local Axyl public chain
./rayls init --local --public-chain rayls-testnet  # source builds, but bridge to the testnet
./rayls init --local --members 3              # 3 hub-less privacy nodes, fully local
./rayls init --local --with-hub               # keep the minimal PNH (lean hub stack)
./rayls init --full --members 3 --monitoring  # full 3-participant demo stack + monitoring
```

**Note:** The first `init` may take several minutes as Docker images are pulled sequentially from the registry. Subsequent runs will be faster as images are cached locally.

### Modes

The `init` flags compose into several distinct stack flavors:

#### 1. Default — local privacy node → public chain

A **minimal privacy-node → public-chain bridge** for a single participant (`a`): the Axyl privacy node plus the services needed to bridge to an external public chain, with **no** private relayer, gnark/proofs API, or governance. This is what a bare `rayls init` does.

```bash
./rayls init                               # bridge to rayls-testnet (default) + privacy-chain explorer
./rayls init --public-chain rayls-testnet  # same; --public-chain overrides the target
```

Hub-less (`--local`, the default topology) the stack is 7 core services: `postgres` (shared Postgres — backs the pubrelayer + KOS databases), `nats`, `privacy-node-a`, `public-chain` (the in-stack Axyl public chain), `contracts`, `kos-a`, `pubrelayer-a` — plus the per-node Blockscout explorer services (default-on; `--no-blockscout` drops them). KOS is kept because the pubrelayer fetches its signing keys from it. With `--with-hub` (and on pulled-image inits, until the published images support hub-less) the stack additionally runs `private-network-hub` (the Besu commit chain), `relayer-a` (the private relayer, PN↔PNH message relaying) and `proofs-api` (Enygma proofs).

Bridging to the testnet deploys from **your own funded key**: `rayls init` prompts for it and saves it to the stack `.env` (see [Funding](#funding)). Non-interactive/CI runs pass it via the environment instead:

```bash
PUBLIC_CHAIN_PRIVATE_KEY=<hex> ./rayls init --public-chain rayls-testnet   # 0x prefix optional
```

Once the stack is healthy, bridge a token end-to-end (see [Verifying the bridge](#verifying-the-bridge)).

> This mode uses the dedicated `rayls-contracts:lean-no-pnh` contracts image (built from the sibling `rayls-privacy-contracts` repo, branch `cli-lean-no-pnh`) and the `rayls-privacy-axyl` node image. Both are published to ECR, so the default `init` works out of the box.

#### 2. Full demo stack (`--full`)

The full Rayls stack with N participants, a **local Besu commit chain** (`private-network-hub`), proofs API, and governance services. Bridging happens between the privacy nodes and the local commit chain, plus a public chain: with `--local` the in-stack `local` preset is included by default (the 3.0.1 source deploy requires a public chain — its PN deploy ABI-encodes `PUBLIC_CHAIN_ID`); with pulled images the public chain stays optional (the 3.0.0-era deploy supports the commit-chain-only demo).

```bash
./rayls init --full                              # 2 participants, local commit chain (pulled images)
./rayls init --full --members 4                  # 4 participants
./rayls init --full --local                      # source builds + in-stack local public chain
./rayls init --full --public-chain rayls-testnet # full stack + external public chain
```

The deployer for the local Besu / privacy nodes can be overridden:

```bash
PRIVATE_KEY_SYSTEM=<0x-hex> ./rayls init --full
```

#### 3. Hub-less (the default topology)

The default topology runs the environment **without the Private Network Hub**, mirroring `start_dev.sh --no-hub` in the `rayls-sovereign-relayer` repo: no `private-network-hub` (Besu), no private relayers, no proofs API, no governance and no audit explorer (those belong to the hub topologies — use `--with-hub` or `--full` for them). The privacy nodes intercommunicate through the **public chain only**, so a public chain is always configured — with `--local` it defaults to the `local` preset (an Axyl public chain inside the stack), making the whole system **fully self-contained on one host**; pass `--public-chain rayls-testnet` to bridge to the external testnet instead. The contracts deploy runs with `HUB_ENABLED=false`, writes no `PNH_*` values into the per-participant env files, and the CTS detects hub-less mode from their absence.

```bash
./rayls init --local                                 # single PN <-> local public chain, fully isolated
./rayls init --local --members 3                     # N PNs interconnected via the local public chain
./rayls init --local --public-chain rayls-testnet    # hub-less, bridged to the testnet
```

> Hub-less needs the `HUB_ENABLED`-aware contracts deploy and the hub-less-capable CTS, which ship in the 3.0.1 `rayls-sovereign-contracts` / `rayls-sovereign-relayer` repos (on `main`). The published ECR images predate that, so hub-less currently applies to `--local` inits only (pulled-image inits keep the minimal hub); the CLI records the build sources in the stack `.env` — preferring the local sibling checkouts (`CONTRACTS_SRC`/`RELAYER_SRC`, so in-flight hub-less branches build as checked out) and falling back to the `main` git contexts (`CONTRACTS_REF`/`RELAYER_REF`). Override either in `.env` if needed.

#### 4. Privacy node only

Minimal stack: a single Axyl `privacy-node-a`, with no bridge or surrounding services. The EVM JSON-RPC is exposed on `127.0.0.1:8545` (gasless, chain id `12345`). No contracts, relayer, KOS, governance, NATS, postgres, commit chain, or proofs API — Axyl self-stores in its datadir.

```bash
./rayls init --privacy-node-only
```

Useful when external tooling only needs an EVM RPC endpoint. All other flags are ignored. The `verify` command is unavailable in this mode (no contracts container).

#### 5. With monitoring

Adds Grafana (`:3300`), Loki, Prometheus, Tempo, and eBPF auto-instrumentation. Combines with the default or `--full` modes.

```bash
./rayls init --monitoring
./rayls init --full --members 3 --monitoring
```

#### 6. With Blockscout explorers

Runs a Blockscout backend + frontend per participant (proxied behind nginx), **by default in every mode**. `--blockscout <list>` narrows the set; `--no-blockscout` disables them.

```bash
./rayls init                               # explorer for the default single node -> http://localhost:10004
./rayls init --full --blockscout a,b      # only participants a and b get explorers
./rayls init --no-blockscout               # no explorers
```

#### 7. Local builds (dev mode)

Builds the Rayls app components from source (pinned git refs, or your checkouts via `rayls dev`) instead of pulling their ECR images; infra images are still pulled. Combines with any of the above. Useful when iterating on Rayls source out-of-tree.

```bash
./rayls init --local
./rayls init --full --local
./rayls init --local --privacy-node-only
```

### Verifying the bridge

Once the default (or `--public-chain`) stack is healthy, bridge a token end-to-end:

```bash
./rayls verify public-chain
```

This creates a user, deploys a fresh `DEMO_*` ERC-20 on the privacy node, waits for the pubrelayer to deploy the mirror token on the public chain, bridges 100 DEMO, and confirms the destination balance. It prints the privacy-node token, the public-chain token, the recipient, and the public chain id; verify the public-chain token on the explorer (https://testnet-explorer.rayls.com/).

If a step (usually relayer authorization on the public chain) hasn't propagated yet, a clean `./rayls down -v` and re-run typically clears it.

### Building the Axyl node image (dev)

The `rayls-privacy-axyl` image is a multi-arch image published to ECR. To build it yourself from the sibling `axyl` repo (its dev single-node mode is behind a Cargo feature):

```bash
docker build -f etc/docker-network/Dockerfile \
  --build-arg BUILD_FEATURES=dev-single-node-setup \
  --build-arg VERGEN_GIT_SHA=$(git rev-parse HEAD) \
  -t public.ecr.aws/w0k9o1t3/rayls-demo/rayls-privacy-axyl:latest .
```

`VERGEN_GIT_SHA` must be set (reth's build script slices a short SHA and panics on an empty value). Run the CLI with `--no-pull` (or `--local`) to use a locally-built image instead of pulling from ECR.

### Managing the Stack

*   **Start the environment:**
    ```bash
    ./rayls start              # Start all services
    ./rayls start privacy-node-a kos-a   # Start specific services
    ```
*   **Stop the environment:**
    Pauses running containers without removing them.
    ```bash
    ./rayls stop               # Stop all services
    ./rayls stop pl-b          # Stop specific services
    ```
*   **Tear down the environment:**
    Stops and removes containers and networks. By default, volumes are preserved (data is kept).
    ```bash
    ./rayls down               # Remove containers/networks, keep volumes (data preserved)
    ./rayls down -v            # Remove containers/networks AND volumes (destructive)
    ./rayls down --remove-orphans  # Also remove orphaned containers
    ```

### Monitoring & Inspection

*   **Check status:**
    View the running status of all Rayls services.
    ```bash
    ./rayls ps                 # Show all services
    ./rayls ps -a              # Show all containers (including stopped)
    ```
*   **View logs:**
    Stream logs from the services.
    ```bash
    ./rayls logs               # Show logs from all services
    ./rayls logs privacy-node-a kos-a -f  # Follow logs from specific services
    ./rayls logs contracts --tail=100  # Show last 100 lines
    ./rayls logs -f -t         # Follow all logs with timestamps
    ```
*   **System Info:**
    Display information about Docker and Docker Compose versions.
    ```bash
    ./rayls info
    ./rayls stats              # Show Docker daemon statistics
    ```

### Version & Updates

*   **Check CLI version:**
    Display the current version and build information.
    ```bash
    ./rayls version            # Show version info
    ./rayls version --check    # Check for available updates
    ```
*   **Check for updates:**
    Check if a newer version is available and get installation instructions.
    ```bash
    ./rayls update check       # Check for updates
    ```

    The CLI caches update checks for 24 hours to avoid excessive network requests. Update information is stored in `~/.rayls/update-check.json`.

### Access Endpoints

After successful initialization, the exposed services depend on the mode.

**Default (local privacy node → public chain):**
*   **Privacy Node RPC:** `http://localhost:8545` — Axyl EVM JSON-RPC (chain id `12345`, gasless)
*   **Private Network Hub:** `http://localhost:3445` — the minimal commit chain (only with `--with-hub` or pulled-image inits; hub-less stacks have none)
*   **Pubrelayer:** `http://localhost:9050` — bridges to the public chain
*   **KOS (Key Orchestration):** `localhost:8080` — the CTS gRPC endpoint (mTLS, not plain HTTP)
*   **Blockscout explorer:** `http://localhost:10004` — privacy-chain explorer for node `a` (default-on; per-node at `10004 + 100·i`)

**Full stack (`--full`) — additional shared services:**
*   **Block Explorer:** `http://localhost:8181` - View blockchain transactions and blocks
*   **Private Network Hub:** `http://localhost:3445` - Commit chain coordination
*   **Governance API:** `http://localhost:9100` - Governance management
*   **Grafana (if `--monitoring` enabled):** `http://localhost:3300` - Observability dashboards

For each participant (A, B, C, …) in `--full`, services are exposed on incrementing ports:

| Service | Participant A | Participant B | Participant C |
|---------|--------------|--------------|--------------|
| Privacy Node (PL) | `http://localhost:8545` | `http://localhost:8546` | `http://localhost:8547` |
| Relayer | `http://localhost:9000` | `http://localhost:9001` | `http://localhost:9002` |
| KOS (Key Orchestration) | `http://localhost:8080` | `http://localhost:8081` | `http://localhost:8082` |

*Pattern continues for participants D, E, F with incrementing port numbers.*

## Local vs. remote images — where every image comes from

A bare `rayls init` pulls the published `rayls-demo` images from ECR. `rayls init --local` instead builds the Rayls **app** components from source (see [Working on a component](#working-on-a-component-hot-reload)); infra images are still pulled.

| Image group | default `rayls init` (remote) | `rayls init --local` (from source) |
|---|---|---|
| kos, pubrelayer, private relayer, contracts | published ECR `rayls-demo` images | **built by Docker** from the pinned git refs (the `rayls-sovereign-*` repos on `main`), or your checkouts via `rayls dev` |
| `--full` extras: governance-api/listener/flagger, proofs-api (gnark), audit-explorer | ECR | **built by Docker** from their `rayls-sovereign-*` repos on `main` (see note on gnark's Git-LFS keys below) |
| Axyl privacy node | pulled from ECR | pulled from ECR + retagged (a local `rayls-privacy-axyl:latest` you built yourself is left alone) |
| nats, Private Network Hub (Besu) | ECR | ECR (pulled — infra images aren't source-built) |
| Third-party (postgres, blockscout, nginx, grafana) | public registries | public registries |

The published `rayls-demo` service images are **multi-arch (amd64 + arm64)**, so remote `rayls init` runs natively on both Intel/AMD and Apple Silicon. Use `--local` when you want to build from a specific ref or hack on a component — not as an architecture workaround.

> ⚠️ **gnark (`proofs-api`) uses Git-LFS.** Its proving keys are Git-LFS blobs, which a pinned **git-context** build can't fetch, so the default `--local` build of `proofs-api` would ship pointer files. For a working Enygma stack, run `rayls dev gnark` (clones the repo and runs `git lfs pull`) or use the pulled ECR image. The hub-less default stack never builds `proofs-api`, so this only matters for `--with-hub` / `--full`.
>
> The `rayls-sovereign-ops-api` service (the former backend) is intentionally **not** wired into the CLI.

## Choosing a version

In `--local` mode every app component builds from a pinned git ref. By default that ref is **`main`** of each component's public `rayls-sovereign-*` repo — the 3.0.1 code, which the repos currently carry as a single `main` branch (no version tags yet). Once the repos tag coordinated releases, one variable in `.env` (or the environment) selects the whole set:

```bash
RAYLS_VERSION=3.0.2        # all components build from tag v3.0.2 (once such tags exist)
```

Any single component can deviate — point it at a tag, branch, or your own fork:

```bash
RELAYER_REF=fix/reorg-handling                                   # branch on the canonical repo
CONTRACTS_REF=main                                               # a specific ref
RELAYER_REPO=git@github.com:you/rayls-sovereign-relayer.git      # your fork
CONTRACTS_SRC=../rayls-sovereign-contracts                       # a local checkout (what `rayls dev` sets)
```

Re-run `rayls init --local` after changing `RAYLS_VERSION` / `*_REF` / `*_REPO` (they're baked into the generated compose file); `*_SRC` changes apply on the next `docker compose up` without regenerating.

### Configuration reference (`.env`)

The stack directory's `.env` file drives both the CLI and compose; the process environment always wins over the file. See [.env.example](.env.example) for a commented template. The component prefixes are `CONTRACTS`, `RELAYER` (kos + pubrelayer + private relayer), `GOVERNANCE`, `GNARK`, and `AUDITOR`.

| Variable | Purpose | Applies |
|---|---|---|
| `PUBLIC_CHAIN_PRIVATE_KEY` | Your funded testnet deployer key (64 hex chars, `0x` optional); required for `rayls-testnet` stacks, prompted by `rayls init` and saved here (see [Funding](#funding)) | next `up` |
| `RAYLS_VERSION` | Coordinated release: all components build from tag `v<version>` | re-run `init --local` |
| `<COMPONENT>_REF` | Per-component branch/tag/SHA; wins over `RAYLS_VERSION` | re-run `init --local` |
| `<COMPONENT>_REPO` | Per-component git URL (e.g. a fork); cloned by BuildKit, so no ssh-config aliases | re-run `init --local` |
| `<COMPONENT>_SRC` | Local checkout path; replaces the git context (managed by `rayls dev`) | next `up` / rebuild |
| `RAYLS_SRC_DIR` | Where `rayls dev` looks for / clones checkouts (default: parent of the stack dir) | `rayls dev` |
| `RAYLS_AXYL_IMAGE` | Image to pull + retag as `rayls-privacy-axyl:latest` in `--local` | next `init --local` |

> **Default refs** (until the component repos tag coordinated `v<X.Y.Z>` releases in lockstep): every component defaults to `main` — the `rayls-sovereign-*` repos' 3.0.1 code. Set `RAYLS_VERSION` or a per-component `*_REF` to override.

## Working on a component (hot reload)

`rayls dev <component>` switches a component from its pinned build to a local checkout you can edit, with hot reload via `rayls watch`. See **[DEV_MODE.md](DEV_MODE.md)** for the full guide. In short:

```bash
./rayls dev relayer        # clone (if needed) + switch relayer to a local checkout you edit
./rayls watch              # sync saves into the container; air rebuilds & restarts in ~seconds
./rayls dev --status       # what's in dev mode?
./rayls dev --off relayer  # back to the pinned build
```

| Component | Repo | Services | Hot reload |
| --- | --- | --- | --- |
| `relayer` | rayls-sovereign-relayer | `kos-*`, `pubrelayer-*`, `relayer-*` | yes |
| `governance` | rayls-sovereign-pnh-governance | `governance-api`, `governance-listener`, `governance-flagger` | no — build from checkout, rebuild to apply (`--full` only) |
| `gnark` | rayls-sovereign-gnark-api | `proofs-api` | no — Git-LFS keys; use `rayls dev gnark` (LFS-aware) or the pulled image |
| `auditor` | rayls-sovereign-pnh-auditor-ui | `audit-explorer` | no — Angular→nginx build from checkout (`--full` only) |
| `contracts` | rayls-sovereign-contracts | `contracts` | no — builds from your checkout, but deploys are explicit: re-run `rayls init --local` (or restart the contracts service) after contract changes |
| node | axyl | `privacy-node-*` | no — pulled as a published image; build it yourself for node work |

## Architecture

### Privacy node (Axyl)

Each privacy node is an **Axyl (`rayls-network`) node** running in its single-validator dev mode. It exposes an Ethereum-compatible JSON-RPC (`eth_*`, `net_*`, `web3_*`, `debug_*`, `trace_*`) on `8545+i`, keeps chain state in its own datadir (a named volume, no external database), and runs the `local` hardfork profile so it is **gasless** (EIP-1559 active from block 0 with a 0 base-fee floor). The genesis is generated externally at startup via a one-shot init container (keytool + genesis ceremony), keeping the node image config-free.

### Full demo stack (`--full`)

When running with `--full`, the CLI provisions the complete Rayls stack:

**Infrastructure Layer**
*   **Postgres** - Relational database for relayer/KOS/governance data
*   **Commit Chain** - Besu-based blockchain for cross-ledger coordination
*   **OpenTelemetry Stack** - Optional (when `--monitoring` is enabled): eBPF auto-instrumentation, Grafana, Loki, Tempo, Prometheus

**Contract Deployment Layer**
*   **Proofs API** - Zero-knowledge proof generation service
*   **Contracts** - Smart contract compilation and deployment service (deploys to all privacy nodes and the commit chain)

**Governance Layer**
*   **Governance API / Listener / Flagger** - governance operations, event listening, and feature flags

**Per-Participant Services**
For each participant (A, B, C, …):
*   **Privacy Node (PL)** - Axyl private blockchain node (e.g. `privacy-node-a`, aliases `pl-a`/`pn-a`)
*   **KOS (Key Orchestration Service)** - Cryptographic key management (e.g. `kos-a`)
*   **Relayer** - Cross-chain transaction relay service (e.g. `relayer-a`)

A single shared **NATS** instance is deployed for all participants and the governance services.

### Service Dependencies

The stack uses health checks and dependency ordering to ensure services start in the correct sequence:

1. Infrastructure services (Postgres, NATS) start first
2. Privacy nodes (and, in `--full`, the commit chain) start after their init/infrastructure is healthy
3. Contract deployment waits for the privacy nodes (and, in `--full`, the proofs API and commit chain)
4. Application services (KOS, relayers, pubrelayer) wait for contracts to be deployed
5. Governance services (in `--full`) start after contracts and databases are ready

This orchestration ensures that all required dependencies are available before dependent services attempt to start.

## Troubleshooting

*   **`invalid empty ssh agent socket` during a `--local` build:** the git build contexts are private, so BuildKit needs your ssh-agent — make sure `SSH_AUTH_SOCK` is set and your GitHub key is loaded (`ssh-add -l`).
*   **BuildKit can't resolve a custom SSH host alias:** git contexts are cloned by the Docker daemon, which doesn't read your `~/.ssh/config`. Use the plain `github.com` host in `*_REPO` URLs and select the right key via your agent, or fall back to a local checkout with `rayls dev`.
*   **`docker-compose.override.yaml exists but was not generated by rayls dev`:** you have a hand-written override; move it aside (its job is likely covered by `rayls dev` now).

## Contributing

We are not accepting external contributions at this time — see [CONTRIBUTING.md](./CONTRIBUTING.md). Please also read our [Code of Conduct](./CODE_OF_CONDUCT.md).

## Security

To report a security vulnerability, see [SECURITY.md](./SECURITY.md) — please do not open a public issue.

## Disclaimer

⚠️ **Experimental / Development Use Only**

This CLI tool is currently designed for **local development and demonstration purposes**. It deploys a simplified version of the Rayls stack on a single host. It is **not** intended for production deployments or multi-host environments at this stage.

## License

Licensed under the Apache License, Version 2.0 — see [LICENSE](./LICENSE).

Copyright 2026 Rayls Core Ltd.

[license-badge]: https://img.shields.io/badge/License-Apache_2.0-blue.svg
[license-url]: ./LICENSE
[go-badge]: https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white
[go-url]: https://go.dev
[discord-badge]: https://img.shields.io/badge/Discord-join%20chat-5865F2?logo=discord&logoColor=white
[discord-url]: https://discord.gg/6THZ96357r
[x-badge]: https://img.shields.io/badge/X-%40RaylsLabs-000000?logo=x&logoColor=white
[x-url]: https://x.com/RaylsLabs
[linkedin-badge]: https://img.shields.io/badge/LinkedIn-Rayls-0A66C2?logo=linkedin&logoColor=white
[linkedin-url]: https://www.linkedin.com/company/rayls/
[youtube-badge]: https://img.shields.io/badge/YouTube-Rayls-FF0000?logo=youtube&logoColor=white
[youtube-url]: https://www.youtube.com/@Rayls_blockchain
