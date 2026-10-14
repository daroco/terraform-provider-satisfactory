provider "satisfactory" {
  # Base URL of the mod's localhost API. Optional; defaults to the
  # SATISFACTORY_ENDPOINT env var, then http://localhost:8090.
  endpoint = "http://localhost:8090"

  # Bearer token, only needed when the mod is configured with one (for
  # non-loopback / dedicated-server setups). Optional; defaults to the
  # SATISFACTORY_TOKEN env var. Prefer the env var over hardcoding it.
  # token = var.satisfactory_token
}

# Show every placement a plan would make as a hologram in the running game,
# and surface the game's placement verdict as plan warnings. Off by default;
# also settable with SATISFACTORY_PREVIEW=1.
# preview = true
