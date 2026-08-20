package stacks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
)

// ExecContracts executes a command inside the running contracts container
func ExecContracts(args []string) error {
	// Check Docker environment
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return err
	}

	// Prepare arguments for docker compose exec
	// We target the "contracts" service
	commandArgs := []string{"exec", "contracts"}
	commandArgs = append(commandArgs, args...)

	var cmd *exec.Cmd
	if version == docker.ComposeV2 {
		// docker compose exec ...
		fullArgs := append([]string{"compose"}, commandArgs...)
		cmd = exec.Command("docker", fullArgs...)
	} else {
		// docker-compose exec ...
		cmd = exec.Command("docker-compose", commandArgs...)
	}

	// Connect streams to allow interaction and viewing output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

// ExecContractsCaptureSilent runs a command inside the contracts container with
// BOTH stdout and stderr captured and nothing echoed to the terminal. For steps
// whose failure is expected and handled (e.g. idempotent re-registration), so
// their revert dumps don't alarm the user.
func ExecContractsCaptureSilent(args []string) (string, error) {
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return "", err
	}

	commandArgs := []string{"exec", "-T", "contracts"}
	commandArgs = append(commandArgs, args...)

	var cmd *exec.Cmd
	if version == docker.ComposeV2 {
		fullArgs := append([]string{"compose"}, commandArgs...)
		cmd = exec.Command("docker", fullArgs...)
	} else {
		cmd = exec.Command("docker-compose", commandArgs...)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	return buf.String(), err
}

// ExecContractsCapture runs a command inside the contracts container, returning
// captured stdout. When quiet is false the output is also teed to the user's
// terminal so they can watch the hardhat task progress. "-T" disables TTY
// allocation so output is plain text we can grep.
func ExecContractsCapture(args []string, quiet bool) (string, error) {
	version, err := docker.CheckDockerConfig()
	if err != nil {
		return "", err
	}

	commandArgs := []string{"exec", "-T", "contracts"}
	commandArgs = append(commandArgs, args...)

	var cmd *exec.Cmd
	if version == docker.ComposeV2 {
		fullArgs := append([]string{"compose"}, commandArgs...)
		cmd = exec.Command("docker", fullArgs...)
	} else {
		cmd = exec.Command("docker-compose", commandArgs...)
	}

	var buf bytes.Buffer
	if quiet {
		cmd.Stdout = &buf
	} else {
		cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	}
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	return buf.String(), err
}
