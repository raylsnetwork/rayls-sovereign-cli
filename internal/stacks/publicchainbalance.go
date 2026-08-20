package stacks

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/fatih/color"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"golang.org/x/crypto/sha3"
)

// Preflight for testnet-bridged inits: an underfunded deployer key fails
// MINUTES into the deploy with errors that name neither account nor amount.
// Cost model (observed on rayls-testnet at the deploy's fixed 100 gwei):
// ~2 deploy gas (~1.67 upfront) + 0.5 x 5 relayer wallets per participant,
// re-spent on every fresh deploy; auth re-seeds on each contracts restart.
const (
	deployGasRaylsX10          = 20 // ~2.0 deploy gas, in tenths to stay integer
	perParticipantSeedRaylsX10 = 25 // 2.5 relayer seeding per participant, in tenths
)

func requiredInitFundsWei(participants int) *big.Int {
	tenthRayls := new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil) // 0.1 in wei
	tenths := int64(deployGasRaylsX10) + int64(perParticipantSeedRaylsX10)*int64(participants)
	return new(big.Int).Mul(big.NewInt(tenths), tenthRayls)
}

// deployerAddress derives the 0x EVM address from a bare or 0x-prefixed hex
// private key.
func deployerAddress(privKeyHex string) (string, error) {
	keyHex, err := normalizePrivateKey(privKeyHex)
	if err != nil {
		return "", err
	}
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", err
	}
	priv := secp256k1.PrivKeyFromBytes(keyBytes)
	pub := priv.PubKey().SerializeUncompressed() // 65 bytes, leading 0x04
	h := sha3.NewLegacyKeccak256()
	h.Write(pub[1:])
	return "0x" + hex.EncodeToString(h.Sum(nil)[12:]), nil
}

// fetchBalanceWei is one eth_getBalance POST with a short timeout; the check
// must never stall an init.
func fetchBalanceWei(rpcURL, address string) (*big.Int, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1,
		"method": "eth_getBalance",
		"params": []string{address, "latest"},
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(rpcURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", out.Error.Message)
	}
	bal, ok := new(big.Int).SetString(strings.TrimPrefix(out.Result, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("unparseable balance %q", out.Result)
	}
	return bal, nil
}

// weiToRayls renders wei as a decimal string with 4 fractional digits.
func weiToRayls(wei *big.Int) string {
	r := new(big.Rat).SetFrac(wei, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	return r.FloatString(4)
}

// checkDeployerBalance refuses the init only on a definitive shortfall; an
// unreachable RPC just warns, since a flaky endpoint must not block an init
// the deploy itself would settle.
func checkDeployerBalance(pc *docker.PublicChain, privKeyHex string, participants int) error {
	yellow := color.New(color.FgYellow)

	address, err := deployerAddress(privKeyHex)
	if err != nil {
		return fmt.Errorf("deriving the deployer address: %v", err)
	}
	balance, err := fetchBalanceWei(pc.RPC, address)
	if err != nil {
		yellow.Printf("Could not check the deployer balance on %s (%v). Proceeding anyway; an underfunded key will fail during the deploy.\n", pc.Name, err)
		return nil
	}
	required := requiredInitFundsWei(participants)
	if balance.Cmp(required) >= 0 {
		fmt.Printf("Deployer %s balance on %s: %s (needs ~%s for a fresh deploy) ✓\n",
			address, pc.Name, weiToRayls(balance), weiToRayls(required))
		return nil
	}
	shortfall := new(big.Int).Sub(required, balance)
	return fmt.Errorf(`insufficient funds on the deployer account for %s.

  account:   %s
  balance:   %s
  required:  ~%s  (deploy gas ~2 + 2.5 x %d participant(s) relayer seeding)
  shortfall: %s

Top up via %s and re-run.`,
		pc.Name, address, weiToRayls(balance), weiToRayls(required), participants,
		weiToRayls(shortfall), pc.Faucet)
}
