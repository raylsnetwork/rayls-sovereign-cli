#!/bin/bash
# Generate manifest.json with checksums from dist/ directory

set -e

VERSION="${1:-v0.0.1}"
RELEASED=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
RELEASE_NOTES="${2:-https://github.com/raylsnetwork/rayls-sovereign-cli/releases/tag/${VERSION}}"

if [ ! -d "dist" ]; then
    echo "Error: dist/ directory not found. Run 'make build-all' first."
    exit 1
fi

echo "Generating manifest.json for version ${VERSION}..."

# Read checksums
DARWIN_AMD64_SHA=$(cat dist/rayls-darwin-amd64.sha256 2>/dev/null || echo "")
DARWIN_ARM64_SHA=$(cat dist/rayls-darwin-arm64.sha256 2>/dev/null || echo "")
LINUX_AMD64_SHA=$(cat dist/rayls-linux-amd64.sha256 2>/dev/null || echo "")
LINUX_ARM64_SHA=$(cat dist/rayls-linux-arm64.sha256 2>/dev/null || echo "")

# Generate manifest
cat > manifest.json <<EOF
{
  "version": "${VERSION}",
  "released": "${RELEASED}",
  "releaseNotes": "${RELEASE_NOTES}",
  "platforms": {
    "darwin-amd64": {
      "url": "https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-amd64",
      "sha256": "${DARWIN_AMD64_SHA}"
    },
    "darwin-arm64": {
      "url": "https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64",
      "sha256": "${DARWIN_ARM64_SHA}"
    },
    "linux-amd64": {
      "url": "https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-amd64",
      "sha256": "${LINUX_AMD64_SHA}"
    },
    "linux-arm64": {
      "url": "https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-arm64",
      "sha256": "${LINUX_ARM64_SHA}"
    }
  }
}
EOF

echo "✓ manifest.json generated successfully"
echo ""
echo "Version:  ${VERSION}"
echo "Released: ${RELEASED}"
echo ""
echo "Next steps:"
echo "  1. Review manifest.json"
echo "  2. Upload binaries: make upload-s3"
echo "  3. Or manually: aws s3 cp dist/* s3://rayls-cli/ --recursive --acl public-read"
