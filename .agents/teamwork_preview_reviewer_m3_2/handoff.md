# Handoff Report: Reviewer 2 (Milestone M3 - Security Audit Review)

## 1. Observation

### 1.1 Deliverable & Codebase Inspection
1. **Deliverable Identity & Format**:
   - Deliverable path: `docs/SECURITY_AUDIT.md` (996 lines, 75,805 bytes).
   - Line endings: Verified via PowerShell `(Get-Content docs/SECURITY_AUDIT.md -Raw) -match "\r\n"` which returned `False` (strictly LF, conforming to `AGENTS.md`).
2. **Codebase Line Number & Logic Direct Citations**:
   - **OA-SEC-NET-01 (SSRF / IMDS)**: `internal/adapter/wire.go:37-69` (`NormalizeBaseURL`) permits arbitrary `https://` hosts including cloud metadata (`https://169.254.169.254`, `https://[fd00:ec2::254]`) and private subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`). `internal/adapter/adapter.go:371-390` configures `http.Transport` with default `DialContext` lacking dial-time IP validation or DNS rebinding defense.
   - **OA-SEC-NET-02 (Missing WriteTimeout / Slowloris DoS)**: `cmd/server/main.go:94-107` configures `ReadHeaderTimeout`, `ReadTimeout`, and `IdleTimeout`, but omits `WriteTimeout` (defaults to 0, indefinite). The comment states: `"Streaming handlers set their own deadlines through http.ResponseController."` Codebase search for `SetWriteDeadline` across all source files returned 0 occurrences outside of `docs/SECURITY_AUDIT.md` and audit notes. In `internal/httpx/sse.go:121-126`, `flush()` executes `s.rc.Flush()` without setting write deadlines.
   - **OA-SEC-DEP-01 (Hardcoded Default DB Password)**: `docker-compose.yml:35, 79` configures `OBSIDIAN_DB_DSN: postgres://obsidian:${POSTGRES_PASSWORD:-obsidian}@db:5432/obsidian?sslmode=disable` and `POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-obsidian}`, defaulting to `"obsidian"`.
   - **OA-SEC-AUTH-01 (Unthrottled Password Change)**: `internal/auth/http.go:433-455` and `internal/auth/service.go:682-722` (`ChangePassword`) verify `currentPassword` via `s.hasher.Verify` without rate limiter integration, unlike `Login` and `VerifyCredential` which invoke `s.limiter.Begin`.
   - **OA-SEC-AUTH-02 (Session Retention on Password Change)**: `internal/auth/service.go:716-720` invalidates sessions with `DELETE FROM sessions WHERE user_id = ? AND id <> ?` (`keepSessionID`), preserving the active session token without rotation or cookie refresh.
   - **OA-SEC-ADM-01 (Masked Secret Overwrite)**: `internal/admin/instance.go:217-221` filters `secretMask` in `updateSettings`, but `importSettings` (`internal/admin/instance.go:326-414`) omits this check, saving `"••••••••"` directly to `turnstile.secret_key` upon JSON import.
   - **OA-SEC-CONC-01 (Append Lock RowsAffected Omission)**: `internal/conversation/conversation.go:475-479` executes `UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?`, discarding `RowsAffected()`. If `in.ConversationID` belongs to another tenant, 0 rows update without error, proceeding to insert messages into the foreign conversation ID.
   - **OA-SEC-CONC-02 (Startup Race in Default Group)**: `internal/server/bootstrap.go:43-69` (`ensureGroup`) omits `settings.Lock(ctx, tx)`, while `ensureAdmin` (`internal/server/bootstrap.go:107-120`) explicitly acquires `settings.Lock(ctx, tx)`.
   - **OA-SEC-CONC-03 (Migration Advisory Lock)**: `internal/database/migrate.go:34-63` applies schema migrations without inter-process or distributed advisory locks on PostgreSQL.
   - **OA-SEC-CONC-04 (Quota Settle/Release Window)**: `internal/server/server.go:249-256` invokes `quotaService.Settle(ctx, record.User, quota.Estimate{}, actual)` with empty estimate, while the 4,096-token reservation is only released when `defer release()` runs in `internal/chat/http.go:212`, causing a temporary double-counting window.
   - **OA-SEC-CONC-05 (Non-Transactional Card Consumption)**: `internal/card/http.go:60-82` executes `Spend` and `OnSpend` sequentially without a wrapping transaction; `internal/card/card.go:162-174` omits `database.Queryer` parameter.
   - **OA-SEC-CONC-08 (Orphaned RecordRejection)**: `internal/quota/service.go:356-363` defines `RecordRejection`, which is dead code and never invoked when `quotaService.Reserve` fails in `internal/server/server.go:191-197`.
   - **OA-SEC-FE-01 (Missing target="_blank")**: `web/src/lib/safe-intro.ts:61-76` sets `rel="noopener noreferrer"` on anchors but omits `target="_blank"`. In `web/test/safe-intro.test.ts:106`, the test explicitly asserts `expect(a.hasAttribute('target')).toBe(false)`.
   - **OA-SEC-FE-02 (DOM Sanitizer Unbounded Recursion)**: `web/src/lib/safe-intro.ts:38-59` recurses in `copySafeIntroChildren` on both non-whitelisted tags and clean tags without depth tracking.
   - **OA-SEC-FE-03 (Dead Auth Guard)**: `web/src/router/index.ts:119-121` defines `mayAdminister()`, which is never called in `router.beforeEach`.
   - **Frontend DOM Sanitization Policy**: Grep for `innerHTML` in `web/src/` returned exactly one operational assignment: `web/src/lib/safe-intro.ts:32` inside a detached template. Grep for `v-html` in `web/src/` returned 0 template usages.

### 1.2 Test Suite Execution
- Running `go test ./...` in `E:\Project\ObsidianArc`: All 39 packages passed with zero failures and zero skips.
- Running uncached tests: `go test -count=1 ./internal/server ./internal/conversation ./internal/auth ./internal/adapter` completed with 100% success (server 7.006s, conversation 1.166s, auth 13.197s, adapter 4.714s).

---

## 2. Logic Chain

1. **Logical Soundness of Severity Scoring & Vulnerability Taxonomy**:
   - The findings categorize 26 distinct issues across OWASP Top 10 (2021) and CWE standards:
     - 3 High severity findings: SSRF IMDS (CVSS 7.2), Slowloris / Slow-Read DoS (CVSS 7.5), Default Postgres Password (CVSS 7.5).
     - 14 Medium severity findings (CVSS 4.3 - 6.5).
     - 9 Low / Informational findings (CVSS 2.3 - 3.7).
   - Adversarial analysis verified that the severity scores are balanced:
     - SSRF requires admin privilege (`PR:H`) to configure BaseURL, appropriately capping CVSS at 7.2 rather than claiming unauthenticated RCE.
     - Append lock bypass (`OA-SEC-CONC-01`) is rated Medium (6.5) rather than Critical because `internal/chat/chat.go:574` validates ownership via `s.conversations.Get` before triggering `Append` on standard chat paths.
     - Router admin guard (`OA-SEC-FE-03`) is rated Low (3.1) because backend endpoints under `/api/admin/*` universally enforce `auth.RequireRole("admin")`, preventing data exfiltration.
2. **Concurrency & Database Invariant Verification**:
   - `AGENTS.md` explicitly mandates:
     - "Check-then-write is a database lock, never a mutex"
     - "A transaction never spans a provider call"
     - "Database queries are dialect-free with positional `?` placeholders"
   - The audit accurately identified instances where check-then-write invariants were violated:
     - `ensureGroup` in `internal/server/bootstrap.go` omitted the instance-wide sentinel lock `settings.Lock(ctx, tx)`, while `ensureAdmin` in the same file included it.
     - `internal/card/card.go:Spend` did not accept `database.Queryer`, preventing `card/http.go` from running `Spend` and `OnSpend` inside a single `db.Tx`.
     - `internal/quota/service.go:RecordRejection` was orphaned, causing quota rejection rollbacks to erase rate-limit penalties.
3. **Frontend & Network Defense Verification**:
   - Verified that `web/src/` contains 0 instances of `v-html` and only 1 detached `innerHTML` in `web/src/lib/safe-intro.ts:32`.
   - Verified that markdown and LaTeX formulas use native AST builders (`createElement`, `createElementNS`, `textContent`).
   - Verified that `internal/httpx/compress.go:57-66` excludes `text/event-stream` from compression.
   - Verified that `SetWriteDeadline` is completely missing from SSE flushing, leaving connections open to slow-read resource exhaustion.
4. **Remediation Quality & Zero-Dependency Conformance**:
   - Every single proposed remediation in `docs/SECURITY_AUDIT.md`:
     - Introduces **zero** external Go dependencies (no third-party WAFs, ORMs, or caching libraries).
     - Introduces **zero** npm packages (native DOM depth cap, standard attributes).
     - Employs native PostgreSQL / SQLite constructs (`pg_advisory_lock`, `settings.Lock`, `database.Queryer`).
     - Explicitly warns the implementer that modifying `safe-intro.ts` to add `target="_blank"` requires updating `web/test/safe-intro.test.ts:106` to prevent test failures.
5. **Integrity Verification**:
   - No hardcoded test results, facade implementations, or fabricated verification outputs were present.
   - The report differentiates between actionable security vulnerabilities, intentional architectural tradeoffs (e.g. single-binary design, loopback localhost allowance for Ollama, refusal modal UX), and defense-in-depth hardening.

---

## 3. Caveats

- **Active Developer Workspace State**: During the audit review, `git status` showed an uncommitted modification to `internal/feedback/http.go` timestamped `2026/9/19 23:12:45`. Inspection of the diff confirmed this was a local enhancement by the repository owner adding Turnstile challenge gating to feedback replies (matching recent commit `42661c256854a4102aee24cec090113252546d76`). Per `AGENTS.md` constraints, the reviewer did not touch or revert this file.
- **Production Infrastructure Testing**: Real PostgreSQL 16 and Docker container builds are validated in GitHub Actions CI per `AGENTS.md` guidelines ("trust the CI run, not your laptop"). Concurrency models were analyzed against PostgreSQL transaction isolation semantics (`Read Committed`).

---

## 4. Conclusion

**Verdict: APPROVE**

The deliverable `docs/SECURITY_AUDIT.md` represents an exceptionally rigorous, technically sound, and defensively valuable security audit:
- All 26 vulnerability findings were empirically confirmed against the codebase.
- Citations, file paths, and line numbers are verified.
- Severity scoring (CVSS v3.1, CWE, OWASP Top 10) is accurate and non-hyperbolic.
- Concurrency models and database row-level locking adhere strictly to `AGENTS.md`.
- Remediation guidance strictly complies with the zero-dependency, single-binary minimalism constraint.

---

## 5. Verification Method

To independently verify the audit conclusions and codebase facts:
1. **Verify Test Suite Baseline**:
   ```bash
   go test -count=1 ./...
   ```
   *Expected outcome*: 100% pass across all 39 packages.
2. **Verify Line Endings of Deliverable**:
   ```powershell
   (Get-Content docs/SECURITY_AUDIT.md -Raw) -match "`r`n"
   ```
   *Expected outcome*: `False` (pure LF).
3. **Verify Zero `v-html` and Single Detached `innerHTML`**:
   ```bash
   grep -rn "v-html" web/src/
   grep -rn "innerHTML" web/src/
   ```
   *Expected outcome*: Zero `v-html` usages; exactly one `innerHTML` assignment at `web/src/lib/safe-intro.ts:32`.
4. **Verify Omission of `SetWriteDeadline`**:
   ```bash
   grep -rn "SetWriteDeadline" cmd/ internal/
   ```
   *Expected outcome*: 0 occurrences.
