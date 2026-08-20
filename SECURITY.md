# Security Policy

The Rayls Network team takes the security of the Rayls stack and its users seriously.
We are grateful to the security researchers and operators who help keep the project safe.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, pull requests, or Discord.**

Report vulnerabilities privately through **GitHub's private vulnerability reporting**:

1. Open the [Security tab](https://github.com/raylsnetwork/rayls-sovereign-cli/security) of this repository.
2. Click **"Report a vulnerability"** and complete the advisory form.

This opens a channel visible only to the maintainers. If you are unable to use GitHub's
private reporting, contact a maintainer via the
[Rayls Network Discord](https://discord.com/channels/1252990258514235544/1252996402942836857)
to arrange a secure disclosure channel — do **not** include vulnerability details in public messages.

Where possible, please include:

- A description of the vulnerability and its potential impact.
- Steps to reproduce, or a proof of concept.
- The affected component(s), version(s), and configuration.
- Any suggested remediation.

## Response Process

1. We will acknowledge receipt of your report within **48 hours**.
2. We will provide an initial assessment within **5 business days**.
3. We will keep you informed of our progress as we investigate and resolve the issue.
4. Once resolved, we will notify you and coordinate public disclosure timing.

## Scope

This policy covers the `rayls-sovereign-cli` tool in this repository. The CLI orchestrates a
local Rayls demo stack from published container images; vulnerabilities in those images or in
the other Rayls components should be reported against their respective repositories.

### Out of Scope
- Already reported vulnerabilities.
- Vulnerabilities in third-party dependencies (report to the dependency maintainer).
- Theoretical vulnerabilities without a proof of concept.
- Social engineering attacks.

## Disclosure Policy

- All vulnerability reports and associated communications are treated as confidential.
- We kindly ask that you **not publicly disclose** any details until we have released a fix and
  agreed on a disclosure timeline.
- We aim to fix critical vulnerabilities as quickly as possible.

## Supported Versions

The CLI is under active development. Security fixes are released against the **latest release**;
users are strongly encouraged to always run the latest version (`rayls update check`).
