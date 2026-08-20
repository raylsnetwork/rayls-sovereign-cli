# Release Process for Rayls CLI

## Quick Release Guide

### 1. Build All Platforms

```bash
# Set the version you're releasing
VERSION=v0.0.2 make build-all
```

This will:
- Build binaries for darwin-amd64, darwin-arm64, linux-amd64, linux-arm64
- Generate SHA256 checksums for each binary
- Place everything in the `dist/` directory

### 2. Update manifest.json

Edit `manifest.json` and update:
- `version`: Set to the new version (e.g., "v0.0.2")
- `released`: Set to current ISO timestamp
- `releaseNotes`: Update URL if you have release notes
- `sha256` fields: Copy from the `.sha256` files in `dist/`

Example:
```bash
# Get checksums
cat dist/rayls-darwin-amd64.sha256
cat dist/rayls-darwin-arm64.sha256
cat dist/rayls-linux-amd64.sha256
cat dist/rayls-linux-arm64.sha256
```

### 3. Upload to S3

#### Manual Upload (Current Process)

```bash
# Upload binaries
aws s3 cp dist/rayls-darwin-amd64 s3://rayls-cli/rayls-darwin-amd64 --acl public-read
aws s3 cp dist/rayls-darwin-arm64 s3://rayls-cli/rayls-darwin-arm64 --acl public-read
aws s3 cp dist/rayls-linux-amd64 s3://rayls-cli/rayls-linux-amd64 --acl public-read
aws s3 cp dist/rayls-linux-arm64 s3://rayls-cli/rayls-linux-arm64 --acl public-read

# Upload manifest
aws s3 cp manifest.json s3://rayls-cli/manifest.json --acl public-read --content-type application/json
```

#### Automated Upload (Requires AWS CLI)

```bash
VERSION=v0.0.2 make upload-s3
```

### 4. Test the Update

```bash
# Build with new version
VERSION=v0.0.1 make build

# Check for updates (should show v0.0.2 available)
./rayls version --check
# or
./rayls update check
```

## Version Numbering

Use semantic versioning: `vMAJOR.MINOR.PATCH`

- **v0.0.x**: Initial development, bug fixes
- **v0.x.0**: New features during alpha/beta
- **v1.0.0**: First stable release
- **v1.x.0**: New features (stable)
- **v1.0.x**: Bug fixes (stable)

## manifest.json Structure

```json
{
  "version": "v0.0.2",
  "released": "2026-01-14T15:30:00Z",
  "releaseNotes": "https://github.com/raylsnetwork/rayls-sovereign-cli/releases/tag/v0.0.2",
  "platforms": {
    "darwin-amd64": {
      "url": "https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-amd64",
      "sha256": "abc123..."
    },
    ...
  }
}
```

## Checklist

Before releasing:
- [ ] Update `VERSION` in Makefile (or pass as env var)
- [ ] Run `make build-all`
- [ ] Update manifest.json with new version and checksums
- [ ] Upload binaries to S3
- [ ] Upload manifest.json to S3
- [ ] Test update check: `./rayls version --check`
- [ ] Commit and tag: `git tag v0.0.2 && git push origin v0.0.2`

## User Installation

Users can install/update with:

```bash
# macOS (Apple Silicon)
curl -L https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64 -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/

# macOS (Intel)
curl -L https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-amd64 -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/

# Linux (x86_64)
curl -L https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-amd64 -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/

# Linux (ARM64)
curl -L https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-linux-arm64 -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/
```

## Troubleshooting

### Users see "no binary available for platform"

Make sure you've built and uploaded binaries for all platforms listed in manifest.json.

### Update check fails

Verify manifest.json is accessible:
```bash
curl https://rayls-cli.s3.eu-west-2.amazonaws.com/manifest.json
```

Make sure the S3 bucket has public-read ACL for manifest.json and all binaries.
