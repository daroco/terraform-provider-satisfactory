# Changelog

All notable changes to this provider are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Release tooling for the Terraform Registry: GoReleaser config, registry
  manifest (protocol 6), and a tag-triggered release workflow.
- Generated registry documentation (`docs/`) and per-resource examples under
  `examples/`, wired through `tfplugindocs` (`make docs`).
- Community health files: contributing guide, security policy, code of
  conduct, issue/PR templates, and CODEOWNERS.
- CI now enforces `gofmt` and `golangci-lint` in addition to build, vet, and
  tests.

## [0.1.0] - Unreleased

Initial functionality, verified live end to end against a running game
session.

### Added

- `satisfactory_foundation` — passive structural buildables; any change
  replaces the actor.
- `satisfactory_building` — production machines; `recipe` and `clock_speed`
  update in place, everything else replaces.
- `satisfactory_belt` — a conveyor between two factory connectors.
- `satisfactory_power_line` — a wire between two power connectors.
- `tf_id`-based identity persisted in the save game, so plans stay clean across
  save/load and in-game dismantling surfaces as drift.
- In-memory mockserver mirroring the mod API for provider development and CI
  without launching the game.

[Unreleased]: https://github.com/daroco/terraform-provider-satisfactory/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/daroco/terraform-provider-satisfactory/releases/tag/v0.1.0
