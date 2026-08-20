// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Rayls Core Ltd.
package main

import "github.com/raylsnetwork/rayls-sovereign-cli/cmd"

func main() {
	// Set version information for cmd package
	cmd.Version = Version
	cmd.CommitSHA = CommitSHA
	cmd.BuildDate = BuildDate

	cmd.Execute()
}
