# Security Policy

## Supported versions

This provider is pre-1.0. Security fixes are made against the latest release
and `main`.

| Version | Supported |
| ------- | --------- |
| latest  | yes       |
| < latest | no       |

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability):
go to the repository's **Security** tab and choose **Report a vulnerability**.

Include enough detail to reproduce the issue (affected version, configuration,
and steps). You can expect an acknowledgement within a few days.

## Scope and threat model

The provider talks to the companion mod over a **localhost** HTTP API. A few
properties are important to understand when assessing risk:

- The mod's API is intended to bind to loopback. Exposing it beyond the local
  machine (for a dedicated server) requires setting `SATISFACTORY_TOKEN`; the
  provider sends it as a bearer token. Running the API on a non-loopback
  interface without a token is out of the supported configuration.
- The provider stores no secrets in state. The `token` attribute is marked
  sensitive and is read from the `SATISFACTORY_TOKEN` environment variable by
  default.
- Class and recipe names are opaque strings validated by the game at apply
  time, not by the provider; the provider does not execute game content.

Vulnerabilities in the in-game mod itself live in the
[daroco/SatisfactoryTerraform](https://github.com/daroco/SatisfactoryTerraform)
repository; report those there.
