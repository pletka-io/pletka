# Security Policy

## Reporting a vulnerability

**Please do not open public GitHub issues for security vulnerabilities.**

The preferred channel is GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-vulnerability) feature on this repository: navigate to the **Security** tab -> **Report a vulnerability**. This is end-to-end encrypted between you and the maintainers, and creates a tracked advisory once acknowledged.

If you cannot use GitHub private vulnerability reporting, send an email to:

> **sjoerd.siebinga@gmail.com**

Use the subject prefix `[pletka security]`. PGP encryption is not currently offered.

## Runtime debug endpoints

Pletka can start an optional pprof server for performance debugging. It is
disabled by default and binds to `localhost:6060` unless explicitly configured:

```yaml
debug:
  pprof:
    enabled: false
    host: localhost
    port: "6060"
```

Do not expose pprof directly to the public internet. If production profiling is
needed, bind it to localhost or a private interface and access it through a
secured tunnel or operator-only network path.

## What to include

- A clear description of the issue and the affected version(s) / commit(s).
- Steps to reproduce, or a minimal proof-of-concept.
- The impact you observed and the impact you believe is possible.
- Any suggested mitigations or patches.

## Disclosure timeline

- **Within 72 hours:** acknowledgement of receipt.
- **Within 14 days:** initial triage with a severity assessment and an estimate of the fix timeline.
- **Coordinated disclosure:** we aim to publish a fix and a GitHub Security Advisory within 90 days of the report. We will work with you on the public disclosure date and credit.

## Supported versions

Pletka is in early v0 development. Only the latest tagged release on `main` receives security fixes.

| Version | Supported          |
| ------- | ------------------ |
| `v0.x.y` (latest) | :white_check_mark: |
| earlier | :x: |

## Out of scope

- Reports relying on misconfiguration of the deployer's environment (TLS, firewalls, OS, etc.) rather than the Pletka code itself.
- Denial of service via unreasonable resource exhaustion in development-mode endpoints.
- Findings from automated scanners without a working proof of concept.
