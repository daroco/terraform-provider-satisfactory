# A straight run of power poles between two points, wired in series, so a
# far-off miner or a coal plant can reach the factory grid. Poles are spaced
# at most `max_span` apart; the first and last can be wired to whatever
# needs connecting.

terraform {
  required_providers {
    satisfactory = {
      source = "daroco/satisfactory"
    }
  }
}

locals {
  dx   = var.to.x - var.from.x
  dy   = var.to.y - var.from.y
  dz   = var.to.z - var.from.z
  dist = pow(local.dx * local.dx + local.dy * local.dy + local.dz * local.dz, 0.5)
  # Number of spans; at least one so there are always two poles to wire
  # between (a single pole could not bridge anything).
  spans = max(1, ceil(local.dist / var.max_span))
  poles = {
    for i in range(local.spans + 1) : tostring(i) => {
      # Rounded to the centimetre: the game stores float32 anyway, and
      # a plan full of 40-digit coordinates is unreadable.
      x = floor(var.from.x + local.dx * i / local.spans + 0.5)
      y = floor(var.from.y + local.dy * i / local.spans + 0.5)
      z = floor(var.from.z + local.dz * i / local.spans + 0.5)
    }
  }
}

resource "satisfactory_building" "pole" {
  for_each = local.poles
  class    = var.pole_class
  x        = each.value.x
  y        = each.value.y
  z        = each.value.z
}

resource "satisfactory_power_line" "span" {
  for_each       = { for k, p in local.poles : k => p if tonumber(k) < local.spans }
  from_id        = satisfactory_building.pole[each.key].id
  from_connector = 0
  to_id          = satisfactory_building.pole[tostring(tonumber(each.key) + 1)].id
  to_connector   = 0
}

resource "satisfactory_power_line" "feed" {
  count          = var.feed != null ? 1 : 0
  from_id        = var.feed.from_id
  from_connector = var.feed.from_connector
  to_id          = satisfactory_building.pole["0"].id
  to_connector   = 0
}

resource "satisfactory_power_line" "sink" {
  count          = var.sink != null ? 1 : 0
  from_id        = satisfactory_building.pole[tostring(local.spans)].id
  from_connector = 0
  to_id          = var.sink.to_id
  to_connector   = var.sink.to_connector
}
