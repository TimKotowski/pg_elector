# pg_elector

A Postgres based leader election library for Go. Using pure MVCC and lease expiration to coordinate leadership, no external consensus system required.

## Leadership guarantee

pg_elector provides a **strong single-leader guarantee** through lease-based election with Postgres as the source of truth.

The election clock enforces an invariant where the leader **always steps down before the lease expires in Postgres**. This means a new leader cannot be elected until the previous leader has already cancelled its work context and stopped operating. Under normal conditions, including Go's GC pauses, some transient network failures, and database timeouts, only one node is ever acting as leader.

### How it works

Each leader holds a lease with an `expires_at` timestamp. The leader periodically renews this lease. If renewal fails, a local deadline timer or harder checks other than deadline timer, ensures the leader resigns and cancels **before** the database lease expires. Extra steps are taken to ensure deadline timers is priority. For example, the Go runtime does not always guarantee that the renew timer (heartbeat to renew lease at short intervals) or deadline timer if both concurrently firing timers, will have its goroutine scheduled first. Check are done all all sides, for these contingencies, preserving a safe gap.

```
LeaderRetryPeriod < LeaderDeadline < LeaseDuration < ElectionInterval
```

- **LeaderRetryPeriod**: (heartbeat) How often the leader attempts to renew its lease.
- **LeaderDeadline**: If no successful renewal occurs within this window, the leader steps down immediately.
- **LeaseDuration**: How long the lease lives (TTL). Always longer than LeaderDeadline, so the leader resigns before anyone else can acquire.
- **ElectionInterval**: How often followers attempt to acquire leadership.

When leadership is lost, the context passed to every running task is cancelled immediately. Tasks that check `ctx.Done()` before committing work will stop promptly.

### The edge case

The only scenario where two leaders can briefly coexist is a **full process freeze**. Where container throttling, VM live migration, `SIGSTOP`, or work happening in the leader callback is causing swap thrashing, that lasts longer than the entire `LeaseDuration`. During a freeze, no Go code executes, timers don't fire, contexts don't cancel, and the leader cannot detect that its lease has expired. Meanwhile, Postgres clock continues, the lease expires, and another node acquires leadership. When the frozen process resumes, there is a brief window before the deadline check runs where stale work could execute.

This is not a clock skew issue or a GC issue. Go's garbage collector pauses are sub-millisecond and do not cause this. It requires the entire OS process to be suspended for longer than `LeaseDuration`, which is an extreme operational scenario. This is where fencing with terms is incorporated as a extra safety measure.

### Fencing with terms

For applications where even the process-freeze edge case is unacceptable, pg_elector exposes a **monotonic term** on the `ElectedLeader` object passed to every task.

```go
type ElectedLeader struct {
    Name     string
    LeaderID string
    Term     int64
}
```

The term increments atomically on every leadership change. It never decreases, never reuses a value, and is not affected by clocks or timing.

To achieve a **mathematically guaranteed single-leader** system, pass the term with every write to your downstream resources and have those resources reject any write carrying a term lower than the highest they have seen:

```go
Work In progress fencing client calls.
```

With term enforcement at the resource layer, a stale leader's writes are rejected regardless of timing, freezes, or detection lag. The context cancellation handles the common case fast; the term handles the edge case correctly.

### Summary

| Scenario | Single leader? |
|---|---|
| Normal operation | Yes |
| Go GC pauses | Yes |
| Transient network failures | Yes |
| Database timeouts with retry exhaustion | Yes |
| Full process freeze < LeaseDuration | Yes |
| Full process freeze > LeaseDuration | Yes, **if** term is enforced at the resource layer |

Without term enforcement, pg_elector provides a strong single-leader guarantee that covers all realistic operating conditions. With term enforcement, the guarantee is absolute
