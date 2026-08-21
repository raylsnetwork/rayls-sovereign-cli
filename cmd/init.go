package cmd

import (
	"os"
	"strings"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/stacks"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var membersCount int
var monitoringEnabled bool
var blockscoutNodes string
var noBlockscout bool
var localImages bool
var publicChainPreset string
var privacyNodeOnly bool
var leanPublic bool // deprecated: the lean bridge is now the default
var fullStack bool
var withHub bool
var noPull bool

// defaultPublicChain is the preset a bare `rayls init` (pulled images)
// bridges to. The primary use case is a single local Axyl privacy node
// connected to a public chain. `--local` inits default to the `local` preset
// instead — a public chain running inside the stack — so a source-built
// system is fully self-contained.
const defaultPublicChain = "rayls-testnet"

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Spin up a local Axyl privacy node bridged to a public chain",
	Long: `Initialize a Rayls environment.

By default this spins up Axyl privacy node(s) bridged to a public chain — the
primary use case. Hub-less is the default topology: nodes intercommunicate via
the public chain only. --with-hub adds the minimal Private Network Hub;
--full brings the complete hub demo stack.

  rayls init                          # published images -> rayls-testnet
                                      # (keeps the minimal hub until the
                                      # published images support hub-less)
  rayls init --local                  # fully local hub-less system: source
                                      # builds + a local Axyl public chain
  rayls init --local --members 3      # 3 hub-less privacy nodes, all local
  rayls init --with-hub               # keep the minimal PNH explicitly
  rayls init --public-chain <preset>  # bridge to 'local' or 'rayls-testnet'
  rayls init --privacy-node-only      # just the Axyl node, no bridge
  rayls init --full [--members N]     # full multi-participant demo stack
                                      # (Private Network Hub, governance, ...)`,
	Run: func(cmd *cobra.Command, args []string) {
		red := color.New(color.FgRed)
		yellow := color.New(color.FgYellow)

		if privacyNodeOnly {
			if err := stacks.InitPrivacyNodeOnly(localImages); err != nil {
				red.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// Default is the lean privacy-node -> public-chain bridge; --full brings
		// up the demoted multi-participant PNH/governance stack. (--lean is now
		// the default and is kept only as a deprecated no-op.)
		lean := !fullStack

		// Hub-less is the DEFAULT topology: the privacy nodes intercommunicate
		// via the public chain only. --with-hub opts the lean stack into the
		// minimal Private Network Hub; --full always brings the complete hub
		// stack. Hub-less needs the hub-less-capable CTS (the rayls-sovereign-*
		// 3.0.1 sources on main), which the published ECR images predate — the
		// 3.0.0 CTS's ParticipantRegistrar calls
		// ParticipantStorageV1.getChainViewData on its hub at startup, a
		// function the PN-side replica doesn't implement, so it cannot run
		// without a PNH even with the :lean-no-pnh deploy's PNH_ENABLED=false
		// registry aliasing (verified empirically 2026-08-20). Pulled-image
		// stacks therefore keep the minimal hub until the images are
		// republished; --local stacks (3.0.1 source builds) run hub-less.
		noHub := !fullStack && !withHub && localImages

		// Resolve the public chain target.
		//   - default (lean): always bridges; --public-chain overrides.
		//   - --full --local: defaults to the in-stack local public chain — the
		//     3.0.1 source deploy cannot run PC-less (the PN deploy ABI-encodes
		//     PUBLIC_CHAIN_ID into RNEndpointV1.initialize and aborts with
		//     "PUBLIC_CHAIN_ID not set" without one), and upstream's local dev
		//     stack always carries the in-stack public chain (7331).
		//   - --full with pulled images: public chain stays optional (the
		//     3.0.0-era :latest deploy supports the local commit-chain demo).
		// Which chain the lean default resolves to depends on --local:
		// source-built stacks default to the fully local Axyl public chain (an
		// isolated system: no external connectivity, no user-funded testnet
		// key needed); pulled-image stacks keep bridging to rayls-testnet.
		presetName := publicChainPreset
		if presetName == "" {
			switch {
			case lean:
				if localImages {
					presetName = "local"
				} else {
					presetName = defaultPublicChain
				}
			case fullStack && localImages:
				presetName = "local"
				yellow.Println("Note: --full with --local includes the in-stack local public chain (the 3.0.1 source deploy requires one; use --public-chain to bridge elsewhere).")
			}
		}
		var publicChain *docker.PublicChain
		if presetName != "" {
			preset, ok := docker.PublicChainPresets[presetName]
			if !ok {
				red.Printf("Error: unknown --public-chain preset %q. Available: ", presetName)
				for name := range docker.PublicChainPresets {
					red.Printf("%s ", name)
				}
				red.Println()
				os.Exit(1)
			}
			publicChain = &preset
		}

		// --members scales the participant count. --full requires 2-6; the
		// hub-less default accepts 1-6 (nodes intercommunicate via the public
		// chain, so any count is a meaningful topology). The lean WITH-HUB stack
		// stays single-participant by design: it is the one-institution hub
		// experience (PN <-> PNH relaying, Enygma) — the multi-participant hub
		// demo with governance is what --full provides.
		members := 1
		if fullStack {
			if membersCount < 2 || membersCount > 6 {
				red.Println("Error: --members must be between 2 and 6")
				os.Exit(1)
			}
			members = membersCount
		} else if cmd.Flags().Changed("members") {
			switch {
			case !noHub:
				yellow.Println("Note: --members is ignored on hub-carrying lean stacks (single privacy node). Use --full for the multi-participant hub stack, or drop --with-hub / add --local for hub-less multi-participant.")
			case membersCount < 1 || membersCount > 6:
				red.Println("Error: --members must be between 1 and 6")
				os.Exit(1)
			default:
				members = membersCount
			}
		}

		participants := []string{"a", "b", "c", "d", "e", "f"}[:members]

		// Blockscout: default-on for every participant in every mode (a bare
		// init gets an explorer for node a). --blockscout narrows the set,
		// --no-blockscout disables it entirely.
		var blockscout []string
		switch {
		case noBlockscout:
			if cmd.Flags().Changed("blockscout") {
				yellow.Println("Note: --no-blockscout overrides --blockscout; no Blockscout services will run.")
			}
		case cmd.Flags().Changed("blockscout"):
			blockscout = parseBlockscoutNodes(blockscoutNodes, participants)
		default:
			blockscout = participants
		}

		if err := stacks.InitStack(participants, monitoringEnabled, blockscout, localImages, publicChain, lean, noHub, noPull); err != nil {
			red.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&fullStack, "full", false, "Bring up the full multi-participant demo stack (Private Network Hub, governance, multiple privacy nodes). Combine with --members and/or --public-chain.")
	initCmd.Flags().BoolVar(&withHub, "with-hub", false, "Include the Private Network Hub (PNH) in the default (lean) stack: PNH plus the private relayer and proofs-api (PN<->PNH messaging, Enygma). Hub-less is the default topology for --local stacks; pulled-image stacks keep the hub regardless (their 3.0.0 CTS requires one) until the published images support hub-less. --full always includes the full hub (plus governance).")
	initCmd.Flags().IntVar(&membersCount, "members", 2, "Number of privacy node participants: 2-6 with --full, 1-6 for the hub-less default (default 1 there). Ignored on hub-carrying lean stacks.")
	initCmd.Flags().StringVar(&publicChainPreset, "public-chain", "", "Public chain preset to bridge to: 'local' (an Axyl public chain inside the stack) or 'rayls-testnet'. Defaults: lean stacks get 'local' with --local and 'rayls-testnet' otherwise; --full --local gets 'local' (the 3.0.1 source deploy requires a public chain). Only --full with pulled images runs without one.")
	initCmd.Flags().BoolVar(&privacyNodeOnly, "privacy-node-only", false, "Run just a single Axyl privacy node, with no bridge or surrounding services. Ignores other flags.")
	initCmd.Flags().BoolVar(&monitoringEnabled, "monitoring", false, "Enable monitoring stack (Grafana, Loki, Tempo, Prometheus)")
	initCmd.Flags().StringVar(&blockscoutNodes, "blockscout", "", "Comma-separated list of nodes to enable Blockscout for (e.g. 'a,b,c'). Defaults to every participant; use this to narrow the set, or --no-blockscout to disable.")
	initCmd.Flags().BoolVar(&noBlockscout, "no-blockscout", false, "Disable the per-node Blockscout explorers (they run for every participant by default).")
	initCmd.Flags().BoolVar(&localImages, "local", false, "Dev mode: build Rayls components (kos/CTS, pubrelayer, contracts) from source inside Docker — pinned git contexts by default, local checkouts via `rayls dev` — instead of pulling images from ECR. Also defaults --public-chain to 'local', making the stack fully self-contained.")
	initCmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip the image pull step; `up` fetches only missing images. Use to keep a locally-built image (e.g. a custom contracts build) instead of overwriting it from ECR.")
	// --lean is now the default behavior; keep the flag as a deprecated no-op so
	// existing invocations don't break. cobra hides deprecated flags from help
	// and prints the message below when one is used.
	initCmd.Flags().BoolVar(&leanPublic, "lean", false, "Deprecated: the lean privacy-node -> public-chain bridge is now the default.")
	initCmd.Flags().MarkDeprecated("lean", "it is now the default (use --full for the multi-participant stack)")
}

// parseBlockscoutNodes parses the comma-separated blockscout flag and validates nodes
func parseBlockscoutNodes(nodesList string, participants []string) []string {
	// Empty string means no blockscout
	if nodesList == "" {
		return []string{}
	}

	// Create participant set for validation
	participantSet := make(map[string]bool)
	for _, p := range participants {
		participantSet[p] = true
	}

	// Parse and validate
	nodes := strings.Split(nodesList, ",")
	validNodes := make([]string, 0)

	for _, node := range nodes {
		node = strings.ToLower(strings.TrimSpace(node))
		if node == "" {
			continue
		}

		// Validate node is in participants
		if !participantSet[node] {
			red := color.New(color.FgRed)
			red.Printf("Warning: Blockscout node '%s' is not in participants list, skipping\n", node)
			continue
		}

		validNodes = append(validNodes, node)
	}

	return validNodes
}
