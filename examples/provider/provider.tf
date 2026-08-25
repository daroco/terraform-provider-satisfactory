provider "satisfactory" {
  # Base URL of the mod's localhost API. Optional; defaults to the
  # SATISFACTORY_ENDPOINT env var, then http://localhost:8090.
  endpoint = "http://localhost:8090"

  # Bearer token, only needed when the mod is configured with one (for
  # non-loopback / dedicated-server setups). Optional; defaults to the
  # SATISFACTORY_TOKEN env var. Prefer the env var over hardcoding it.
  # token = var.satisfactory_token
}
