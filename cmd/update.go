package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	manifestURL = "https://rayls-cli.s3.eu-west-2.amazonaws.com/manifest.json"
	cacheTTL    = 24 * time.Hour
)

// Manifest represents the version manifest from S3
type Manifest struct {
	Version      string              `json:"version"`
	Released     string              `json:"released"`
	ReleaseNotes string              `json:"releaseNotes,omitempty"`
	Platforms    map[string]Platform `json:"platforms"`
}

// Platform represents a platform-specific binary
type Platform struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// UpdateCache stores the last update check result
type UpdateCache struct {
	LastChecked    string `json:"lastChecked"`
	LatestVersion  string `json:"latestVersion"`
	CurrentVersion string `json:"currentVersion"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Manage CLI updates",
	Long:  `Check for updates and get installation instructions for the latest version.`,
}

var updateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for available updates",
	Long:  `Check if a newer version of Rayls CLI is available and show installation instructions.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkForUpdates(); err != nil {
			color.Red("Error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd)
}

func checkForUpdates() error {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	white := color.New(color.FgWhite).SprintFunc()

	// Fetch manifest
	manifest, err := fetchManifest()
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Compare versions
	currentVersion := strings.TrimSpace(Version)
	latestVersion := strings.TrimSpace(manifest.Version)

	fmt.Printf("%s %s\n", cyan("Current version:"), white(currentVersion))
	fmt.Printf("%s %s\n", cyan("Latest version:"), white(latestVersion))

	// Save to cache (always, regardless of result)
	if err := saveUpdateCache(latestVersion, currentVersion); err != nil {
		// Non-fatal, just warn
		fmt.Printf("%s Failed to save update cache: %v\n", yellow("⚠"), err)
	}

	if currentVersion == latestVersion {
		fmt.Printf("\n%s You're running the latest version!\n", green("✓"))
		return nil
	}

	// Check if update is needed
	isNewer, err := isVersionNewer(latestVersion, currentVersion)
	if err != nil {
		return fmt.Errorf("failed to compare versions: %w", err)
	}

	if !isNewer {
		fmt.Printf("\n%s Your version is newer than the published version.\n", yellow("⚠"))
		return nil
	}

	// Update available
	fmt.Printf("\n%s A new version is available!\n\n", yellow("🎉"))

	// Determine platform
	platformKey := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	platform, exists := manifest.Platforms[platformKey]
	if !exists {
		return fmt.Errorf("no binary available for platform: %s", platformKey)
	}

	// Show installation instructions
	fmt.Printf("%s\n", cyan("To update, run:"))
	fmt.Printf("  curl -L %s -o rayls && chmod +x rayls\n\n", platform.URL)
	//fmt.Printf("  curl -L %s -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/\n\n", platform.URL)

	if manifest.ReleaseNotes != "" {
		fmt.Printf("%s %s\n", cyan("Release notes:"), manifest.ReleaseNotes)
	}

	return nil
}

func fetchManifest() (*Manifest, error) {
	resp, err := http.Get(manifestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// isVersionNewer compares two semantic versions
// Returns true if remote is newer than local
func isVersionNewer(remote, local string) (bool, error) {
	// Strip 'v' prefix if present
	remote = strings.TrimPrefix(remote, "v")
	local = strings.TrimPrefix(local, "v")

	// Simple string comparison works for semantic versions
	// v0.0.1 < v0.0.2 < v0.1.0 < v1.0.0
	remoteParts := strings.Split(remote, ".")
	localParts := strings.Split(local, ".")

	// Pad to same length
	for len(remoteParts) < 3 {
		remoteParts = append(remoteParts, "0")
	}
	for len(localParts) < 3 {
		localParts = append(localParts, "0")
	}

	// Compare each part
	for i := 0; i < 3; i++ {
		var remoteNum, localNum int
		fmt.Sscanf(remoteParts[i], "%d", &remoteNum)
		fmt.Sscanf(localParts[i], "%d", &localNum)

		if remoteNum > localNum {
			return true, nil
		} else if remoteNum < localNum {
			return false, nil
		}
	}

	return false, nil // Equal
}

func getUpdateCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	cacheDir := filepath.Join(home, ".rayls")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, "update-check.json"), nil
}

func saveUpdateCache(latestVersion, currentVersion string) error {
	cachePath, err := getUpdateCachePath()
	if err != nil {
		return err
	}

	cache := UpdateCache{
		LastChecked:    time.Now().Format(time.RFC3339),
		LatestVersion:  latestVersion,
		CurrentVersion: currentVersion,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0644)
}

func loadUpdateCache() (*UpdateCache, error) {
	cachePath, err := getUpdateCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache exists
		}
		return nil, err
	}

	var cache UpdateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// checkForUpdatesBackground is a helper that can be called from other commands
// It only checks if cache is stale (>24h old)
func checkForUpdatesBackground() {
	cache, err := loadUpdateCache()
	if err != nil {
		return // Silent fail
	}

	// No cache or cache is stale
	if cache == nil {
		return // Don't check on first run
	}

	lastChecked, err := time.Parse(time.RFC3339, cache.LastChecked)
	if err != nil {
		return
	}

	// If checked recently, skip
	if time.Since(lastChecked) < cacheTTL {
		// If we have a cached newer version, show notification
		if cache.LatestVersion != cache.CurrentVersion {
			yellow := color.New(color.FgYellow).SprintFunc()
			fmt.Printf("%s Update available: %s → %s (run 'rayls update check' for details)\n\n",
				yellow("⚠"), cache.CurrentVersion, cache.LatestVersion)
		}
		return
	}

	// Cache is stale, refresh in background (don't block)
	// For now, just a skeleton - we can implement async check later
}
