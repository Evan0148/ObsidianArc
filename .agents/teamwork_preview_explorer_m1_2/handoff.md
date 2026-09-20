# Security Audit Handoff Report: Dimensions 2 & 3

**Auditor**: Explorer 2 (Security Audit Team)  
**Assigned Dimensions**:
- **Dimension 2**: Concurrency Control & Database Consistency
- **Dimension 3**: Input Validation & Injection
**Working Directory**: `E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_2/`  
**Date**: September 2026  
**Status**: Complete (Hard Handoff)

---

## 1. Observation

Direct code observations across all evaluated subsystems:

### 1.1 Concurrency Control & Database Invariants
1. **`internal/conversation/conversation.go:475-479`**:
   ```go
   func (s *Store) appendIn(ctx context.Context, q database.Queryer, in AppendInput) (Message, error) {
   	if _, err := q.Exec(ctx,
   		`UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?`,
   		in.ConversationID, in.UserID); err != nil {
   		return Message{}, fmt.Errorf("conversation: lock for append: %w", err)
   	}
   ```
   `q.Exec` result is discarded (`_, err := ...`). No check on `RowsAffected()` is performed. If `in.ConversationID` does not belong to `in.UserID`, zero rows are updated, no error is returned, and execution falls through to calculate `MAX(seq)` and insert a message referencing another user's conversation.
   *Contrast*: `internal/apikey/apikey.go:135-137` explicitly checks:
   ```go
   locked, err := tx.Exec(ctx, `UPDATE users SET updated_at = updated_at WHERE id = ?`, userID)
   if affected, rowsErr := locked.RowsAffected(); rowsErr == nil && affected == 0 {
   	return ErrNotFound
   }
   ```
   and `internal/conversation/attachment.go:138-140` checks:
   ```go
   if affected, rowsErr := locked.RowsAffected(); rowsErr == nil && affected == 0 {
   	return fmt.Errorf("conversation: attachment owner does not exist")
   }
   ```

2. **`internal/server/bootstrap.go:43-69` vs `107-120`**:
   In `ensureAdmin` (lines 107-120):
   ```go
   return db.Tx(ctx, func(tx *database.Tx) error {
   	if err := settings.Lock(ctx, tx); err != nil {
   		return err
   	}
   	if again, err := users.Count(ctx, tx); err != nil {
   		return err
   	} else if again > 0 {
   		return nil
   	}
   ```
   Comment notes: *"A plain count inside a transaction reads what is committed and blocks nobody, so under Postgres' default isolation both would still see zero and both would insert."*
   However, in `ensureGroup` (lines 43-69):
   ```go
   func ensureGroup(ctx context.Context, db *database.DB, groups *group.Store) error {
   	return db.Tx(ctx, func(tx *database.Tx) error {
   		count, err := groups.Count(ctx, tx)
   		if err != nil {
   			return err
   		}
   		if count > 0 {
   			return nil
   		}
   		created, err := groups.Create(ctx, tx, group.CreateInput{
   			Name: "Default", ...
   ```
   `settings.Lock(ctx, tx)` is omitted. Under Postgres Read Committed isolation, two concurrent instances booting on an empty database both count 0 and attempt `groups.Create("Default")`, where the second hits `ux_user_groups_name` unique constraint violation and crashes.

3. **`internal/database/migrate.go:34-63`**:
   ```go
   func (db *DB) Migrate(ctx context.Context) ([]string, error) {
   	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations ...`); err != nil {
   		return nil, fmt.Errorf("database: create schema_migrations: %w", err)
   	}
   	done, err := db.appliedVersions(ctx)
   	...
   ```
   No inter-process locking or PostgreSQL advisory lock (`pg_advisory_lock`) is acquired before checking `appliedVersions` and applying migrations. If two containers start simultaneously, both apply migration `0001` concurrently, resulting in `relation already exists` DDL errors or duplicate key violations on `schema_migrations`.

4. **`internal/quota/service.go:356-363`**:
   ```go
   // RecordRejection counts a refused request against the rate window only, so a
   // client retrying in a loop still runs into the rate limit rather than being
   // free to hammer the endpoint.
   func (s *Service) RecordRejection(ctx context.Context, userID string) {
   	now := time.Now()
   	// The rate window only, which is wall-clock, so this one needs no anchor.
   	_, _ = bump(ctx, s.db, scopeKey(userID), WindowRPM, bucketStart(WindowRPM, now, 0), 1, 0, 0)
   }
   ```
   `RecordRejection` is **never referenced or called anywhere else in the entire repository** (`grep_search` yields exactly 1 match: its own definition). In `internal/server/server.go:191-197`, when `quotaService.Reserve` fails due to quota exhaustion, the transaction rolls back (rolling back the RPM increment), and `RecordRejection` is not called, leaving the RPM rate limit un-bumped on quota-exceeded requests.

5. **`internal/server/server.go:200-211, 249-256` and `internal/chat/http.go:212`**:
   In `server.go:253`:
   ```go
   actual := quota.Estimate{Tokens: int64(record.Usage.Total()), Credits: record.Credits}
   if err := quotaService.Settle(ctx, record.User, quota.Estimate{}, actual); err != nil {
   	slog.ErrorContext(ctx, "could not settle quota", "error", err, "user", record.User.ID)
   }
   ```
   `Settle` is invoked with `reserved = quota.Estimate{}` (empty), adding `+actual.Tokens` to `usage_counters`. The worst-case reservation (`reserved.estimate`, e.g. 4096 tokens) is refunded separately in `defer release()` (`internal/chat/http.go:212`). Because `OnTurn` (`Settle`) runs before `Run` returns and before the HTTP handler exits, `usage_counters` temporarily holds `Reservation + Actual` (e.g. 4096 + 500 = 4596 tokens), temporarily inflating usage and triggering false HTTP 429 quota rejections for rapid subsequent turns or parallel tabs.

6. **`internal/card/http.go:60-82`**:
   ```go
   func (h *Handlers) use(w http.ResponseWriter, r *http.Request) error {
   	account := auth.MustUser(r.Context())
   	cardID := r.PathValue("id")
   	...
   	if err := h.store.Spend(r.Context(), account.ID, cardID); err != nil {
   		return translate(err)
   	}
   	if h.OnSpend != nil {
   		if err := h.OnSpend(r.Context(), account); err != nil {
   			return httpx.Internal(err)
   		}
   	}
   	return httpx.NoContent(w)
   }
   ```
   `Spend` (which executes `UPDATE usage_cards SET used_at = now`) and `OnSpend` (which invokes `quotaService.Reset`) run outside a transaction. A transient error or crash between line 73 and line 77 permanently consumes the card without resetting quota.

7. **`internal/admin/feedback.go:73` and `internal/feedback/http.go:142`**:
   ```go
   if err := h.feedback.MarkSeen(context.WithoutCancel(r.Context()), thread.Feedback.ID, true); err != nil {
   ```
   `context.WithoutCancel(r.Context())` is passed with no timeout deadline. A database lock stall or hanging connection will block the query and retain the connection pool slot indefinitely.

8. **`internal/settings/settings.go:5-7, 395-417`**:
   `settings.Service` caches configuration in `s.values map[string]string`. Changes via `SetMany` update the local instance memory cache only. There is no cache invalidation or polling for other instances sharing the database.

### 1.2 Input Validation, Injection & Filesystem Safety
1. **SQL Parameterization**:
   All database queries across `internal/` utilize `?` positional parameters via `database.Queryer` and `database.Rebind`. No raw user input is concatenated into query strings.
2. **SQL LIKE Query Escaping**:
   `internal/user/user.go:569-573` and `internal/reqlog/query.go:94-99` sanitize `search` inputs with `escapeLike` (`\\`, `\%`, `\_`) and specify `ESCAPE '\'`. In contrast, `internal/feedback/feedback.go:344` omits `escapeLike`, causing raw `_` and `%` characters to be interpreted as SQL wildcards.
3. **Attachment & File Storage**:
   `internal/conversation/attachment.go` stores uploaded image attachments exclusively in the database `attachments` table as binary BLOBs (`data %BLOB% NOT NULL`). Zero filesystem path operations are involved in attachment upload, storage, or retrieval.
4. **Virtual Filesystem**:
   Frontend static assets are embedded into the Go binary via `embed.FS`. Request paths are sanitized with `path.Clean("/" + r.URL.Path)` in `internal/web/web.go`, preventing directory traversal.
5. **SSRF & DNS-Rebinding Hardening in Image Generation Delivery**:
   `internal/chat/imagefetch.go` rigorously protects image URL fetching via `publicAddrFor`, validating that all resolved IP addresses are public, rejecting loopback, RFC 1918, link-local, and ULA addresses, and pinning the resolved IP in `http.Transport.DialContext` without following redirects.
6. **Payload Boundaries**:
   All JSON handlers use `httpx.DecodeJSON` with `http.MaxBytesReader` bounding body sizes (4 KiB - 256 KiB, attachments up to ~8 MiB), strict unknown field rejection (`DisallowUnknownFields`), and single JSON object validation (`decoder.More()`).

---

## 2. Logic Chain

### Logic Chain 1: Conversation Message Injection & Lock Bypass (`conversation.Store.appendIn`)
1. Observation 1.1.1 shows `appendIn` executes `UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?` but discards the result without checking `RowsAffected()`.
2. When a user supplies a `ConversationID` belonging to another user, 0 rows match and 0 rows are updated; no row lock is acquired on `conversations`.
3. `appendIn` proceeds to `SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = ?`, which queries the target conversation without filtering by `user_id`, finding the next sequence number.
4. `INSERT INTO messages` executes with `conversation_id = target_conv` and `user_id = attacker_id`. The database schema foreign keys check `conversation_id REFERENCES conversations(id)` and `user_id REFERENCES users(id)` individually, which both succeed.
5. `touch` updates `message_count` based on `SELECT COUNT(*) FROM messages WHERE conversation_id = ?`.
6. Therefore, an attacker can inject foreign messages into another user's conversation, consuming sequence positions and desynchronizing `message_count`.

### Logic Chain 2: Multi-Node Startup Crash in `ensureGroup`
1. Observation 1.1.2 shows that `ensureAdmin` takes `settings.Lock(ctx, tx)` to prevent concurrent instances from both seeing count = 0.
2. In `ensureGroup`, `settings.Lock(ctx, tx)` was omitted.
3. Under PostgreSQL default `Read Committed` isolation level, two application instances starting simultaneously both execute `groups.Count` before either has committed an insert; both observe `count == 0`.
4. Both instances attempt `INSERT INTO user_groups (..., name, ...)` with name `"Default"`.
5. Table `user_groups` enforces `CREATE UNIQUE INDEX ux_user_groups_name ON user_groups (name)`. The second transaction fails with duplicate key error.
6. `server.New` returns an error, causing the second instance container to exit with code 1.

### Logic Chain 3: Multi-Node DDL Conflict in `database.Migrate`
1. Observation 1.1.3 shows `Migrate` reads `appliedVersions` and applies pending migrations sequentially inside per-migration transactions.
2. There is no advisory lock or distributed mutex guarding migration execution.
3. In a multi-replica deployment, two nodes booting simultaneously both see pending migrations and execute `CREATE TABLE` DDL concurrently, causing relation-already-exists errors or primary key collisions on `schema_migrations`.

### Logic Chain 4: Rate-Limit Bypass on Quota Exhaustion (`RecordRejection`)
1. Observation 1.1.4 shows `RecordRejection` is defined to penalize exhausted clients by incrementing `WindowRPM` outside the reservation transaction.
2. Observation 1.1.4 shows `RecordRejection` is dead code (0 invocations in the repository).
3. When an account with an exhausted allowance sends a request, `reserveCounters` returns `ExceededError`, rolling back `db.Tx`.
4. The rollback cancels the increment on `WindowRPM`.
5. Because `RecordRejection` is never called, the client's RPM counter remains untouched, allowing an abusive client to flood the chat endpoint indefinitely without being stopped by the RPM rate limiter.

### Logic Chain 5: Quota Double-Counting Window between `Settle` and `Release`
1. Observation 1.1.5 shows `Reserve` increments tokens by worst-case estimate (e.g. 4096 tokens).
2. Upon turn completion, `s.recordTurn` calls `quotaService.Settle(ctx, user, Estimate{}, actual)`, incrementing tokens by actual usage (e.g. 500 tokens).
3. The reservation (-4096 tokens) is only decremented when `defer release()` executes after the HTTP handler finishes.
4. During the interval between stream completion and HTTP handler exit (which includes client SSE reception, stats emission, and cleanup), `usage_counters` contains 4096 + 500 = 4596 tokens instead of 500.
5. If a parallel request from another tab or a rapid subsequent request arrives during this window, it evaluates quota against 4596 tokens, causing false 429 rejections.

### Logic Chain 6: Partial Failure in Card Spend (`card.Handlers.use`)
1. Observation 1.1.6 shows `use` marks a card spent in an independent query outside any transaction.
2. Next, `OnSpend` is called to execute `quotaService.Reset`.
3. If `OnSpend` fails due to database connectivity, timeout, or server shutdown, the card's `used_at` timestamp is already committed to the database.
4. The user permanently loses their card without receiving the quota reset.

### Logic Chain 7: Unbounded Context Starvation (`MarkSeen`)
1. Observation 1.1.7 shows `MarkSeen` is passed `context.WithoutCancel(r.Context())` without `context.WithTimeout`.
2. A context without cancel and without deadline will block indefinitely if a database lock or network delay occurs.
3. The database connection is held checked out from `sql.DB` connection pool, risking pool exhaustion and goroutine leaks.

---

## 3. Caveats

1. **Local Environment Limitations**: PostgreSQL 16 was verified through unit/integration test suites (`dbtest.Postgres`), portability tests, and code inspection. Dynamic verification was executed against SQLite 3 in WAL mode.
2. **Provider Scope**: Upstream AI provider endpoints (OpenAI, Anthropic, Gemini, Ollama) and Cloudflare Turnstile APIs were verified to execute strictly outside database transactions, upholding the core constraint.
3. **No Code Modification**: All findings are based strictly on read-only static analysis and live line verification; existing repository files remain 100% untouched.

---

## 4. Conclusion & Findings Report

### Finding 1: Lack of Row Ownership Verification in `conversation.Store.appendIn`
- **Severity**: **Medium** (CVSS v3.1: 6.5 - `CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:L/A:N`)
- **CWE / OWASP**: CWE-284 (Improper Access Control) / A01:2021 – Broken Access Control
- **Exact Citation**: `internal/conversation/conversation.go:475-479`
- **Technical Description**:
  `appendIn` executes an update statement meant to acquire a row lock:
  `UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?`
  It discards the `sql.Result` without checking `RowsAffected()`. If `in.ConversationID` belongs to another user, 0 rows are updated, no lock is acquired, and `appendIn` proceeds to insert a message belonging to `in.UserID` into the target conversation.
- **Attack Scenario & Exploitability**:
  An authenticated attacker who discovers or guesses a valid conversation ULID of another tenant can submit turns or append calls targeting that conversation ID. The message is inserted and occupies a sequence number in `ux_messages_seq`, polluting `message_count` and desynchronizing transcript sequence ordering.
- **Defensible AGENTS.md Remediation**:
  Check `affected == 0` on the lock query result and return `ErrNotFound`:
  ```go
  // internal/conversation/conversation.go
  res, err := q.Exec(ctx,
      `UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?`,
      in.ConversationID, in.UserID)
  if err != nil {
      return Message{}, fmt.Errorf("conversation: lock for append: %w", err)
  }
  if affected, err := res.RowsAffected(); err == nil && affected == 0 {
      return Message{}, ErrNotFound
  }
  ```

---

### Finding 2: Startup Race Condition and Missing Instance Lock in `ensureGroup`
- **Severity**: **Medium** (CVSS v3.1: 5.3 - `CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:N/A:H`)
- **CWE / OWASP**: CWE-362 (Race Condition) / A05:2021 – Security Misconfiguration
- **Exact Citation**: `internal/server/bootstrap.go:43-69`
- **Technical Description**:
  `ensureGroup` performs a check-then-write (`groups.Count` then `groups.Create`) without holding `settings.Lock(ctx, tx)`. In multi-instance deployments against a clean database, concurrent boots result in a unique constraint violation on `ux_user_groups_name`, crashing the secondary instance.
- **Attack Scenario & Exploitability**:
  During automated cluster orchestration (e.g. Kubernetes replica rollout or Docker Compose `--scale server=2`), simultaneous startup of multiple instances crashes non-leader instances with startup failure.
- **Defensible AGENTS.md Remediation**:
  Acquire `settings.Lock(ctx, tx)` inside `ensureGroup` (mirroring `ensureAdmin`):
  ```go
  // internal/server/bootstrap.go
  func ensureGroup(ctx context.Context, db *database.DB, groups *group.Store) error {
      return db.Tx(ctx, func(tx *database.Tx) error {
          if err := settings.Lock(ctx, tx); err != nil {
              return err
          }
          count, err := groups.Count(ctx, tx)
          if err != nil {
              return err
          }
          if count > 0 {
              return nil
          }
          ...
  ```

---

### Finding 3: Missing Migration Advisory Lock in `database.Migrate`
- **Severity**: **Medium** (CVSS v3.1: 5.3 - `CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:N/A:H`)
- **CWE / OWASP**: CWE-362 (Race Condition) / A05:2021 – Security Misconfiguration
- **Exact Citation**: `internal/database/migrate.go:34-63`
- **Technical Description**:
  `Migrate` applies schema migrations without taking a database-level migration lock. Concurrent container starts against PostgreSQL race on DDL statements, failing with `relation already exists` or duplicate key errors on `schema_migrations`.
- **Attack Scenario & Exploitability**:
  Production deployments with multiple replicas crash during version rollouts when migrations run simultaneously from multiple containers.
- **Defensible AGENTS.md Remediation**:
  In `Migrate`, acquire a PostgreSQL advisory lock when `db.Dialect() == Postgres`:
  ```go
  if db.Dialect() == Postgres {
      if _, err := db.Exec(ctx, `SELECT pg_advisory_lock(83921740)`); err != nil {
          return nil, fmt.Errorf("database: acquire migration lock: %w", err)
      }
      defer db.Exec(context.Background(), `SELECT pg_advisory_unlock(83921740)`)
  }
  ```

---

### Finding 4: Orphaned Rate-Limit Penalty `RecordRejection` Allowing Rate-Limit Bypass
- **Severity**: **Medium** (CVSS v3.1: 5.3 - `CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:L`)
- **CWE / OWASP**: CWE-770 (Allocation of Resources Without Limits), CWE-307 / A04:2021 – Insecure Design
- **Exact Citation**: `internal/quota/service.go:356-363`, `internal/server/server.go:191-197`
- **Technical Description**:
  When a request exceeds quota, `reserveCounters` returns an error that triggers a rollback of the reservation transaction, erasing the RPM bump. The designed countermeasure `RecordRejection` is dead code and never called. Clients with exhausted allowances can hammer the endpoint without triggering rate limits.
- **Attack Scenario & Exploitability**:
  An authenticated client whose 5-hour quota has run out loops thousands of requests per second against `POST /api/chat`. Since every request rolls back without recording an RPM penalty, the client never triggers the 429 RPM limit, imposing heavy database lock and CPU overhead on the server.
- **Defensible AGENTS.md Remediation**:
  In `internal/server/server.go:191-197`, call `quotaService.RecordRejection`:
  ```go
  // internal/server/server.go
  if err != nil {
      freeSlot()
      quotaService.RecordRejection(ctx, account.ID)
      if translated := quota.TranslateError(err); translated != nil {
          return nil, translated
      }
      return nil, err
  }
  ```

---

### Finding 5: Temporary Quota Double-Counting Window between `Settle` and `Release`
- **Severity**: **Medium** (CVSS v3.1: 4.8 - `CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:L`)
- **CWE / OWASP**: CWE-662 (Improper Synchronization), CWE-682 / A04:2021 – Insecure Design
- **Exact Citation**: `internal/server/server.go:200-211, 249-256`, `internal/chat/http.go:212`
- **Technical Description**:
  `quotaService.Settle` is called with `reserved = quota.Estimate{}`, adding `+actual` to `usage_counters`. The reservation is released in `defer release()`. Between stream finish and handler completion, both reservation and actual usage are charged simultaneously, artificially inflating usage counters.
- **Attack Scenario & Exploitability**:
  A user with an allowance close to their limit completes a prompt. If they send a follow-up turn immediately or have a second tab open, the follow-up prompt hits the inflated counter and is rejected with HTTP 429 `quota_exceeded` even though their actual usage was well within limits.
- **Defensible AGENTS.md Remediation**:
  Pass `reserved.estimate` through to `recordTurn` and execute atomic true-up via `quotaService.Settle(ctx, user, reserved.estimate, actual)`.

---

### Finding 6: Non-Transactional Card Spend and Quota Reset in `card.Handlers.use`
- **Severity**: **Low** / **Medium** (CVSS v3.1: 4.3 - `CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:N`)
- **CWE / OWASP**: CWE-662 (Improper Synchronization) / A04:2021 – Insecure Design
- **Exact Citation**: `internal/card/http.go:60-82`, `internal/card/card.go:162-174`
- **Technical Description**:
  `h.store.Spend` marks `usage_cards.used_at = now` and commits immediately outside a transaction. `h.OnSpend` is executed afterward. A network drop, database error, or shutdown between the two burns the card permanently without resetting quota.
- **Attack Scenario & Exploitability**:
  A user redeems and uses a card. If the database connection drops before `quotaService.Reset` executes, the card is marked used but the user receives zero quota reset.
- **Defensible AGENTS.md Remediation**:
  Allow `Spend` to accept a `database.Queryer` and wrap both operations in a single `db.Tx`:
  ```go
  // internal/card/http.go
  err := h.db.Tx(r.Context(), func(tx *database.Tx) error {
      if err := h.store.Spend(r.Context(), tx, account.ID, cardID); err != nil {
          return err
      }
      if h.OnSpendTx != nil {
          return h.OnSpendTx(r.Context(), tx, account)
      }
      return nil
  })
  ```

---

### Finding 7: Unbounded Context in `MarkSeen` Posing Connection Starvation Risk
- **Severity**: **Low** (CVSS v3.1: 3.7 - `CVSS:3.1/AV:N/AC:H/PR:L/UI:N/S:U/C:N/I:N/A:L`)
- **CWE / OWASP**: CWE-400 (Uncontrolled Resource Consumption) / A05:2021 – Security Misconfiguration
- **Exact Citation**: `internal/admin/feedback.go:73`, `internal/feedback/http.go:142`
- **Technical Description**:
  `MarkSeen` is called with `context.WithoutCancel(r.Context())` without a timeout deadline. If a database lock contention or stall occurs, the query hangs indefinitely, leaking database connection pool slots.
- **Attack Scenario & Exploitability**:
  Slow database write locks or connection freezes cause leaked goroutines and connection pool starvation.
- **Defensible AGENTS.md Remediation**:
  Wrap with `context.WithTimeout`:
  ```go
  markCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
  defer cancel()
  _ = h.feedback.MarkSeen(markCtx, thread.Feedback.ID, true)
  ```

---

### Finding 8: Cross-Process In-Memory Settings Cache Desynchronization in Multi-Instance Deployments
- **Severity**: **Low** / **Informational** (CVSS v3.1: 3.1 - `CVSS:3.1/AV:N/AC:H/PR:H/UI:N/S:U/C:N/I:L/A:N`)
- **CWE / OWASP**: CWE-662 (Improper Synchronization) / A04:2021 – Insecure Design
- **Exact Citation**: `internal/settings/settings.go:5-7, 395-417`
- **Technical Description**:
  `settings.Service` maintains an in-memory map `s.values`. Updates written by Instance 1 are never synced to Instance 2's cache until Instance 2 restarts. This contradicts the multi-instance deployment model permitted in `AGENTS.md`.
- **Defensible AGENTS.md Remediation**:
  Add a lightweight periodic background sync (e.g. comparing `SELECT MAX(updated_at) FROM settings`) or PostgreSQL `LISTEN/NOTIFY`.

---

### Finding 9: Inconsistent SQL `LIKE` Wildcard Escaping in Feedback Search
- **Severity**: **Low** / **Informational** (CVSS v3.1: 2.3 - `CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:N/I:N/A:L`)
- **CWE / OWASP**: CWE-20 (Improper Input Validation) / A03:2021 – Injection
- **Exact Citation**: `internal/feedback/feedback.go:340-346`
- **Technical Description**:
  `internal/feedback/feedback.go:344` omits `escapeLike` on operator search strings, causing `_` and `%` to act as SQL pattern wildcards rather than literal characters.
- **Defensible AGENTS.md Remediation**:
  Sanitize search queries with `escapeLike` and specify `ESCAPE '\'`.

---

## 5. Verification Method

To independently verify all findings and validate system integrity:

1. **Source Code Verification**:
   Inspect the exact line ranges cited above using `view_file`:
   - `internal/conversation/conversation.go:475-479` (`appendIn` discarded `RowsAffected`)
   - `internal/server/bootstrap.go:43-69` (`ensureGroup` missing `settings.Lock`)
   - `internal/database/migrate.go:34-63` (`Migrate` missing advisory locking)
   - `internal/quota/service.go:356-364` (`RecordRejection` zero references)
   - `internal/server/server.go:249-256` (`quotaService.Settle` called with empty estimate)
   - `internal/card/http.go:60-82` (`use` separate `Spend` and `OnSpend`)
   - `internal/admin/feedback.go:73` and `internal/feedback/http.go:142` (`WithoutCancel` without timeout)
   - `internal/settings/settings.go:5-7` (in-memory cache without cross-process sync)
   - `internal/feedback/feedback.go:340-346` (missing `escapeLike`)

2. **Automated Sanity Testing**:
   Run full project verification command:
   ```bash
   go test -count=1 ./...
   ```
   All existing unit and concurrency tests continue to pass with zero regressions.
