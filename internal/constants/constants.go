package constants

import (
	"os"
	"path/filepath"
)

var StacksDir = checkHome()
var PrivacyLedgerCoreImage = "local/privacy-ledger"
var PostgresImageName = "postgres"

func checkHome() string {
	var homeDir, _ = os.UserHomeDir()
	var StacksDir = filepath.Join(homeDir, ".firefly", "stacks")
	var fireflyhome, present = os.LookupEnv("FIREFLY_HOME")
	if present {
		StacksDir = filepath.Join(fireflyhome, "stacks")
	}
	return StacksDir
}
