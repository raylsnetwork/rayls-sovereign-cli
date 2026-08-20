package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/stacks"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// verifyCmd represents the verify command
var verifyCmd = &cobra.Command{
	Use:   "verify [command]...",
	Short: "Verify the environment or execute a command in the contracts container",
	Long: `Execute a command inside the running contracts container.

Subcommands:
  contracts - Run the default E2E test suite
  time <participant> - Check current blockchain time for a privacy node
  public-chain - Bridge 100 units of a fresh DEMO token from PN A to the public chain
                 and verify the balance arrives.

Examples:
  rayls verify contracts
  rayls verify time a
  rayls verify public-chain
  rayls verify ls -la`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		red := color.New(color.FgRed)
		var commandToRun []string

		switch args[0] {
		case "public-chain":
			return runVerifyPublicChain()

		case "contracts":
			// Run the forge suite — the maintained test suite in the
			// contracts repo (the old hardhat e2e/enygma path only ever
			// existed in a dirty worktree and is absent from the 3.0.0
			// image). The full-stack E2E is `rayls verify public-chain`.
			commandToRun = []string{"forge", "test"}

		case "get-contracts":
			// Check blockchain time: verify time <participant>
			if len(args) < 2 {
				red.Println("Error: 'time' subcommand requires a participant identifier (a, b, c, d, e, or f)")
				return fmt.Errorf("missing participant argument")
			}

			participant := strings.ToLower(strings.TrimSpace(args[1]))

			// Validate participant identifier
			validParticipants := []string{"a", "b", "c", "d", "e", "f"}
			isValid := false
			for _, valid := range validParticipants {
				if participant == valid {
					isValid = true
					break
				}
			}

			if !isValid {
				red.Println("Error: participant must be one of: a, b, c, d, e, f")
				return fmt.Errorf("invalid participant identifier: %s", participant)
			}

			participantUpper := strings.ToUpper(participant)
			commandToRun = []string{"npx", "hardhat", "deployment-proxy:get-all-contracts", "--node", participantUpper}

		// default:
		// 	// Pass through any other command
		// 	commandToRun = args
		// }

		//return stacks.ExecContracts(commandToRun)

		case "time":
			// Check blockchain time: verify time <participant>
			if len(args) < 2 {
				red.Println("Error: 'time' subcommand requires a participant identifier (a, b, c, d, e, or f)")
				return fmt.Errorf("missing participant argument")
			}

			participant := strings.ToLower(strings.TrimSpace(args[1]))

			// Validate participant identifier
			validParticipants := []string{"a", "b", "c", "d", "e", "f"}
			isValid := false
			for _, valid := range validParticipants {
				if participant == valid {
					isValid = true
					break
				}
			}

			if !isValid {
				red.Println("Error: participant must be one of: a, b, c, d, e, f")
				return fmt.Errorf("invalid participant identifier: %s", participant)
			}

			participantUpper := strings.ToUpper(participant)
			commandToRun = []string{
				"npx",
				"hardhat",
				"utils:check-blockchain-time",
				"--pl",
				participantUpper,
			}

		default:
			// Pass through any other command
			commandToRun = args
		}

		return stacks.ExecContracts(commandToRun)
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

// The demo deployer the rest of the stack already trusts: the publicly known
// Anvil dev account #0, genesis-funded on the in-stack chains and
// pre-registered by the deploy. Using it (rather than a fresh wallet) skips
// funding and registration before bridging.
const (
	verifyDeployerPrivateKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	verifyDeployerAddress    = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

	// 100 tokens (18 decimals): a human-readable number on explorers. Minted to
	// the deployer before the bridge (the 3.0.1 ProductionErc20Token mints
	// nothing at deploy, unlike the legacy TokenExample's 2M pre-mint, and the
	// bridge send locks this amount on the PN side).
	verifyBridgeAmountWei = "100000000000000000000"
)

var (
	tokenAddrRegex   = regexp.MustCompile(`Token Deployed At Address (0x[0-9a-fA-F]{40})`)
	publicTokenRegex = regexp.MustCompile(`(?:Found public token address|Public Token Address): (0x[0-9a-fA-F]{40})`)
	rawBalanceRegex  = regexp.MustCompile(`Raw Balance:\s*(\d+)`)
)

// contractsTaskFailed folds the two failure channels of a hardhat task run
// into one error. The 3.0.1 token/user tasks catch their own exceptions and
// `return {success:false}` — the process then EXITS 0 — printing a
// moca-logger "❌ ..." line to stdout instead, so trusting the exit code
// alone makes on-chain/config failures invisible until some later step
// times out and takes the blame. A non-zero exit still wins (HH errors,
// compile failures, docker exec problems).
func contractsTaskFailed(out string, err error) error {
	if err != nil {
		return err
	}
	if idx := strings.Index(out, "❌"); idx >= 0 {
		tail := strings.TrimSpace(out[idx:])
		if len(tail) > 400 {
			tail = tail[:400] + "…"
		}
		return fmt.Errorf("task reported failure: %s", tail)
	}
	return nil
}

// outputTail returns the last few non-empty lines of a task's captured
// output, for surfacing swallowed evidence without dumping whole traces.
func outputTail(out string, n int) string {
	lines := []string{}
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n    ")
}

func runVerifyPublicChain() error {
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)
	bold := color.New(color.Bold)

	bold.Println(">> [1/8] Reading PUBLIC_CHAIN_ID from contracts container")
	chainOut, err := stacks.ExecContractsCapture([]string{"printenv", "PUBLIC_CHAIN_ID"}, true)
	if err != nil {
		// The most common cause is running from the wrong directory: every
		// rayls command operates on the docker-compose.yaml in the CURRENT
		// directory (like docker compose itself), so a stale compose file in
		// some other directory yields `service "contracts" is not running`
		// even while the stack is healthy in its own directory.
		return fmt.Errorf("could not read PUBLIC_CHAIN_ID from contracts container: %w\n\nrayls commands act on the docker-compose.yaml in the current directory — run this from\nthe directory where you ran `rayls init` (check with `rayls ps`)", err)
	}
	chainID := strings.TrimSpace(chainOut)
	if chainID == "" {
		return fmt.Errorf("PUBLIC_CHAIN_ID is empty — is the stack initialized with --public-chain?")
	}
	fmt.Printf("    public chain id: %s\n", chainID)

	userIDBytes := make([]byte, 32)
	if _, err := rand.Read(userIDBytes); err != nil {
		return fmt.Errorf("rand: %w", err)
	}
	userID := "0x" + hex.EncodeToString(userIDBytes)

	// Steps 2-3 are idempotent by intent and EXPECTED to fail on stacks whose
	// deploy pre-registers the deployer (the 3.0.1 deploy does:
	// RNUserGovernanceV1__PublicAddressAlreadyMapped) — run them silently, name
	// the KNOWN benign outcomes in one line, and surface anything else (the
	// tasks swallow their own errors and exit 0, so the captured output is the
	// only evidence there is). Both steps continue either way: the bridge's
	// final balance poll is the ground truth.
	reportIdempotentStep := func(out string, err error, benignMarkers []string, benignNote string) {
		failure := contractsTaskFailed(out, err)
		if failure == nil {
			return
		}
		for _, marker := range benignMarkers {
			if strings.Contains(out, marker) {
				yellow.Println("    " + benignNote)
				return
			}
		}
		yellow.Printf("    step did not succeed (continuing — the bridge outcome below is the ground truth):\n    %s\n", outputTail(out, 4))
	}

	bold.Println(">> [2/8] Registering deployer as a user (idempotent)")
	out2, err2 := stacks.ExecContractsCaptureSilent([]string{
		"npx", "hardhat", "createUser",
		"--pn", "A",
		"--user-id", userID,
		"--public-address", verifyDeployerAddress,
		"--private-address", verifyDeployerAddress,
	})
	// 0x609fe1e4 is the RNUserGovernanceV1__PublicAddressAlreadyMapped()
	// selector: the same benign outcome, seen raw when the task's ethers call
	// cannot decode the custom error.
	reportIdempotentStep(out2, err2,
		[]string{"PublicAddressAlreadyMapped", "already registered", "already exists", "0x609fe1e4"},
		"deployer address already registered — continuing (expected: the contracts deploy pre-registers it)")

	bold.Println(">> [3/8] Approving the user")
	out3, err3 := stacks.ExecContractsCaptureSilent([]string{
		"npx", "hardhat", "approveUser",
		"--pn", "A",
		"--user-id", userID,
	})
	reportIdempotentStep(out3, err3,
		[]string{"has no address pairs", "already approved", "User does not exist"},
		"approval not needed for this user — continuing (the deployer's pre-registered mapping is already active)")

	suffixBytes := make([]byte, 3)
	if _, err := rand.Read(suffixBytes); err != nil {
		return fmt.Errorf("rand: %w", err)
	}
	tokenName := "DEMO_" + strings.ToUpper(hex.EncodeToString(suffixBytes))

	bold.Printf(">> [4/8] Deploying %s ERC20 on PN A\n", tokenName)
	// The deploy task in the current contracts image fails at the final
	// submitTokenRegistration call (method removed from the new TokenExample),
	// but the token itself is deployed before that point. We swallow the
	// error and grab the address from the streamed output.
	deployOut, _ := stacks.ExecContractsCapture([]string{
		"npx", "hardhat", "tokens:erc20:deploy",
		"--pn", "A",
		"--name", tokenName,
		"--symbol", tokenName,
	}, false)
	m := tokenAddrRegex.FindStringSubmatch(deployOut)
	if len(m) < 2 {
		red.Println("    could not parse deployed token address from output")
		return fmt.Errorf("token deploy failed")
	}
	tokenAddr := m[1]
	green.Printf("    deployed %s at %s\n", tokenName, tokenAddr)

	// The 3.0.1 token-registry epic replaced the TokenGovernance flow (addToken
	// + approveLastRaylsNodeToken) with PNTokenRegistry tasks: tokens:register
	// -> tokens:approve-pn (privacyNodeStatus = AUTHORIZED) ->
	// submitTokenToPublicChain (activates public-chain bridging). Probe which
	// task set this contracts image ships (`hardhat help <task>` exits non-zero
	// on an unknown task) and keep the legacy flow for 3.0.0-era images. The
	// discriminator is tokens:approve-pn — it only exists once the epic
	// completed; the published lean image is a mid-epic hybrid that already
	// carries a (different-signature) tokens:register but still bridges via the
	// legacy TokenGovernance flow.
	_, probeErr := stacks.ExecContractsCaptureSilent([]string{"npx", "hardhat", "help", "tokens:approve-pn"})
	tokenRegistryFlow := probeErr == nil

	// Fatal task steps: check BOTH the exit code and the captured output —
	// see contractsTaskFailed for why exit codes alone are blind here.
	runTask := func(label string, args ...string) error {
		out, err := stacks.ExecContractsCapture(append([]string{"npx", "hardhat"}, args...), false)
		if failure := contractsTaskFailed(out, err); failure != nil {
			return fmt.Errorf("%s failed: %w", label, failure)
		}
		return nil
	}

	if tokenRegistryFlow {
		bold.Println(">> [5/8] Registering token in the PN token registry (tokens:register)")
		if err := runTask("tokens:register", "tokens:register", "--pn", "A", "--token-address", tokenAddr); err != nil {
			return err
		}

		bold.Println(">> [6/8] Authorizing token and submitting it to the public chain")
		if err := runTask("tokens:approve-pn", "tokens:approve-pn", "--pn", "A", "--token-address", tokenAddr); err != nil {
			return err
		}
		// Mint the bridge amount to the deployer — ProductionErc20Token starts
		// at zero supply and the bridge locks tokens on the PN
		// (RaylsErc20Handler__InsufficientBalanceToLock otherwise). mint() is
		// whenPrivacyNodeActive-gated, so it must follow tokens:approve-pn.
		if err := runTask("tokens:erc20:mint", "tokens:erc20:mint", "--pn", "A", "--symbol", tokenName, "--to", verifyDeployerAddress, "--amount", verifyBridgeAmountWei); err != nil {
			return err
		}
		if err := runTask("submitTokenToPublicChain", "submitTokenToPublicChain", "--pn", "A", "--token-address", tokenAddr); err != nil {
			return err
		}
	} else {
		bold.Println(">> [5/8] Registering token in TokenGovernance (addToken)")
		if err := runTask("addToken", "addToken", "--pn", "A", "--token-address", tokenAddr); err != nil {
			return err
		}

		bold.Println(">> [6/8] Activating token (approveLastRaylsNodeToken)")
		if err := runTask("approveLastRaylsNodeToken", "approveLastRaylsNodeToken", "--pn", "A"); err != nil {
			return err
		}
	}

	bold.Println(">> [7/8] Waiting for relayer to deploy the public-chain mirror")
	publicTokenAddr := ""
	const mapAttempts = 36
	for attempt := 1; attempt <= mapAttempts; attempt++ {
		time.Sleep(5 * time.Second)
		out, _ := stacks.ExecContractsCapture([]string{
			"npx", "hardhat", "checkPublicChainBalance",
			"--private-token-address", tokenAddr,
			"--user-address", verifyDeployerAddress,
			"--destination-chain-id", chainID,
			"--pn", "A",
		}, true)
		mm := publicTokenRegex.FindStringSubmatch(out)
		if len(mm) >= 2 && mm[1] != "0x0000000000000000000000000000000000000000" {
			publicTokenAddr = mm[1]
			break
		}
		fmt.Printf("    (waiting for public mapping… attempt %d/%d)\n", attempt, mapAttempts)
	}
	if publicTokenAddr == "" {
		return fmt.Errorf("relayer never produced a public-chain mapping for %s\n"+
			"  The public relayer deploys the mirror on the public chain and then writes the mapping\n"+
			"  back to the PN token registry (updatePublicTokenAddress) — one of those two steps failed.\n"+
			"  Check its logs:  docker compose logs pubrelayer-a | grep -iE 'deploy|public address'", tokenAddr)
	}
	green.Printf("    public mirror at %s\n", publicTokenAddr)

	bold.Println(">> [8/8] Bridging 100 DEMO to the public chain")
	if err := runTask("sendTokenToPublicChain",
		"sendTokenToPublicChain",
		"--pn", "A",
		"--token-address", tokenAddr,
		"--destination-address", verifyDeployerAddress,
		"--amount", verifyBridgeAmountWei,
		"--destination-chain-id", chainID,
		"--private-key", verifyDeployerPrivateKey,
	); err != nil {
		return err
	}

	bold.Println("   Polling destination balance on the public chain")
	// 5 min window: a healthy stack settles in ~30s, but the pubrelayer's
	// private listener can be slow to warm up on the first bridge after
	// `init`, and the original 2-min window was tight enough to fail
	// false-positively on cold starts.
	const balAttempts = 60
	var rawBalance string
	for attempt := 1; attempt <= balAttempts; attempt++ {
		time.Sleep(5 * time.Second)
		out, _ := stacks.ExecContractsCapture([]string{
			"npx", "hardhat", "checkPublicChainBalance",
			"--private-token-address", tokenAddr,
			"--user-address", verifyDeployerAddress,
			"--destination-chain-id", chainID,
			"--pn", "A",
		}, true)
		bm := rawBalanceRegex.FindStringSubmatch(out)
		if len(bm) >= 2 && bm[1] != "0" {
			rawBalance = bm[1]
			break
		}
		fmt.Printf("    (waiting for balance… attempt %d/%d)\n", attempt, balAttempts)
	}
	if rawBalance == "" {
		return fmt.Errorf("bridged balance never arrived on chain %s", chainID)
	}

	fmt.Println()
	green.Println("================================================================")
	green.Println("  Public-chain bridge verified end-to-end")
	green.Println("================================================================")
	fmt.Printf("  Token symbol:           %s\n", tokenName)
	fmt.Printf("  Privacy-node token:     %s\n", tokenAddr)
	fmt.Printf("  Public-chain token:     %s\n", publicTokenAddr)
	fmt.Printf("  Recipient address:      %s\n", verifyDeployerAddress)
	fmt.Printf("  Public chain id:        %s\n", chainID)
	fmt.Printf("  Final balance (wei):    %s\n", rawBalance)
	return nil
}
