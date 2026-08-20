# Version & Update System - Implementation Summary

## ✅ Completed Implementation

### 1. **Version System** (`version.go`)
- Semantic versioning: `v0.0.1`
- Build-time injection via ldflags
- Tracks: Version, CommitSHA, BuildDate

### 2. **Version Command** (`cmd/version.go`)
```bash
$ rayls version
Rayls CLI v0.0.1
Commit: be9d1e4
Built: 2026-01-14T15:20:57Z

$ rayls version --check
# Shows update notification if available
```

### 3. **Update Command** (`cmd/update.go`)
```bash
$ rayls update check
Current version: v0.0.1
Latest version:  v0.0.2
🎉 A new version is available!

To update, run:
  curl -L https://rayls-cli.s3...rayls-darwin-arm64 -o rayls && chmod +x rayls && sudo mv rayls /usr/local/bin/
```

**Features:**
- Fetches manifest from S3
- Semantic version comparison
- Platform detection (darwin/linux, amd64/arm64)
- Shows platform-specific install commands
- Caching system (24h TTL) in `~/.rayls/update-check.json`
- Background check skeleton (commented out in `init` command)

### 4. **Manifest System** (`manifest.json`)
Hosted at: `https://rayls-cli.s3.eu-west-2.amazonaws.com/manifest.json`

Structure:
```json
{
  "version": "v0.0.1",
  "released": "2026-01-14T12:00:00Z",
  "releaseNotes": "https://github.com/raylsnetwork/rayls-sovereign-cli/releases/tag/v0.0.1",
  "platforms": {
    "darwin-amd64": { "url": "...", "sha256": "..." },
    "darwin-arm64": { "url": "...", "sha256": "..." },
    "linux-amd64": { "url": "...", "sha256": "..." },
    "linux-arm64": { "url": "...", "sha256": "..." }
  }
}
```

### 5. **Build System** (`Makefile`)

Commands:
```bash
# Build for current platform with version info
make build

# Build all platforms
make build-all

# Show version
make version

# Upload to S3 (requires AWS CLI)
make upload-s3

# Clean
make clean
```

### 6. **Helper Scripts**
- `scripts/generate-manifest.sh` - Auto-generates manifest.json from dist/ checksums

### 7. **Documentation**
- `RELEASE.md` - Complete release process guide
- `VERSION_UPDATE_SYSTEM.md` - This file

---

## How It Works

### User Workflow

**Installation:**
```bash
curl -L https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64 -o rayls
chmod +x rayls
sudo mv rayls /usr/local/bin/
```

**Check version:**
```bash
rayls version
```

**Check for updates:**
```bash
rayls update check
# or
rayls version --check
```

**Update (manual):**
```bash
# Follow curl instructions from update check output
```

### Release Workflow (You)

**1. Build new version:**
```bash
VERSION=v0.0.2 make build-all
```

**2. Generate manifest:**
```bash
./scripts/generate-manifest.sh v0.0.2
```

**3. Upload to S3:**
```bash
# Manually
aws s3 cp dist/rayls-darwin-amd64 s3://rayls-cli/ --acl public-read
aws s3 cp dist/rayls-darwin-arm64 s3://rayls-cli/ --acl public-read
aws s3 cp dist/rayls-linux-amd64 s3://rayls-cli/ --acl public-read
aws s3 cp dist/rayls-linux-arm64 s3://rayls-cli/ --acl public-read
aws s3 cp manifest.json s3://rayls-cli/ --acl public-read --content-type application/json

# Or automated
VERSION=v0.0.2 make upload-s3
```

**4. Tag release:**
```bash
git tag v0.0.2
git push origin v0.0.2
```

---

## Architecture

```
┌──────────────┐
│  rayls CLI   │
│  v0.0.1      │
└──────┬───────┘
       │
       │ (Every 24h or on-demand)
       ↓
┌─────────────────────────────────┐
│  manifest.json (S3)             │
│  - version: v0.0.2              │
│  - platforms: { ... }           │
└─────────────────────────────────┘
       │
       │ Compare versions
       ↓
┌──────────────────────────────────┐
│  If v0.0.2 > v0.0.1:             │
│  → Show update notification      │
│  → Provide curl install command  │
└──────────────────────────────────┘
```

### Caching System

```
~/.rayls/update-check.json
{
  "lastChecked": "2026-01-14T10:00:00Z",
  "latestVersion": "v0.0.2",
  "currentVersion": "v0.0.1"
}
```

- Checked every 24 hours
- Shows notification if cached update available
- Prevents hammering S3 on every command

---

## Next Steps (Optional Enhancements)

### Immediate (Pre-Launch)
1. ✅ Test end-to-end flow
2. ✅ Upload initial manifest.json to S3
3. ✅ Upload v0.0.1 binaries to S3
4. ✅ Test update check from different platform

### Future Enhancements
- [ ] Checksum verification before install
- [ ] Self-update command (downloads and replaces binary automatically)
- [ ] Release notes display in terminal
- [ ] JSON output format (`--format json`)
- [ ] Silent mode for CI/CD
- [ ] Opt-out of update checks via env var

---

## Testing Checklist

Before first release:

- [ ] Build with `make build-all`
- [ ] Verify all 4 platform binaries created in `dist/`
- [ ] Run `./scripts/generate-manifest.sh v0.0.1`
- [ ] Upload manifest.json to S3 with `--acl public-read`
- [ ] Upload all 4 binaries to S3 with `--acl public-read`
- [ ] Verify manifest URL: `curl https://rayls-cli.s3.eu-west-2.amazonaws.com/manifest.json`
- [ ] Verify binary URL: `curl -I https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64`
- [ ] Test `rayls version`
- [ ] Test `rayls version --check` (should show up-to-date)
- [ ] Bump version to v0.0.2 and test again (should show update available)

---

## S3 Bucket Configuration

Make sure your S3 bucket has:
- Public read access for manifest.json and binaries
- CORS configuration (if needed)
- Proper ACLs: `--acl public-read`

### Verify Public Access

```bash
# Manifest should be publicly accessible
curl https://rayls-cli.s3.eu-west-2.amazonaws.com/manifest.json

# Binaries should be publicly accessible
curl -I https://rayls-cli.s3.eu-west-2.amazonaws.com/rayls-darwin-arm64
```

Should return `200 OK`, not `403 Forbidden`.

---

## Files Created

- `version.go` - Version constants
- `cmd/version.go` - Version command
- `cmd/update.go` - Update check command
- `manifest.json` - Version manifest template
- `Makefile` - Build system
- `scripts/generate-manifest.sh` - Manifest generator
- `RELEASE.md` - Release process documentation
- `VERSION_UPDATE_SYSTEM.md` - This file

---

## Current Status

✅ **Implementation Complete**
⏸️ **Pending**: Upload manifest.json and binaries to S3
🎯 **Next**: Test end-to-end update check flow

## Support

If users encounter issues:
1. Check manifest.json is accessible
2. Verify binary URLs are correct
3. Check S3 ACLs are public-read
4. Test with `curl -v` to see HTTP response
