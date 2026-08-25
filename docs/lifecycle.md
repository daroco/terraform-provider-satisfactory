# The full lifecycle: provider → mod → game world

This is the whole system end to end — how a line of HCL becomes a building in
a running game, how in-game changes come back as Terraform drift, and where
every hard-won subtlety lives. Names are exact so the diagrams double as a map
of the code. The provider is in this repo; the mod (`ASTFApiServerSubsystem`,
`ASTFRegistrySubsystem`) is in
[daroco/SatisfactoryTerraform](https://github.com/daroco/SatisfactoryTerraform).

## System at a glance

```mermaid
flowchart LR
    subgraph Local["Your machine"]
        direction LR
        TF["terraform CLI<br/>(state on disk)"]
        subgraph Prov["terraform-provider-satisfactory (Go)"]
            RES["4 resources<br/>foundation · building<br/>belt · power_line"]
            CLI["HTTP client<br/>Bearer + JSON"]
            RES --> CLI
        end
        TF -->|"plan / apply"| RES
        CLI -.->|"dev + CI, no game"| MOCK["mockserver (Go)<br/>same contract"]

        subgraph Game["FactoryGame process (UE) — SatisfactoryTerraform mod"]
            API["ASTFApiServerSubsystem<br/>HTTP listener :8090"]
            GATE{{"CheckRequest gate<br/>Host · Origin · Content-Type · token"}}
            REG["ASTFRegistrySubsystem<br/>3 maps, SaveGame"]
            WORLD["Game subsystems<br/>Buildable · Lightweight · Dismantle"]
            API --> GATE --> REG
            REG <--> WORLD
        end
        CLI -->|"HTTP/JSON, loopback only"| API
        REG <-->|"persisted with the world"| SAVE[("save game")]
    end

    CONTRACT["api/openapi.yaml — the single source of truth<br/>changed first, then client · mock · mod together"]
    CONTRACT -.-> CLI
    CONTRACT -.-> MOCK
    CONTRACT -.-> API
```

The contract (`api/openapi.yaml`) is the pivot: the Go client, the in-memory
mock, and the C++ mod are three implementations of it that must stay in
lockstep. The mock is why the whole provider can be built and CI-gated without
launching the game — and, this project having learned it the hard way, why the
mock *can't* catch bugs that only exist in the real game's object lifecycle.

## A request, end to end

One sequence, three acts: **create** (apply), **read** (plan / drift), and
**delete** (destroy or in-game dismantle). The notes mark the places where the
obvious implementation was wrong and the code does something specific instead.

```mermaid
sequenceDiagram
    autonumber
    participant TF as terraform
    participant P as provider (Go)
    participant API as mod API (ASTFApiServerSubsystem)
    participant R as registry (ASTFRegistrySubsystem)
    participant G as game world (UE subsystems)

    Note over TF,G: == ACT 1 - CREATE (terraform apply) ==
    TF->>P: Create(satisfactory_building)
    P->>P: assign tf_id (UUID), default clock_speed<br/>in Create, not schema (non-manufacturers omit it)
    P->>API: POST /api/v1/buildables<br/>Authorization: Bearer, Content-Type: application/json
    API->>API: CheckRequest: Host loopback? Origin absent?<br/>JSON? token? -> else 4xx
    API->>G: BeginSpawnBuildable(class, transform)
    API->>G: FinishSpawning(transform)
    Note over API,G: The game converts lightweight-eligible buildables<br/>(foundations, walls...) to a non-actor instance<br/>SYNCHRONOUSLY inside BeginPlay — already done<br/>by the time FinishSpawning returns. The mod must<br/>NOT convert too, or every tile doubles.
    alt manufacturer (constructor, smelter...)
        API->>G: SetRecipe / SetPendingPotential<br/>(after FinishSpawning — earlier silently no-ops)
        API->>R: Register(tf_id -> AFGBuildable*)
    else lightweight (actor was destroyed on convert)
        API->>R: RegisterLightweightByIdentity(tf_id, class, transform)<br/>resolves the game's own instance by class + location
    end
    R->>G: (state lives in the save game, SaveGame-flagged)
    API-->>P: 201 + JSON echo
    P->>P: absorb float32 round-trip noise<br/>(preserveWithinEpsilon) so yaw 90 != 89.9999... drift
    P-->>TF: resource in state (id = tf_id)

    Note over TF,G: == ACT 2 - READ (terraform plan -> drift detection) ==
    TF->>P: Read(id)
    P->>API: GET /api/v1/buildables/{tf_id}
    API->>R: Find(tf_id), else FindLightweight(tf_id)
    Note over R,G: FindLightweight re-resolves by class + location,<br/>NEVER trusts a cached ref (a removed instance's<br/>slot is recycled, not freed, so IsValid lies).<br/>Ambiguous or gone -> fail closed, a dead record<br/>is pruned so it can't resurrect onto a new tile.
    alt still present
        API-->>P: 200 + JSON
        P-->>TF: no change
    else dismantled in-game (404)
        API-->>P: 404
        P->>P: RemoveResource(state)
        P-->>TF: plan: 1 to add (recreate)
    end

    Note over TF,G: == ACT 3 - DELETE (destroy, or a tainted replace) ==
    TF->>P: Delete(id)
    P->>API: DELETE /api/v1/buildables/{tf_id}
    API->>R: Unregister(tf_id)<br/>also prunes connections that referenced it
    alt full actor
        API->>G: DismantleBuildable -> IFGDismantleInterface,<br/>but conveyor attachments Destroy() directly<br/>(their Dismantle_Implementation crashes the game)
    else lightweight
        API->>G: FLightweightBuildableInstanceRef::Remove()
    end
    API-->>P: 204
    P-->>TF: removed from state
```

### Connections are a fourth act

Belts and power lines (`POST /api/v1/connections`) spawn through the same
spline strategy the game's own placement uses, then are wired **into** the
chain — `source output → GetConnection0()` (belt input),
`GetConnection1()` (belt output) `→ destination input`, verified from both
ends. An earlier version wired the two machine connectors straight together
and left the belt dangling: items still moved, but the impossible
connector-to-connector state crashed the game on dismantle. The connection's
endpoints are recorded in a third registry map so the list endpoints can tell
a belt apart from a plain buildable.

## The security gate

Every request passes `CheckRequest` (or `CheckTransport` alone, for
`/health`) before its handler runs. The layers exist because the port is a
control plane — it can build and dismantle anything — and each stops a
different attacker.

```mermaid
flowchart TD
    REQ["incoming request"] --> HOST{"Host header<br/>= loopback?"}
    HOST -->|no| B403a["403 — host not allowed<br/>(defeats DNS rebinding)"]
    HOST -->|yes| ORIGIN{"Origin header<br/>present?"}
    ORIGIN -->|yes| B403b["403 — cross-origin<br/>(a browser tells on itself,<br/>the Go client never sends it)"]
    ORIGIN -->|no| CT{"mutating verb<br/>without<br/>application/json?"}
    CT -->|yes| B415["415 — forces a preflight<br/>this server never answers<br/>(blocks text/plain CSRF POST)"]
    CT -->|no| HEALTH{"/health?"}
    HEALTH -->|yes| OK["handler runs"]
    HEALTH -->|no| TOKEN{"token set?"}
    TOKEN -->|no| OK
    TOKEN -->|"yes & Bearer matches"| OK
    TOKEN -->|"yes & missing/wrong"| B401["401 — bad bearer token"]
```

Underneath all of that, the listener is pinned to `127.0.0.1` in `BeginPlay`
before it is created — FactoryGame overrides UE's `localhost` default to
`any`, so without the pin this binds to `0.0.0.0` and the gate above would be
the only thing between your factory and the LAN.

Loopback bind blocks the network entirely; Host + Origin + Content-Type block a
malicious web page (including via DNS rebinding); the optional bearer token
(`SATISFACTORY_TOKEN`, read from the environment at launch) is defense-in-depth
locally and the only guard for a tunnel forwarded to the loopback port.

## Why the mock can't find these

Nearly every subtlety the notes above call out — synchronous lightweight
conversion in `BeginPlay`, recycled instance slots, the conveyor-attachment
dismantle crash, the impossible connector state, float32 transform noise — is a
property of the *real game's* object lifecycle. The Go mockserver implements the
same wire contract faithfully and gates CI, but it models none of that runtime
behavior, so each of these was found only by driving the API against a running
game and watching the world. That live-verification loop is not a phase of the
project; it is the test suite for everything downstream of the HTTP boundary.
