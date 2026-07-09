# Design Notes — Internal Transfers System

Audience: a reviewer reading the code, and a CTO deciding whether this scales to a licensed
payment institution. This documents the load-bearing decisions, the honest limits of the
simple version that ships here, and the production direction with tradeoffs. It is deliberately
not marketing — where the simple choice is a placeholder, it says so.

The system is a small Go service (chi → service → repo → Postgres) exposing account creation,
balance reads, and transfers, with money held as `NUMERIC(20,5)` and moved inside a single
serialized database transaction.

---

## 1. Concurrency — READ COMMITTED + `FOR UPDATE` + deterministic lock ordering

**Choice.** Each transfer runs in one Postgres transaction at the default **READ COMMITTED**
isolation. It locks *both* account rows with `SELECT … FOR UPDATE` before reading balances,
and it always locks the **lower `account_id` first**. Under those locks it does the
check-funds → debit → credit → insert-ledger-row sequence, then commits. Correctness rests on
three things working together:

- `FOR UPDATE` gives a row-level write lock, so two transfers touching the same account
  serialize on that row — no lost updates, no torn read-modify-write.
- The funds check reads the *locked* balance, so check-then-debit is atomic per account (no
  time-of-check/time-of-use race that could overdraw).
- Deterministic lock ordering means concurrent transfers can never acquire the same two rows in
  opposite orders, so they cannot form a lock cycle → no deadlock between transfers.

This is proven by an integration test that runs 200 concurrent random transfers (asserting
conservation and no negative balances) plus a tight A→B / B→A contention test that would hang
without the ordering.

**Why this over SERIALIZABLE.** SERIALIZABLE (SSI in Postgres) protects against anomalies that
arise when a transaction makes a decision over a *set* or *range* of rows that another
transaction changes — write skew, phantoms. A transfer here makes no such decision: it touches
exactly two explicitly named, explicitly locked rows and decides based only on those. There is
no range predicate to be invalidated. So SERIALIZABLE would add serialization-failure retries
and overhead to buy a guarantee we already get, more cheaply, from two row locks. For this
operation, explicit pessimistic locking is both simpler to reason about and cheaper.

**Limitation of the simple version.** Pessimistic row locks serialize all activity on a hot
account. If one account is a party to a very high rate of concurrent transfers (a merchant
settlement account, a house/float account), every transfer queues on that single row and
throughput collapses to serial. The lock is correct but it is a contention bottleneck.

**Production evolution & tradeoffs.**
- **Where SERIALIZABLE wins:** the moment we add a rule spanning multiple rows — "total exposure
  across a customer's accounts must stay under a limit," "daily outbound volume cap" — a
  READ COMMITTED check is racy (two transfers each read a stale aggregate and both pass).
  There we either take an explicit lock over the predicate (a limits row we `FOR UPDATE`) or
  move that operation to SERIALIZABLE and handle 40001 retries. SERIALIZABLE earns its cost
  exactly when the decision is over a set, not a pair of named rows.
- **Where optimistic locking wins:** under high contention on hot rows, pessimistic locks
  serialize even non-conflicting work and hold locks across the whole transaction. An optimistic
  scheme — read balance + version, compute, `UPDATE … WHERE version = $read` , retry on
  mismatch — holds no lock while computing and lets independent transfers proceed; it trades
  lock-wait latency for retry-on-conflict. It shines when conflicts are *rare* but concurrency
  is high. On a genuinely hot account it degrades (everyone retries), so the real answer for a
  hot account is not a locking-strategy tweak but a data-model change: represent balance as an
  append-only sum of ledger entries (see §2) so transfers *insert* rather than *update-in-place*
  and stop contending on one mutable row.
- **Contention mitigation before any of that:** balance sharding (split a hot account into N
  sub-balances and pick one per transfer), or moving hot-path settlement to an async,
  event-sourced posting model with eventual balance projection.

---

## 2. Data model — single-row transaction log → double-entry ledger

**Choice (today).** `accounts(account_id, balance, …)` holds a mutable balance; `transactions`
records one row per transfer `(id, source_account_id, destination_account_id, amount, created_at)`
with FKs to accounts and `CHECK (amount > 0)`. A transfer mutates two balances and appends one
transaction row, atomically.

**Limitation of the simple version.** The mutable `balance` column is the source of truth, and
the transaction log is a side record. That has three problems for a regulated payments context:

1. **Balances aren't derivable.** You can't independently *recompute* a balance from history to
   check the stored one — there's no per-account movement record, only a net counter that was
   overwritten in place. Reconciliation ("does the ledger sum to the balance?") is not a query
   you can run.
2. **It models only a two-party, single-leg transfer.** Fees, FX legs, holds/authorizations,
   reversals, and chargebacks don't fit — they're multi-leg movements, and there's nowhere to
   put the extra legs.
3. **Corrections are mutations.** Fixing a bad balance means an `UPDATE`, which destroys the
   prior state. Auditors and regulators expect an immutable trail.

**Production direction (stated plainly: this is where it should go).** A **double-entry ledger**:

- An append-only `entries` table: one row per account *leg* of a movement —
  `(id, transaction_id, account_id, direction ∈ {debit, credit}, amount, created_at)`. A simple
  transfer becomes two entries sharing one `transaction_id` (debit source, credit destination);
  the invariant is that **debits equal credits for every `transaction_id`** (money is
  conserved by construction, enforceable with a constraint/trigger).
- Entries are **never updated or deleted**. A correction is a new, opposing set of entries
  (a reversal), so the full history is preserved.
- **Balance becomes a projection**, not a source of truth: `balance(account) = Σ credits − Σ
  debits`. In practice you keep a cached balance (materialized/rollup, updated in the same
  transaction as the entries or via periodic rollup) *and* can always rebuild it from entries.

**Why it matters at a licensed PI.** Auditability and reconciliation stop being hopes and become
properties of the schema. You can re-derive every balance from an immutable log, prove money was
conserved on every transaction, produce point-in-time balances for any date, and represent the
real product surface (fees, holds, multi-currency, reversals) without schema gymnastics. The
tradeoff is more rows and more read cost for a current balance (mitigated by the cached rollup),
plus the discipline that nothing in the money path ever mutates history. That cost is the point:
immutability is the feature.

I did not build double-entry here because this is a two-party transfer and single-row is
the honest minimal model — but it is a placeholder, and this section is the migration path.

---

## 3. Money — `NUMERIC(20,5)` + `decimal` + string I/O; never float

**Choice.** Amounts are `NUMERIC(20,5)` in Postgres, `shopspring/decimal` in Go, and **strings**
on the JSON boundary (`"100.50000"`). Every write and every response is normalized with
`StringFixed(5)`, so scale is exact and uniform. Input is parsed and validated as a string
(rejecting empty, scientific notation, and >5 decimal places) before it ever becomes a number.

**Why never float.** IEEE-754 binary floating point cannot represent most decimal fractions
exactly — `0.1 + 0.2 ≠ 0.3`. Over millions of operations those representation errors accumulate
and balances drift by cents, which is both wrong and, for a regulated entity, a compliance
failure. Money must use a base-10 exact type end to end; a single `float64` anywhere in the path
(parse, arithmetic, serialization) reintroduces the drift. Strings on the wire also stop a JSON
client from silently parsing the amount into a float on *their* side.

**Limitation of the simple version.** `NUMERIC(20,5)` fixes scale at 5 and has no notion of
currency; there's no upper-bound guard, so an absurdly large amount that overflows the column
surfaces as a 500 rather than a clean 400.

**Production evolution & tradeoffs.** Add an explicit currency and per-currency scale (JPY has
0 decimals, most have 2, crypto more), and consider storing **integer minor units** (`bigint` of
the smallest unit) instead of `NUMERIC` — integers are unambiguous about scale and faster, at
the cost of readability and a mandatory currency+scale lookup to interpret them. Both are valid;
`NUMERIC(20,5)`+decimal was chosen here for clarity against a fixed 5-dp contract. Also add the
input upper-bound check so overflow is a 400, not a 500.

---

## 4. Error model — sentinel errors per layer, mapped outward

**Choice.** Each layer owns a small set of sentinel errors and translates as errors move up:

- **repo** returns storage-meaningful sentinels: `ErrAccountNotFound`,
  `ErrAccountAlreadyExists`, `ErrInsufficientFunds`, `ErrInvalidAmount`, `ErrSameAccount`,
  `ErrIdempotencyKeyConflict` (Postgres specifics like unique-violation `23505` are detected
  here and never leak upward).
- **service** maps those to domain sentinels: `ErrInvalidInput`, `ErrNotFound`, `ErrConflict`,
  `ErrInsufficientFunds`.
- **handlers** map domain sentinels to HTTP: 400 `invalid_input`/`invalid_json`, 404
  `not_found`, 409 `conflict`/`insufficient_funds`, 500 `internal_error`.

Mapping happens in exactly one place per boundary (`mapRepoError`, `mapError`), and comparisons
use `errors.Is` so wrapped errors still classify correctly.

**Why.** The client gets a small, stable vocabulary of error codes that is decoupled from
internal detail — we can change the database or refactor the repo without changing the API
contract, and internal specifics (SQLSTATEs, driver errors) never reach the client. Anything
unmapped deliberately becomes a 500 rather than being guessed at.

**Limitation of the simple version.** Errors are flat codes with no machine-readable detail
(no field-level validation errors, no correlation id in the body, no problem+json structure).

**Production evolution & tradeoffs.** Adopt a structured error envelope (e.g. RFC 7807
`application/problem+json`) with a stable `type`, a request/correlation id, and field-level
detail for validation; keep the sentinel-per-layer mapping underneath. The tradeoff is a
slightly heavier contract for much better client ergonomics and supportability.

---

## 5. Idempotency — optional, atomic, opt-in

**Choice.** `POST /transactions` accepts an optional `Idempotency-Key` header. Without it,
behavior is unchanged (204, spec preserved). With it, the transfer is deduplicated: the key
lookup, the transfer, and the key insert all happen **in the same database transaction** as the
money movement, so the dedup decision and the debit/credit commit or roll back together. A
repeat key with the same payload returns the original transaction id (no money moved); a repeat
key with a different payload returns 409. The created transaction id is returned in the response
so retries are verifiable and the ledger is auditable.

**Why it's here.** Retries are unavoidable in payments — a client that times out cannot tell
"never happened" from "happened but I didn't hear back," so it *must* retry, and a naive retry
double-applies the transfer. Idempotency is the standard remedy and is cheap: one table, one
optional header, an additive code path. It's slightly beyond the minimal spec; I added it
because omitting it would be a known correctness gap in the one operation that moves money.

**Limitation of the simple version.** Keys never expire (no TTL/GC) and aren't scoped to a
caller; we compare payloads via the transaction the key produced rather than storing the exact
original response.

**Production evolution & tradeoffs.** Scope keys per API client, add `created_at`-based expiry
and a reaper, and optionally store a response fingerprint to replay the *exact* original response
(status + body) on retry. Under concurrency the current design already rolls back the losing
racer and returns the winner's id via the unique-key constraint; at scale you'd also add a short
lock/`ON CONFLICT` fast path to avoid the wasted transfer attempt on the loser.

---

## 6. If I had another day

- **Double-entry ledger (§2)** — the highest-value change; convert `transactions` into
  append-only `entries` with a debit=credit invariant and balance-as-projection, with a
  reconciliation job.
- **Switch balance reads in the transfer to `FOR NO KEY UPDATE`** (see Q&A) after measuring, to
  reduce contention with FK-referencing inserts.
- **Structured errors (problem+json)** with correlation ids threaded through slog.
- **Input hardening:** amount upper-bound → 400; per-currency scale.
- **Observability:** request metrics, DB pool metrics, a `/health` that checks DB readiness, and
  tracing across the handler→repo→DB path.
- **CI with `-race` and the integration suite** wired to a disposable Postgres, so the
  concurrency guarantees are enforced on every push rather than run by hand.
- **Idempotency key expiry + per-client scoping.**

---

## Q&A — questions a senior reviewer would ask

**Q1. What happens if two transfers deadlock?**
Between transfers, they can't: both lock the lower `account_id` first, so there's no cycle. If a
deadlock ever did occur — say a future code path locked in a different order — Postgres detects
it, aborts one side with SQLSTATE `40P01`, and `TransferTx` returns that as a wrapped error; the
deferred `Rollback` undoes every change, so no money moves, and the caller gets a 500 that is
safe to retry (idempotently, if they sent a key). The ordering is what turns "handle deadlocks"
into "prevent them."

**Q2. Why not `SELECT … FOR NO KEY UPDATE`?**
Good catch — it's arguably the more precise lock and I'd move to it in production after
measuring. We only modify `balance`, never the key `account_id`, and `FOR NO KEY UPDATE` still
self-conflicts, so two concurrent transfers on the same account still serialize correctly. Its
advantage is that it does *not* block `FOR KEY SHARE`, which is the lock a row acquires when
another transaction inserts a `transactions` row that FK-references this account. So under heavy
transfer load, `FOR NO KEY UPDATE` lets those FK-referencing inserts proceed instead of queuing
behind us. I used `FOR UPDATE` as the conservative, obviously-correct superset lock; the switch
is a contention optimization, not a correctness fix, so I'd make it with a benchmark in hand.

**Q3. How do you handle a partial failure between the debit and the credit?**
There is no partial state to handle — the debit, the credit, and the ledger insert are one
database transaction. Either all three commit or the deferred `Rollback` (or a crash /
connection loss) discards all three. That's the whole reason the money movement is a single DB
transaction rather than two service calls: two calls could leave money debited but not credited;
one transaction cannot. If the process dies after `COMMIT` but before the client hears back, the
transfer *did* happen, the client retries, and the idempotency key returns the original id
instead of moving money again.

**Q4. Why READ COMMITTED and not SERIALIZABLE — aren't you exposed to anomalies?**
Not for this operation. Every balance we act on is read under `FOR UPDATE`, so we see the latest
committed value and hold it until commit; there's no range/aggregate predicate that a concurrent
writer could invalidate, so no write skew or phantoms. SERIALIZABLE defends against exactly those
set-based anomalies, which we don't have here — it would add retry overhead for no gain. The day
we add a rule over multiple rows (exposure limits, velocity caps), that decision *is* set-based,
and I'd protect it with an explicit lock on the predicate row or bump that path to SERIALIZABLE.

**Q5. Why is the funds check inside the transaction rather than before it?**
Because it reads the locked balance. If I checked before acquiring the lock, another transfer
could drain the account between my check and my debit — a classic time-of-check/time-of-use race
that overdraws. Inside the `FOR UPDATE` lock, check-then-debit is atomic on that row. As a
backstop, the `CHECK (balance >= 0)` constraint means even a logic bug can't persist a negative
balance — the transaction aborts.

**Q6. How would you shard this?**
Shard accounts by `account_id` (hash or range). The hard part is that a transfer spans two
accounts that may land on different shards, turning a local transaction into a distributed one.
Three honest options, in order of how much I'd resist them: (a) colocate related accounts (by
customer or region) so most transfers stay single-shard; (b) a saga/2PC with a transfer
coordinator for genuinely cross-shard moves, accepting the added latency and failure modes; (c)
move to an event-sourced ledger where a transfer is an atomic append to a partitioned log with
asynchronous balance projection. But I'd exhaust vertical scaling, read replicas, and table
partitioning on a single primary first — sharding trades our clean per-row serializability for a
distributed-commit problem, and you don't take that on until one primary is truly saturated.

**Q7. Your balance is a mutable column. How do you know it's right?**
Today, you largely have to trust it — that's the core limitation in §2. There's no per-account
movement record to re-sum, so reconciliation isn't a query. That's precisely why the production
direction is double-entry: make `balance` a projection of an immutable `entries` log so it's
*derivable* and *verifiable*, and money conservation (debits = credits per transaction) is a
schema invariant rather than an application hope.

**Q8. Why NUMERIC + decimal instead of integer minor units?**
Both are correct; neither is float. Integer minor units (store cents/satoshis as `bigint`) are
unambiguous about scale and faster, but require a currency+scale lookup to interpret and are less
readable in the DB. `NUMERIC(20,5)` + `shopspring/decimal` gave exact base-10 arithmetic against
a fixed 5-dp contract with readable storage, which fit this exercise. For multi-currency at scale
I'd seriously consider integer minor units with an explicit per-currency scale. The
non-negotiable part is only that it's never `float`.

**Q9. What exactly does the idempotency key protect, and what are its limits?**
It guarantees that a retried transfer with the same key never applies twice: the key is claimed
in the same transaction that moves the money, so the two commit atomically; same key + same
payload returns the original transaction id, different payload returns 409. Limits: keys don't
expire and aren't scoped per caller yet, and I compare payloads via the produced transaction
rather than storing the exact original response body. Production adds per-client scoping,
TTL+GC, and a stored response fingerprint for exact replay.

**Q10. A client sends the same brand-new key on two requests at the exact same time. What happens?**
Both pass the initial "key not found" lookup and attempt the transfer, serializing on the account
row locks. The first to reach the key `INSERT` wins and commits. The second hits the primary-key
unique violation, **rolls back its own transfer** (so no money moved), then re-reads the key and
returns the winner's transaction id. So even a dead heat applies the money exactly once. The
wasted transfer attempt on the loser is the cost; a production fast path would claim the key up
front (or `INSERT … ON CONFLICT`) to avoid doing the transfer work twice.

**Q11. Why hold the balance check, update, and ledger insert on one connection — could you pipeline or batch?**
They must share one transaction and therefore one connection, because the locks and the atomic
commit are properties of that transaction. Notably, a transfer never needs a *second* connection
mid-transaction, which is what keeps it safe from a pool-exhaustion deadlock (a transaction
holding a row lock while waiting for a connection that another lock-holder occupies). Batching
would only apply across independent transfers, and there the row locks already give us the
serialization we need.
