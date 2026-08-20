package stacks

import (
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

func TestDeployerAddress(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		// Publicly documented Anvil/Hardhat dev pairings.
		{"anvil account 0 with prefix", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"},
		{"anvil account 1 bare", "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deployerAddress(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.EqualFold(got, tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
	if _, err := deployerAddress("nothex"); err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestRequiredInitFundsWei(t *testing.T) {
	rayls := func(tenths int64) *big.Int {
		return new(big.Int).Mul(big.NewInt(tenths), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil))
	}
	if got := requiredInitFundsWei(1); got.Cmp(rayls(45)) != 0 { // 2 + 2.5
		t.Errorf("1 participant: got %s, want 4.5 RAYLS", weiToRayls(got))
	}
	if got := requiredInitFundsWei(3); got.Cmp(rayls(95)) != 0 { // 2 + 7.5
		t.Errorf("3 participants: got %s, want 9.5 RAYLS", weiToRayls(got))
	}
}

func TestWeiToRayls(t *testing.T) {
	wei, _ := new(big.Int).SetString("1566278887397589872", 10)
	if got := weiToRayls(wei); got != "1.5663" {
		t.Errorf("got %s, want 1.5663", got)
	}
}

// stubRPC serves a fixed eth_getBalance result (hex wei), or a 500 when
// balanceHex is empty.
func stubRPC(t *testing.T, balanceHex string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if balanceHex == "" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":"%s"}`, balanceHex)
	}))
}

func TestCheckDeployerBalance(t *testing.T) {
	key := strings.Repeat("ab", 32)

	t.Run("sufficient balance passes", func(t *testing.T) {
		srv := stubRPC(t, "0x4563918244f40000") // 5 RAYLS > 4.5 required
		defer srv.Close()
		pc := &docker.PublicChain{Name: "rayls-testnet", RPC: srv.URL, Faucet: docker.FundingURL}
		if err := checkDeployerBalance(pc, key, 1); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("insufficient balance refuses with shortfall and funding URL", func(t *testing.T) {
		srv := stubRPC(t, "0x15bcacb1eb98fdf0") // ~1.5663 RAYLS < 4.5
		defer srv.Close()
		pc := &docker.PublicChain{Name: "rayls-testnet", RPC: srv.URL, Faucet: docker.FundingURL}
		err := checkDeployerBalance(pc, key, 1)
		if err == nil {
			t.Fatal("expected an error")
		}
		for _, want := range []string{"insufficient funds", "shortfall", docker.FundingURL, "4.5000"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error missing %q:\n%v", want, err)
			}
		}
	})

	t.Run("unreachable RPC warns but does not block", func(t *testing.T) {
		pc := &docker.PublicChain{Name: "rayls-testnet", RPC: "http://127.0.0.1:1", Faucet: docker.FundingURL}
		if err := checkDeployerBalance(pc, key, 1); err != nil {
			t.Errorf("RPC failure must not block init, got %v", err)
		}
	})

	t.Run("rpc-level error warns but does not block", func(t *testing.T) {
		srv := stubRPC(t, "")
		defer srv.Close()
		pc := &docker.PublicChain{Name: "rayls-testnet", RPC: srv.URL, Faucet: docker.FundingURL}
		if err := checkDeployerBalance(pc, key, 1); err != nil {
			t.Errorf("RPC error must not block init, got %v", err)
		}
	})
}
