package stacks

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

// pnARPCURL is PN A's EVM JSON-RPC as published on the host: every generated
// compose maps privacy-node-a (participant index 0) to 127.0.0.1:8545.
const pnARPCURL = "http://127.0.0.1:8545"

// DeployerApprovedOnPNA reports whether the given address has an APPROVED
// address pair in PN A's RNUserGovernanceV1 — the hard precondition of
// teleportToPublicChain. The governance proxy address exists only inside the
// contracts container (the deploy writes it into the image's .env), so it is
// read with an in-container grep; the check itself is a raw eth_call from the
// host, keeping the image's node/ethers runtime out of the loop. The contract
// reverts (PrivateAddressNotMapped) for unknown addresses, so an RPC-level
// error reads as "not approved" rather than a hard failure — the caller
// retries; only a broken transport/setup surfaces as err.
func DeployerApprovedOnPNA(deployer string) (bool, error) {
	out, err := ExecContractsCaptureSilent([]string{"grep", "-m1", "^PRIVACY_NODE_A_RAYLS_NODE_USER_GOVERNANCE=", ".env"})
	if err != nil {
		return false, fmt.Errorf("reading the PN A user-governance address from the contracts container: %w", err)
	}
	_, govAddr, found := strings.Cut(strings.TrimSpace(out), "=")
	if !found || !strings.HasPrefix(govAddr, "0x") {
		return false, fmt.Errorf("unexpected PRIVACY_NODE_A_RAYLS_NODE_USER_GOVERNANCE line %q in the contracts .env", strings.TrimSpace(out))
	}

	// checkUserIsApprovedByPrivateAddress(address): 4-byte selector
	// (0xc4ba03ed) followed by the address as one left-padded 32-byte word.
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte("checkUserIsApprovedByPrivateAddress(address)"))
	callData := "0x" + hex.EncodeToString(h.Sum(nil)[:4]) +
		strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(deployer, "0x"))

	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1,
		"method": "eth_call",
		"params": []any{map[string]string{"to": govAddr, "data": callData}, "latest"},
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(pnARPCURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var rpcOut struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcOut); err != nil {
		return false, err
	}
	if rpcOut.Error != nil {
		// Reverts land here: the deployer is simply not mapped/approved yet.
		return false, nil
	}
	return strings.HasSuffix(rpcOut.Result, "1"), nil
}
