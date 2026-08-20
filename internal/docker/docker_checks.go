package docker

import (
	"fmt"
	"os"
	"os/exec"
)

// CheckDockerConfig verifies that Docker and Docker Compose are installed and running.
// It checks for the Docker daemon and detects which version of Docker Compose is available
// (v1 using docker-compose or v2 using docker compose). Returns the detected version or
// an error if Docker is not available.
func CheckDockerConfig() (DockerComposeVersion, error) {

	dockerCmd := exec.Command("docker", "-v")
	_, err := dockerCmd.Output()
	if err != nil {
		return None, fmt.Errorf("an error occurred while running docker. Is docker installed on your computer?")
	}

	dockerDaemonCheck := exec.Command("docker", "ps")
	_, err = dockerDaemonCheck.Output()
	if err != nil {
		return None, fmt.Errorf("an error occurred while running docker. Is docker daemon running?")
	}

	dockerComposeCmd := exec.Command("docker", "compose", "version")
	_, err = dockerComposeCmd.Output()
	if err == nil {
		return ComposeV2, nil
	}

	dockerComposeV1Cmd := exec.Command("docker-compose", "-v")
	_, err = dockerComposeV1Cmd.Output()
	if err == nil {
		return ComposeV1, nil
	}

	return None, fmt.Errorf("an error occurred while running docker-compose. Is docker-compose installed on your computer?")
}

// DockerInfo retrieves information about the Docker daemon using the 'docker info' command.
// Returns the command output as bytes or an error if the command fails.
func DockerInfo() ([]byte, error) {
	dockerInfoCmd := exec.Command("docker", "info")
	out, err := dockerInfoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error getting Docker info: %w", err)
	}
	return out, nil
}

// DockerStats displays statistics for running containers. It detects the Docker Compose
// version and uses the appropriate command. For Compose v2, it runs 'docker compose stats',
// for Compose v1 it runs 'docker-compose stats', and falls back to 'docker stats' if
// neither is available. Output is streamed directly to stdout.
func DockerStats() error {
	version, err := CheckDockerConfig()
	var cmd *exec.Cmd

	if err == nil && version == ComposeV2 {
		cmd = exec.Command("docker", "compose", "stats", "--no-stream")
	} else if err == nil && version == ComposeV1 {
		cmd = exec.Command("docker-compose", "stats", "--no-stream")
	} else {
		cmd = exec.Command("docker", "stats", "--no-stream")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
