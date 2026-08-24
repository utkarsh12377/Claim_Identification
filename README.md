# Product Claim Identification Workflow

A Go service that reads a product document, detects marketing and compliance
claims in it, classifies them into the thirteen assignment categories, and
returns the original product JSON enriched with a `claims` array.

Claim identification runs **asynchronously**: `POST /claims/identify` opens a
workflow and returns immediately, `GET /claims/status/{workflowId}` reports
progress and hands back the enriched product once the run completes.

---

## Quick start

```bash
go run ./cmd/server
```

The service listens on `:8080`, seeds the sample product from
`seed/product.json` into an in-memory store, and is ready to use. No database
or other infrastructure is required.

```bash
# 1. Trigger a run
curl -X POST http://localhost:8080/claims/identify \
     -H 'Content-Type: application/json' \
     -d '{"productId":"cfe6aa75-5da8-44f5-b587-56857841ad9f"}'
# {"workflowId":"wf-2f257001745c94a4","status":"IN_PROGRESS","productId":"cfe6aa75-..."}

# 2. Read the result
curl http://localhost:8080/claims/status/wf-2f257001745c94a4
```

With PostgreSQL (see [Persistence](#persistence)):

```bash
docker compose up -d
DATABASE_URL="postgres://claims:claims@localhost:5432/claims?sslmode=disable" go run ./cmd/server
```

`make help` lists the shortcuts for all of the above.

---

## Claims found in the sample product

| Claim value | Category | Found in |
|---|---|---|
| `High Proteins` | Nutritional Claims | `title` |
| `Source of Fibre & Iron` | Nutritional Claims | `aboutItems[2]` |
| `Safe for children` | Safety Claims | `shortDescription` |
| `Made in India` | Manufacturing / Origin | `complianceInfo.country_of_origin` |
| `FSSAI certified` | Certification Claims | `complianceInfo.fssai_no` |

Nothing else in the document is classified. In particular
`Made with 20 Spices & Herbs` is **not** an origin claim, `Masala slow roasted
to perfection` and `Appetizing Aroma & Delicious Taste` are not claims at all,
and `Iron 3.70mg` in the nutrition table is data rather than a claim.

The last two rows come from structured compliance fields rather than marketing
copy — see [Structured claims](#structured-claims) for the reasoning and the
switch that turns them off.

---

## API

### `POST /claims/identify`

Starts a claim-identification run.

```json
{ "productId": "cfe6aa75-5da8-44f5-b587-56857841ad9f" }
```

`202 Accepted`

```json
{
  "workflowId": "wf-2f257001745c94a4",
  "status": "IN_PROGRESS",
  "productId": "cfe6aa75-5da8-44f5-b587-56857841ad9f"
}
```

| Status | When |
|---|---|
| `202` | Run accepted and queued |
| `400` | Body is not JSON, or `productId` is missing/blank |
| `404` | No product with that ID |
| `503` | Worker queue saturated, or the service is shutting down — safe to retry |

### `GET /claims/status/{workflowId}`

Reports the state of a run. While in progress:

```json
{
  "workflowId": "wf-2f257001745c94a4",
  "status": "IN_PROGRESS",
  "productId": "cfe6aa75-5da8-44f5-b587-56857841ad9f"
}
```

Once complete, the full product comes back with claims appended (abridged):

```json
{
  "workflowId": "wf-2f257001745c94a4",
  "status": "COMPLETED",
  "productId": "cfe6aa75-5da8-44f5-b587-56857841ad9f",
  "steps": [
    { "name": "FETCH_PRODUCT",  "status": "SUCCEEDED", "durationMs": 0 },
    { "name": "DETECT_CLAIMS",  "status": "SUCCEEDED", "durationMs": 1, "detail": "5 claims identified" },
    { "name": "PERSIST_RESULT", "status": "SUCCEEDED", "durationMs": 0, "detail": "5 claims persisted" }
  ],
  "product": {
    "id": "cfe6aa75-5da8-44f5-b587-56857841ad9f",
    "title": "Maggi Nutri-licious Masala Veg Atta Noodles High Proteins Noodles",
    "...": "every other field exactly as received",
    "claims": [
      {
        "id": "9f1c2d84-8f0a-4c6e-9a2b-5d6e7f801234",
        "claimType": "Nutritional Claims",
        "claimValue": "Source of Fibre & Iron",
        "status": "IDENTIFIED",
        "source": "aboutItems[2]",
        "evidence": "Source of Fibre & Iron",
        "ruleId": "nutrition.source_of"
      }
    ]
  }
}
```

`id`, `claimType`, `claimValue` and `status` are the fields required by the
assignment. `source`, `evidence` and `ruleId` are additive and make every
detection auditable: which field it came from and which rule fired. A failed
run returns `"status": "FAILED"` with an `error` message and the step that
failed.

`404` is returned for an unknown workflow ID.

### Supporting endpoints

| Endpoint | Purpose |
|---|---|
| `GET /products/{productId}` | The stored product, including claims from the latest completed run |
| `GET /claims/categories` | The thirteen categories, flagged if legally restricted |
| `GET /claims/rules` | Detector rule metadata — which rules back which category |
| `GET /healthz` | Liveness plus store reachability |
| `GET /media/{file}` | Local product images (see [Product images](#product-images)) |

Every error response uses one envelope:

```json
{ "error": { "code": "PRODUCT_NOT_FOUND", "message": "no product with id nope" } }
```

Responses carry an `X-Request-Id` header (echoed from the request when
supplied) that matches the correlation ID in the server logs.

---

## How detection works

The detector is a rule catalogue in [`internal/claims/rules.go`](internal/claims/rules.go)
— 39 rules across the thirteen categories, each a set of regular
expressions traceable to one of the example claims in the assignment.

Three decisions do most of the work of keeping **false positives** out:

**1. Only marketing copy is scanned.** An allowlist of fields is examined:
`title`, `aboutItems`, `shortDescription`, `longDescription`. Ingredient lists,
nutrition tables, storage instructions and manufacturer addresses are never
scanned. Without this, `Fiber 5.0g, Iron 3.70mg` in `nutritional_details` would
produce nutritional "claims" out of what is really a data table.

**2. A trigger word alone never matches.** Rules pair a trigger with a term
from a domain vocabulary — nutrients, medical conditions, body functions,
countries, comparison baselines:

| Matches | Does not match | Why |
|---|---|---|
| `source of fibre and iron` | `a source of inspiration` | `source of` must be followed by a nutrient |
| `Made in India` | `Made with 20 Spices` | origin needs `made in` plus a place |
| `treats arthritis` | `delicious treats` | `treats` must be followed by a condition |
| `high protein` | `a high bowl of soup` | `high` must be followed by a nutrient |

Where a proper noun is the only signal — `Made in Kerala`, which no country
list can enumerate — the rule is case-sensitive, so `made in minutes` is
excluded.

**3. Negations are dropped.** A claim preceded by `not`, `never`, `without`,
`doesn't` and friends is discarded rather than reported.

Two further passes clean up the results:

- **Overlap resolution** — when several rules match the same span, the longest
  match wins, so `authentic recipe` is reported once as an Authenticity claim
  rather than twice.
- **Deduplication** — `Source of Fibre & Iron` (a bullet) and `is a source of
  fibre and iron` (the description) are the same claim. Comparison is done on
  a normalised key, and the wording from the highest-priority field is kept.
  Bullets outrank prose because they carry the canonical phrasing.

### Structured claims

Two claims on the sample product are not written in prose at all — they are
asserted by structured compliance data that ends up printed on the pack:

| Field | Claim | Category |
|---|---|---|
| `complianceInfo.country_of_origin: "India"` | `Made in India` | Manufacturing / Origin |
| `complianceInfo.fssai_no: "10012011000168"` | `FSSAI certified` | Certification Claims |

Both are example claims from the assignment's table, and both are true of this
product, so they are reported by default — with `source` naming the exact field
so a reviewer can see where each came from. Placeholder values (`N/A`, empty,
a licence number that is not a licence number) are rejected.

If you want claims made only in marketing copy, set
`CLAIMS_INCLUDE_STRUCTURED=false` and the sample product yields the three
text-derived claims instead.

### Restricted claims

`Medical / Therapeutic (Restricted)` claims are detected like any other, but the
workflow additionally logs them at `WARN` — a medical claim on a food label is
a compliance incident, not a routine detection.

---

## Workflow

```
POST /claims/identify
        │
        ├─ product missing ──────────────► 404, no workflow created
        │
        ▼
   [IN_PROGRESS] ──queued──► worker
        │                       │
        │            1. FETCH_PRODUCT
        │            2. DETECT_CLAIMS
        │            3. PERSIST_RESULT
        │                       │
        ├───────────────────────┼──► [COMPLETED]  product + claims
        └───────────────────────┴──► [FAILED]     error + failing step
```

- Transitions are enforced by `WorkflowStatus.CanTransitionTo`; terminal states
  are final, so a late or duplicated worker can never overwrite a finished run.
- Each step is timed and recorded on the workflow, so a failure points at the
  stage that produced it.
- Every run has a timeout (`WORKFLOW_TIMEOUT`, default 30s). A panic in a
  worker is recovered and recorded as a failed run rather than taking the
  process down.
- Completion is atomic: the enriched product, its claims and the run status are
  written in one transaction, so `COMPLETED` always implies the claims are
  durable.
- Shutdown is graceful: the HTTP listener stops first, then in-flight runs are
  drained.

The result returned by the status endpoint is a **snapshot** taken by that run.
A later run does not rewrite the history of an earlier one.

---

## Persistence

The store is an interface with two implementations, chosen by `DATABASE_URL`:

| `DATABASE_URL` | Store | Notes |
|---|---|---|
| unset | in-memory | Default. Zero setup; data is lost on restart |
| set | PostgreSQL | Schema applied automatically on start-up |

```bash
docker compose up -d      # postgres:16 on :5432
make run-postgres
```

Three tables ([`0001_init.sql`](internal/store/postgres/migrations/0001_init.sql)):

- **`products`** — the full product document as `JSONB`, plus extracted columns
  (`title`, `brand`, `mcr_id`) for lookups.
- **`workflows`** — one row per run: status, error, the step audit trail, and
  the enriched product snapshot.
- **`claims`** — claims stored relationally as well as inside the product JSON,
  so they can be queried across the catalogue:

  ```sql
  SELECT claim_type, count(*) FROM claims GROUP BY claim_type ORDER BY 2 DESC;
  ```

Migrations are embedded in the binary and idempotent, so start-up is safe to
repeat. Optimistic concurrency is handled in SQL: closing a run updates
`WHERE status = 'IN_PROGRESS'`, and zero affected rows is reported as an
invalid transition.

---

## Product images

The `media[].url` values in the supplied product are time-limited signed Google
Storage links and have expired. The service therefore serves images itself:

- Files live in `assets/images/` and are served at `GET /media/{file}`.
- At seed time each `media[].url` is rewritten to
  `http://localhost:8080/media/{file}` (`PUBLIC_BASE_URL`, disable with
  `REWRITE_MEDIA_URLS=false`). `media[].path` is left untouched.
- `go run ./cmd/genmedia` generates a placeholder JPEG per media entry so the
  endpoint works out of the box; both are committed.

To use real photographs, drop them into `assets/images/` under the same file
names — no code or configuration change needed.

The expired `?Expires=…&Signature=…` query strings were trimmed from
`seed/product.json`; every other byte of the supplied document is unchanged.

---

## Configuration

All settings are environment variables with working defaults — see
[`.env.example`](.env.example).

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `:8080` | Listen address; a bare port number also works |
| `DATABASE_URL` | *(empty)* | PostgreSQL DSN; empty means in-memory |
| `SEED_FILE` | `seed/product.json` | Product document loaded at start-up |
| `MEDIA_DIR` | `assets/images` | Directory served at `/media/` |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Origin used when rewriting media URLs |
| `REWRITE_MEDIA_URLS` | `true` | Replace expired catalogue URLs with local ones |
| `CLAIMS_INCLUDE_STRUCTURED` | `true` | Detect claims from compliance fields too |
| `WORKFLOW_WORKERS` | `4` | Worker pool size |
| `WORKFLOW_QUEUE_SIZE` | `64` | Queue depth before `503` |
| `WORKFLOW_TIMEOUT` | `30s` | Per-run timeout |
| `SHUTDOWN_TIMEOUT` | `15s` | Grace period for draining |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Invalid values fail at start-up with a message naming the variable.

---

## Project layout

```
cmd/server           service entrypoint: config, wiring, graceful shutdown
cmd/genmedia         generates local placeholder images for product media

internal/model       Product, Claim, Category, Workflow + the state machine
internal/claims      the detector: rule catalogue, matching, structured fields
internal/workflow    async engine: worker pool, step audit trail, transitions
internal/store       store interface + memory and postgres implementations
internal/httpapi     router, handlers, middleware, error envelope
internal/config      environment configuration
internal/seed        loads the sample product at start-up
internal/uid         UUID / workflow ID generation
```

Dependencies point inwards: `httpapi → workflow → store/claims → model`. The
detector knows nothing about HTTP or SQL, so its rules are testable in
isolation.

The only third-party dependency is `github.com/jackc/pgx/v5`, used for
PostgreSQL. HTTP routing uses the standard library's `net/http` method-and-
wildcard patterns; UUIDs are generated from `crypto/rand`.

---

## Testing

```bash
go test ./...
```

| Area | What is covered |
|---|---|
| `internal/claims` | Golden test pinning the exact claims for the sample product; a false-positive suite over the product's own copy and lookalike phrasings; every example claim from the assignment's table mapped to its category; dedupe, overlap, negation, placeholder rejection |
| `internal/workflow` | Completion, failure recording, queue saturation, shutdown behaviour, double-close rejection, state machine |
| `internal/httpapi` | Trigger → poll → enriched product; request validation; 404/405 handling; product round-trip fidelity; eight concurrent runs |

The race detector (`go test -race`) needs cgo and a C toolchain, which is not
installed on this machine, so it has not been run here.

---

## Notes and trade-offs

- **Rules, not an LLM.** The assignment asks for detection "only from the above
  example patterns", and rules give deterministic, explainable, testable output
  — each claim names the rule that produced it. The cost is that novel phrasings
  need a new rule; `internal/claims/rules.go` is a declarative table designed
  for exactly that.
- **Claim status is always `IDENTIFIED`.** Detection asserts that a claim *is
  made*, not that it is *substantiated*. Verification is the natural next state
  in this model.
- **Products are read, not written, by clients.** There is no product-ingest
  endpoint; the sample product is seeded from disk. Adding one would be a thin
  handler over `store.UpsertProduct`.
- **One product per run.** Batch identification across a catalogue would reuse
  the same engine with a fan-out job type.
