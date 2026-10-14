# tier4-factory

Every automatable part through Tier 4, ore to storage, coal-powered, in one
`terraform apply`. About 600 resources: 7 miners, 4 water extractors, 8 coal
generators, 42 production machines, their splitter manifolds, merger chains,
belts, pipes, poles and wires.

Nothing is a coordinate copied out of the game. Miners bind to nodes found
with `satisfactory_resource_nodes`, nearest `var.anchor` first; the floor is
laid out relative to the anchor; the coal plant relative to the lake it
found. Move the anchor and everything follows.

## What it makes

| Product (per minute) | Line | Machines | Clock |
|---|---|---|---|
| Smart Plating 2 | `smart_plating` | 1 assembler | 1.00 |
| Versatile Framework 2 | `versatile_frame` | 1 assembler | 0.40 |
| Automated Wiring 2.5 | `automated_wiring` | 1 assembler | 1.00 |
| Motor 1 | `motors` | 1 assembler | 0.20 |
| Encased Industrial Beam 2.5 | `eib` | 1 assembler | 0.42 |
| Copper Sheet 10 | `copper_sheet` | 1 constructor | 1.00 |

Those six end in storage containers. Everything else is an intermediate:

| Part (per minute) | Line | Machines | Clock | Feeds |
|---|---|---|---|---|
| Iron Ingot 93 | `iron_smelters` | 4 smelters | 0.775 | rods 61.5, plates 31.5 |
| Iron Rod 61.5 | `rods` | 5 constructors | 0.82 | screws 35.5, rotors 20, frames 6 |
| Screw 142 | `screws` | 4 constructors | 0.8875 | rotors 100, RIP 42 |
| Iron Plate 21 | `plates` | 2 constructors | 0.525 | RIP 21 |
| Reinforced Plate 3.5 | `rip` | 1 assembler | 0.70 | frames 1.5, smart plating 2 |
| Modular Frame 1 | `modular_frames` | 1 assembler | 0.50 | versatile framework 1 |
| Rotor 4 | `rotors` | 1 assembler | 1.00 | motors 2, smart plating 2 |
| Copper Ingot 88 | `copper_smelters` | 3 smelters | 0.978 | wire 68, sheet 20 |
| Wire 136 | `wire` | 5 constructors | 0.907 | cable 100, stators 36 |
| Cable 50 | `cable` | 2 constructors | 0.833 | automated wiring 50 |
| Steel Ingot 98.25 | `foundries` | 3 foundries | 0.728 | beams 78, pipes 20.25 |
| Steel Beam 19.5 | `beams` | 2 constructors | 0.65 | versatile framework 12, EIB 7.5 |
| Steel Pipe 13.5 | `pipes` | 1 constructor | 0.675 | stators 13.5 |
| Stator 4.5 | `stators` | 1 assembler | 0.90 | motors 2, automated wiring 2.5 |
| Concrete 15 | `concrete` | 1 constructor | 1.00 | EIB 15 |

Nodes: two iron, one copper, one limestone, two coal (all non-impure; a Mk1
miner on pure or a Mk2 on normal, both 120/min) and one lake. Power: eight
coal generators (600 MW) fed by one coal node and four water extractors, for
a nominal draw around 480 MW.

Fan-out is by plain splitters. An even split with back-pressure settles at
demand, so the ratios hold as long as each stream's supply covers its total
consumption, which the numbers above do.

## Layout

Two columns of `modules/manifold-line` stacked along +y from the anchor,
each line in a 48 m slot. The coal plant is a ninth line on the lake shore
with `has_output = false`, its generators fed by a splitter manifold like any
other row, water through one cross junction per extractor. Each miner gets a
`modules/pole-chain` from its node to the line it feeds; a trunk of poles
links the plant to the first line; from there the lines chain pole to pole.

`var.floor` (default off) lays 1 m foundations under both columns. It is
about 1,300 tiles; the API does not need them, you might.

## Before trusting it live: calibration

The game does not document which connector index is which, nor which way a
machine faces at yaw 0. The `local.cal` block at the top of `main.tf` holds
every such value with a note saying whether it was confirmed in a running
game ("live") or is the natural guess ("assumed"). Smelters, constructors,
splitters and mergers are live. Foundries, assemblers, coal generators,
water extractors, junctions, storage containers and miners are assumed.

To confirm them, build one of each by hand in the save with a belt, pipe
or wire attached, then ask the mod what it sees:

```sh
curl "http://localhost:8090/api/v1/world/buildables?x=<X>&y=<Y>&z=<Z>&radius=3000"
```

Each belt/pipe/wire entry carries `connects.from/to.connector` using exactly
the indices `satisfactory_belt`/`satisfactory_pipeline` take. Which way a
machine faces: place it at yaw 0 and look. Then edit `local.cal`. A wrong
index fails the apply with a 422; a wrong `side` applies fine and gives you
belts that wrap around the machine, which is what `preview = true` is for.

## Bootstrap

Miners need power before the generators have coal. `bootstrap_biomass_burner`
(default on) puts one biomass burner on the grid; fill it by hand once and
the plant carries itself from there. With the "No Power" advanced game
setting you can turn it off.

The API does not check milestones: a foundry applies whether or not you have
unlocked it.

## Applying live

```sh
export SATISFACTORY_PREVIEW=1   # see the whole thing as holograms first
terraform plan
terraform apply -parallelism=1  # every POST runs on the game thread
terraform plan -detailed-exitcode   # must be 0
```

`-parallelism=1` is caution, not a requirement; raise it if the game keeps
up. If a belt from a miner has to cross terrain the game refuses, put a
merger where you want it to bend and belt through that.

## Against the mock

The mock serves a fixed geography around the default anchor: enough iron,
copper, limestone and coal nodes, one lake, one node already taken. CI
applies this example against it like every other.
