# Obsidian Arc Comprehensive Security Audit Report

**Target Application**: Obsidian Arc (`github.com/OnyxAxisOwO/ObsidianArc`)  
**Audit Scope**: Full-Stack Architecture, Backend Go Subsystems, Vue 3 Single-Page Application, Database Concurrency & Migrations, Deployment Infrastructure  
**Audit Date**: September 2026  
**Auditor**: Teamwork Preview Security Audit Team  
**Integrity Mode**: Defensive Security Audit (Zero External Dependencies, Non-Weaponized Vulnerability Research)  
**Taxonomy & Frameworks**: OWASP Top 10 (2021), Common Weakness Enumeration (CWE), Common Vulnerability Scoring System (CVSS v3.1)

---

## 1. Executive Summary & Security Posture

### 1.1 Executive Overview & Scope
A comprehensive, full-stack defensive security audit and architectural risk assessment was performed on the Obsidian Arc repository. Obsidian Arc is a single-binary, AI-assisted chat application integrating an embedded Vue 3 frontend, multi-model upstream AI provider proxying (OpenAI, Anthropic, Gemini, Ollama), role-based administration, and dual relational database support (PostgreSQL 16 and SQLite 3 in WAL mode).

The evaluation thoroughly investigated all six core security dimensions mandated by the audit charter:
1. **Dimension 1 — Authentication & Access Control**: Password hashing (Argon2id), session lifecycles, CSPRNG token generation, Role-Based Access Control (RBAC), multi-tenant isolation, and administrative route authorization (`internal/auth`, `internal/admin`, `internal/apikey`, `internal/id`, `internal/config`).
2. **Dimension 2 — Concurrency Control & Database Consistency**: Transaction boundaries, row-level locking patterns, check-then-write invariants, quota deduction/settlement synchronization, and multi-instance distributed consistency (`internal/conversation`, `internal/card`, `internal/quota`, `internal/database`, `internal/settings`, `internal/server`).
3. **Dimension 3 — Input Validation & Injection Defenses**: SQL placeholder parameterization, dialect-free query safety, SQL `LIKE` wildcard escaping, and path traversal defenses across virtual and physical filesystems (`internal/database`, `internal/user`, `internal/feedback`, `internal/reqlog`, `internal/web`).
4. **Dimension 4 — Frontend & Client Security**: DOM injection defenses, strict zero-`v-html` compliance, AST-based markdown and MathML rendering, client-side route guards, and browser storage isolation (`web/src/lib/safe-intro.ts`, `web/src/chat/`, `web/src/router/`, `web/src/stores/`).
5. **Dimension 5 — Network & Interface Security**: Upstream provider proxying, Server-Side Request Forgery (SSRF) and cloud instance metadata (IMDS) exposure, Server-Sent Events (SSE) compression and streaming headers, CORS/CSRF protections, and reverse proxy client IP attribution (`internal/adapter`, `internal/provider`, `internal/httpx`, `docker-compose.yml`).
6. **Dimension 6 — Resource Consumption, Denial of Service (DoS) & Deployment Infrastructure**: HTTP server timeouts, slow-read streaming socket exhaustion, quadratic memory allocation in delta streams, containerization boundaries, and default deployment credentials (`cmd/server`, `Dockerfile`, `docker-compose.yml`, `internal/chat`).

### 1.2 Security Risk Distribution Matrix

The audit identified a total of **26 distinct security findings** across the codebase. No critical remote code execution (RCE) or universal unauthenticated authentication bypass vulnerabilities were identified. However, three High-severity issues were confirmed that expose cloud infrastructure metadata, allow unauthenticated denial of service via slow-reading clients, or configure predictable default database credentials.

#### Risk Summary by Severity Level

```
+-------------------+-----------------------+-----------------------+
|  Severity Level   |   Count of Findings   |       Percentage      |
+-------------------+-----------------------+-----------------------+
|  CRITICAL         |                     0 |            0.0%       |
|  HIGH             |                     3 |           11.5%       |
|  MEDIUM           |                    14 |           53.8%       |
|  LOW / INFO       |                     9 |           34.6%       |
+-------------------+-----------------------+-----------------------+
|  TOTAL FINDINGS   |                    26 |          100.0%       |
+-------------------+-----------------------+-----------------------+
```

#### Risk Distribution Matrix by Dimension & Severity

```
+-------------------------------------------------------+------+--------+-----+--------+
| Audit Dimension                                       | High | Medium | Low | Total  |
+-------------------------------------------------------+------+--------+-----+--------+
| 1. Authentication & Access Control                    |    0 |      3 |   1 |      4 |
| 2. Concurrency Control & Database Consistency         |    0 |      5 |   2 |      7 |
| 3. Input Validation & Injection Defenses              |    0 |      0 |   1 |      1 |
| 4. Frontend & Client Security                         |    0 |      2 |   1 |      3 |
| 5. Network & Interface Security                       |    1 |      2 |   1 |      4 |
| 6. Resource Consumption, DoS & Deployment             |    2 |      2 |   3 |      7 |
+-------------------------------------------------------+------+--------+-----+--------+
| TOTAL                                                 |    3 |     14 |   9 |     26 |
+-------------------------------------------------------+------+--------+-----+--------+
```

### 1.3 Positive Security Posture & Architectural Commendations

Obsidian Arc exhibits commendable engineering discipline and adherence to defensive software architecture. Several systemic vulnerability classes that frequently plague web applications have been eliminated by construction:

1. **Universal SQL Parameterization & Zero SQLi**:
   - 100% of SQL statements across all store implementations (`auth`, `user`, `conversation`, `apikey`, `card`, `quota`, `reqlog`, `settings`, `feedback`) employ positional `?` parameter placeholders.
   - Database queries are parsed and rebound per engine dialect via `database.Rebind` (translating `?` to `$1, $2, ...` on PostgreSQL or preserving `?` on SQLite).
   - Dynamic search filters (`user.ListFilter`, `usage.Filter`, `security.Filter`) build conditions by appending parameters to typed argument slices (`args []any`) rather than string concatenation.
   - `internal/database/portability_test.go` statically lints migrations and queries, rejecting non-portable or engine-specific SQL syntax.

2. **Strict Multi-Tenant Database Scoping (Zero IDOR)**:
   - Tenant isolation is enforced universally at the database query layer. Every query manipulating user-scoped records binds the authenticated caller's identity directly in the `WHERE` clause:
     - Conversations: `WHERE id = ? AND user_id = ?` (`internal/conversation/conversation.go:284, 321, 332`)
     - Message transcripts: `WHERE m.conversation_id = ? AND m.user_id = ?` (`internal/conversation/conversation.go:367`)
     - Image attachments: `WHERE id = ? AND user_id = ?` (`internal/conversation/attachment.go:176`)
     - API Keys: Scoped by `user_id` across `List`, `Issue`, `Update`, and `Delete` (`internal/apikey/apikey.go`)
     - Projects & Feedback: Scoped to `account.ID` (`internal/project/http.go`, `internal/feedback/http.go`)
   - The OpenAI-compatible API (`/v1/chat/completions`, `/v1/models`) authenticates strictly through bearer tokens validated against `api_keys` and scopes all downstream execution to the key owner's identity, ignoring ambient browser session cookies.

3. **Explicit Database Row-Level Locking for Check-Then-Write Invariants**:
   - In strict compliance with `AGENTS.md` ("Check-then-write is a database lock, never a mutex"), Obsidian Arc rejects in-memory synchronization primitives for cross-request invariants.
   - Account-level caps (e.g. 20 API keys per user, attachment upload storage quotas) lock the user record using portable no-op row updates:
     ```sql
     UPDATE users SET updated_at = updated_at WHERE id = ?
     ```
   - Instance-wide invariants (e.g. first-admin bootstrap, ensuring at least one active super-administrator, quota allowance anchors) lock a sentinel record in the settings table:
     ```sql
     INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
     ON CONFLICT (key) DO UPDATE SET updated_at = settings.updated_at
     ```
   - All 44 administrative endpoints under `/api/admin/*` are verified in `internal/server/security_test.go:TestAdminRoutesRequireAnAdministrator`, failing the test gate if a new route is registered without admin authorization middleware.

4. **Cryptographic Rigor in Authentication & Credential Storage**:
   - Passwords are hashed with Argon2id using OWASP-compliant parameters (19 MiB memory, 2 iterations, 1 parallelism, 16-byte random salt, 32-byte key) via `golang.org/x/crypto/argon2` (`internal/auth/password.go:86-87`).
   - Argon2id hash computations are bounded by a channel semaphore (`slots chan struct{}`) to prevent memory starvation denial-of-service under high concurrency (`internal/auth/password.go:29, 152-161`).
   - Constant-time verification (`subtle.ConstantTimeCompare`) and dummy hash verification (`h.DummyVerify`) are executed when accounts do not exist, neutralizing timing side-channels and user enumeration (`internal/auth/password.go:118-150`).
   - Session tokens (256 bits CSPRNG entropy from `crypto/rand`) and API keys are stored hashed with SHA-256 (`token_hash = Digest(token)`); raw bearer credentials exist only on client devices and are never stored in plaintext at rest (`internal/auth/session.go:17-23`, `internal/apikey/apikey.go:148-152`).

5. **Frontend DOM Sanitization Rigor & Zero `v-html` Policy**:
   - Zero instances of `v-html` exist in the Vue frontend.
   - Markdown and LaTeX formulas are rendered via custom Abstract Syntax Tree (AST) builders (`web/src/chat/markdown.ts`, `web/src/chat/math.ts`) that construct native DOM nodes via `document.createElement` and `document.createElementNS('http://www.w3.org/1998/Math/MathML', tag)`, setting text exclusively via `textContent`.
   - The single `innerHTML` assignment in the entire repository (`web/src/lib/safe-intro.ts:32`) parses HTML strictly inside a detached, unattached `<template>` element, filtering allowed nodes through an explicit allowlist before DOM attachment.
   - Sensitive session tokens and secrets are never placed in browser `localStorage` or `sessionStorage`. State storage is reserved exclusively for non-sensitive presentation preferences (`oa:theme`, `oa:accent`, `oa:sidebar:width`).

6. **Network & Transport Hygiene**:
   - Cross-Site Request Forgery (CSRF) is blocked by a global `SameOrigin` middleware (`internal/httpx/middleware.go:258-307`) enforcing `Sec-Fetch-Site` inspection and strict `Origin` header validation against `r.Host`.
   - Session cookies enforce `HttpOnly: true`, `SameSite: Lax`, and `Secure: true` in production (`internal/auth/service.go:785-809`).
   - Server-Sent Events (`text/event-stream`) are explicitly excluded from HTTP compression to prevent chunk buffering and stream starvation (`internal/httpx/compress.go:57-66`).

7. **Container Isolation & Filesystem Hardening**:
   - The production container image builds upon `gcr.io/distroless/static-debian12:nonroot`, executing under unprivileged user `nonroot:nonroot` (UID 65532) (`Dockerfile:54, 63`). The image contains zero system shells (`/bin/sh`, `/bin/bash`), package managers, or command-line utilities.
   - Static web assets are embedded into the Go binary as an immutable `embed.FS` and sanitized via `path.Clean("/"+r.URL.Path)` in `internal/web/web.go`, preventing path traversal attacks.

---

## 2. Comprehensive Vulnerability Breakdown Table

The following table catalogues all 26 security findings confirmed during the audit. Findings are ordered by Severity and Audit Dimension.

| Vulnerability ID | Vulnerability Title | Subsystem | Dimension | Severity | CVSS v3.1 | CWE Identifier | OWASP Top 10 | Primary Code Location |
|---|---|---|---|---|---|---|---|---|
| **OA-SEC-NET-01** | Server-Side Request Forgery (SSRF) & IMDS Exposure via Provider Base URL | Network / Provider | Dim 5: Network | **High** | 7.2 | CWE-918 | A10:2021 – SSRF | `internal/adapter/wire.go:37-69`, `internal/adapter/adapter.go:371-390` |
| **OA-SEC-NET-02** | Missing Server `WriteTimeout` & Streaming Write Deadlines (Slow-Read DoS) | Server / HTTPX | Dim 6: DoS | **High** | 7.5 | CWE-400, CWE-770 | A05:2021 – Security Misconfig | `cmd/server/main.go:94-107`, `internal/httpx/sse.go:121-126` |
| **OA-SEC-DEP-01** | Hardcoded Insecure Default PostgreSQL Password in Docker Compose | Deployment / Docker | Dim 6: Deploy | **High** | 7.5 | CWE-1188, CWE-798 | A07:2021 – Auth Failures | `docker-compose.yml:35, 79` |
| **OA-SEC-AUTH-01** | Missing Rate Limiting and Attempt Throttling on Password Change | Auth / Account | Dim 1: Auth | **Medium** | 6.5 | CWE-307 | A07:2021 – Auth Failures | `internal/auth/http.go:433-455`, `internal/auth/service.go:682-722` |
| **OA-SEC-AUTH-02** | Retaining Compromised Session Token Across Password Rotation | Auth / Session | Dim 1: Auth | **Medium** | 5.9 | CWE-384, CWE-613 | A07:2021 – Auth Failures | `internal/auth/http.go:433-455`, `internal/auth/service.go:716-722` |
| **OA-SEC-ADM-01** | Masked Secret Overwrite Vulnerability in Admin Settings Import | Admin / Settings | Dim 1: Auth | **Medium** | 5.5 | CWE-284, CWE-1025 | A01:2021 – Broken Access Control | `internal/admin/instance.go:217-221, 326-414` |
| **OA-SEC-CONC-01** | Conversation Append Lock Bypass & Cross-Tenant Message Injection | Conversation / Store | Dim 2: Concurrency | **Medium** | 6.5 | CWE-284, CWE-662 | A01:2021 – Broken Access Control | `internal/conversation/conversation.go:475-479` |
| **OA-SEC-CONC-02** | Multi-Node Startup Race Condition in Default Group Creation | Server / Bootstrap | Dim 2: Concurrency | **Medium** | 5.3 | CWE-362 | A05:2021 – Security Misconfig | `internal/server/bootstrap.go:43-69` |
| **OA-SEC-CONC-03** | Missing Schema Migration Advisory Lock in Multi-Instance Deployments | Database / Migrate | Dim 2: Concurrency | **Medium** | 5.3 | CWE-362 | A05:2021 – Security Misconfig | `internal/database/migrate.go:34-63` |
| **OA-SEC-CONC-04** | Decoupled Quota Settle and Release Window Inducing False Quota Exhaustion | Server / Quota | Dim 2: Concurrency | **Medium** | 4.8 | CWE-662, CWE-682 | A04:2021 – Insecure Design | `internal/server/server.go:200-211, 249-256`, `internal/chat/http.go:212` |
| **OA-SEC-CONC-05** | Non-Transactional Card Consumption and Quota Reset in Handlers | Card / Handlers | Dim 2: Concurrency | **Medium** | 4.3 | CWE-662 | A04:2021 – Insecure Design | `internal/card/http.go:60-82`, `internal/card/card.go:162-174` |
| **OA-SEC-CONC-08** | Orphaned Quota Rejection Penalty Permitting Chat Rate-Limit Bypass | Quota / Server | Dim 6: DoS | **Medium** | 5.3 | CWE-770, CWE-307 | A04:2021 – Insecure Design | `internal/quota/service.go:356-363`, `internal/server/server.go:191-197` |
| **OA-SEC-FE-01** | Missing `target="_blank"` on Sanitized Operator Links (Navigation Hijack) | Frontend / Sanitizer | Dim 4: Frontend | **Medium** | 6.5 | CWE-1022, CWE-601 | A01:2021 – Broken Access Control | `web/src/lib/safe-intro.ts:61-76`, `web/test/safe-intro.test.ts:106` |
| **OA-SEC-FE-02** | Unbounded Dual-Path Recursion in DOM Sanitizer (Client-Side DoS) | Frontend / Sanitizer | Dim 4: Frontend | **Medium** | 5.3 | CWE-674 | A05:2021 – Security Misconfig | `web/src/lib/safe-intro.ts:38-59`, `web/src/views/SafeIntro.vue:15-20` |
| **OA-SEC-NET-03** | Upstream API Key and Sensitive Credential Reflection in Error Responses | Provider / Chat | Dim 5: Network | **Medium** | 5.3 | CWE-200, CWE-532 | A01:2021 – Broken Access Control | `internal/chat/chat.go:964-987`, `internal/adapter/wire.go:262-308` |
| **OA-SEC-NET-04** | Quadratic Memory Reallocations & Unbounded Response Streams in SSE | Provider / Adapter | Dim 6: DoS | **Medium** | 5.3 | CWE-400, CWE-770 | A05:2021 – Security Misconfig | `internal/adapter/anthropic.go:474, 484`, `internal/adapter/wire.go:326-352` |
| **OA-SEC-DEP-02** | Overly Permissive Default `OBSIDIAN_TRUSTED_PROXIES` Enabling IP Spoofing | Deployment / Network | Dim 5: Network | **Medium** | 5.3 | CWE-345 | A07:2021 – Auth Failures | `docker-compose.yml:47`, `internal/httpx/clientip.go:33-37` |
| **OA-SEC-AUTH-03** | Verification Token Revocation on Mailer Transient Transport Failure | Auth / Verify | Dim 1: Auth | **Low** | 3.1 | CWE-400 | A04:2021 – Insecure Design | `internal/auth/verify.go:67-78, 221-247` |
| **OA-SEC-CONC-06** | Unbounded `context.WithoutCancel` Posing Connection Pool Starvation | Feedback / Admin | Dim 2: Concurrency | **Low** | 3.7 | CWE-400 | A05:2021 – Security Misconfig | `internal/admin/feedback.go:73`, `internal/feedback/http.go:144` |
| **OA-SEC-CONC-07** | In-Memory Settings Cache Desynchronization Across Multi-Instance Nodes | Settings / Cluster | Dim 2: Concurrency | **Low** | 3.1 | CWE-662 | A04:2021 – Insecure Design | `internal/settings/settings.go:5-7, 395-417` |
| **OA-SEC-INJ-01** | Inconsistent SQL `LIKE` Wildcard Escaping in Feedback Search | Feedback / Store | Dim 3: Injection | **Low** | 2.3 | CWE-20 | A03:2021 – Injection | `internal/feedback/feedback.go:340-346` |
| **OA-SEC-FE-03** | Client-Side Router Guard Incomplete Admin Check & Dead Auth Guard Function | Frontend / Router | Dim 4: Frontend | **Low** | 3.1 | CWE-639, CWE-561 | A01:2021 – Broken Access Control | `web/src/router/index.ts:68-71, 84-110, 119-121`, `web/src/views/admin/AdminPage.vue:230-233` |
| **OA-SEC-NET-05** | Incomplete Reserved Header Denylist Permitting Hop-by-Hop Headers | Provider / Headers | Dim 5: Network | **Low** | 3.7 | CWE-444, CWE-113 | A03:2021 – Injection | `internal/provider/provider.go:76-83, 460-484` |
| **OA-SEC-DEP-03** | Missing Web Server Container Healthcheck Mechanism | Deployment / Docker | Dim 6: Deploy | **Low** | 3.1 | CWE-1384 | A05:2021 – Security Misconfig | `Dockerfile:69-71`, `docker-compose.yml:12-72` |
| **OA-SEC-DEP-04** | Fragile Host Timezone Bind Mounts in Docker Compose | Deployment / Docker | Dim 6: Deploy | **Low** | 2.5 | CWE-1258 | A05:2021 – Security Misconfig | `docker-compose.yml:66-67, 82-83` |
| **OA-SEC-DEP-05** | Database Connection Lacks Role Separation Between DDL and DML | Database / Security | Dim 6: Deploy | **Low** | 2.5 | CWE-272 | A01:2021 – Broken Access Control | `docker-compose.yml:35, 78-80`, `internal/database/migrations/` |

---

## 3. Deep-Dive Technical Vulnerability Analyses

---

### 3.1 Dimension 1: Authentication & Access Control

#### Finding OA-SEC-AUTH-01: Missing Rate Limiting and Attempt Throttling on Password Change
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 6.5 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:N`)
- **CWE Identifier**: CWE-307 (Improper Restriction of Excessive Authentication Attempts)
- **OWASP Top 10 (2021)**: A07:2021 – Identification and Authentication Failures
- **Code Locations**: `internal/auth/http.go:433-455`, `internal/auth/service.go:682-722`
- **Root Cause Analysis**:
  The profile password rotation handler `POST /api/profile/password` (`internal/auth/http.go:433`) delegates password verification to `auth.Service.ChangePassword`. While the primary authentication pathways `Login` (`internal/auth/service.go:570`) and `VerifyCredential` (`internal/auth/service.go:635`) invoke `s.limiter.Begin(ip, username)` to enforce exponential backoff after 5 failed verification attempts, `ChangePassword` completely lacks attempt throttling or rate limiter integration.
- **Theoretical Attack Scenario**:
  An adversary gains temporary access to an authenticated session (e.g. via an unattended kiosk, session hijacking, or cross-site script execution). To establish permanent account takeover, the adversary must rotate the victim's password, which requires supplying `current_password`. Because no rate limiting or lockouts are imposed, the adversary automates a dictionary or brute-force attack against `current_password` across the active session. Once guessed, the password is changed, locking out the legitimate account owner.
- **Business & Technical Impact**:
  Compromises account non-repudiation and facilitates permanent account hijacking from a transient session compromise.
- **Defensible Zero-Dependency Remediation**:
  Pass the client IP into `auth.Service.ChangePassword` and bind verification attempts to `s.limiter`:
  ```go
  // internal/auth/service.go: ChangePassword
  func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, keepSessionID, ip string) error {
      if err := ValidatePassword(newPassword); err != nil {
          return err
      }
      account, err := s.users.ByID(ctx, nil, userID)
      if err != nil {
          return err
      }
      attempt, err := s.limiter.Begin(ip, account.Username)
      if err != nil {
          return err
      }
      defer attempt.finish(attemptCancelled)

      _, hash, err := s.users.CredentialsByLogin(ctx, account.Username)
      if err != nil {
          return err
      }
      ok, _, err := s.hasher.Verify(ctx, hash, currentPassword)
      if err != nil {
          return err
      }
      if !ok {
          attempt.finish(attemptFailed)
          return ErrCurrentPasswordWrong
      }
      attempt.finish(attemptSucceeded)
      // ... proceed with password update ...
  }
  ```

---

#### Finding OA-SEC-AUTH-02: Retaining Compromised Session Token Across Password Rotation
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.9 (`CVSS:3.1/AV:N/AC:H/PR:L/UI:N/S:U/C:H/I:H/A:N`)
- **CWE Identifier**: CWE-384 (Session Fixation), CWE-613 (Insufficient Session Expiration)
- **OWASP Top 10 (2021)**: A07:2021 – Identification and Authentication Failures
- **Code Locations**: `internal/auth/http.go:433-455`, `internal/auth/service.go:716-722`
- **Root Cause Analysis**:
  In `internal/auth/service.go:ChangePassword`:
  ```go
  if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`,
      userID, keepSessionID); err != nil {
      return fmt.Errorf("auth: revoke other sessions: %w", err)
  }
  ```
  The service invalidates all sessions belonging to the user *except* the session making the change (`keepSessionID`). However, `keepSessionID` retains its existing raw secret token and cookie value; no session token rolling or replacement occurs.
- **Theoretical Attack Scenario**:
  A user changes their password specifically because they suspect their session cookie was intercepted or exfiltrated (e.g. via network sniffing, malware, or shoulder surfing). The user successfully changes their password, believing all prior access is severed. However, because the current session's token is preserved rather than rotated, the adversary holding the intercepted cookie retains persistent access to the account.
- **Business & Technical Impact**:
  Undermines credential rotation guarantees; compromised sessions persist beyond password changes.
- **Defensible Zero-Dependency Remediation**:
  Upon successful password change, generate a newly minted session token (`id.Secret(TokenBytes)`), insert the replacement session record within the transaction, delete the old session, and emit an updated `Set-Cookie` header to the browser.

---

#### Finding OA-SEC-AUTH-03: Verification Token Revocation on Mailer Transient Transport Failure
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.1 (`CVSS:3.1/AV:N/AC:H/PR:L/UI:N/S:U/C:N/I:L/A:L`)
- **CWE Identifier**: CWE-400 (Uncontrolled Resource Consumption)
- **OWASP Top 10 (2021)**: A04:2021 – Insecure Design
- **Code Locations**: `internal/auth/verify.go:67-78, 221-247`
- **Root Cause Analysis**:
  In `internal/auth/verify.go:Resend`, the transaction calls `s.issueVerification`, which executes `DELETE FROM email_verifications WHERE user_id = ?` (line 67) and inserts a new token record, committing immediately. Afterward, `s.SendVerification` (line 246) attempts SMTP delivery. If the mail server experiences a transient network timeout or connection reset, the new token was never delivered, the old token has been purged, and the user is blocked from retrying for 2 minutes by `maxOutstandingResend` (line 232).
- **Theoretical Attack Scenario**:
  An attacker triggers intermittent network jitter or connection drops against the SMTP relay during user verification flows. Legitimate users attempting to verify or resend confirmation emails are repeatedly locked into 2-minute lockout intervals without ever receiving a valid token.
- **Business & Technical Impact**:
  User onboarding denial of service and customer friction.
- **Defensible Zero-Dependency Remediation**:
  If `s.SendVerification` returns an error, execute a compensating update resetting `created_at` in `email_verifications` to permit immediate retry:
  ```go
  // internal/auth/verify.go: Resend
  if err := s.SendVerification(ctx, siteName, account.Email, token); err != nil {
      _, _ = s.db.Exec(ctx, `UPDATE email_verifications SET created_at = 0 WHERE user_id = ?`, userID)
      return err
  }
  ```

---

#### Finding OA-SEC-ADM-01: Masked Secret Overwrite Vulnerability in Admin Settings Import
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.5 (`CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:N/I:H/A:H`)
- **CWE Identifier**: CWE-284 (Improper Access Control), CWE-1025 (Comparison Using Wrong Factors)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `internal/admin/instance.go:217-221, 326-414`
- **Root Cause Analysis**:
  In `internal/admin/instance.go:updateSettings` (lines 217-221), masked secrets are explicitly filtered out before persistence:
  ```go
  for _, key := range secretSettings {
      if value, present := body[key]; present && (value == "" || value == secretMask) {
          delete(body, key)
      }
  }
  ```
  However, in `importSettings` (lines 326-414), this check was completely omitted. When an administrator exports the settings JSON (which exports `turnstile.secret_key` as `secretMask = "••••••••"`) and later imports that file, the server persists `"••••••••"` directly into the database as the active Turnstile secret key.
- **Theoretical Attack Scenario**:
  An administrator backs up system configuration via the admin export feature and restores it on a new or existing instance. Following import, Cloudflare Turnstile secret key validation fails globally on every registration, login, API key creation, card redemption, and fast chat challenge. Authentication is completely paralyzed until an operator manually updates the database row.
- **Business & Technical Impact**:
  Persistent system-wide authentication denial of service caused by standard administrative workflows.
- **Defensible Zero-Dependency Remediation**:
  In `internal/admin/instance.go:importSettings`, strip `secretMask` and empty values from `applied`:
  ```go
  // internal/admin/instance.go: importSettings
  for _, key := range secretSettings {
      if value, present := applied[key]; present && (value == "" || value == secretMask) {
          delete(applied, key)
          skipped = append(skipped, key)
      }
  }
  ```

---

### 3.2 Dimension 2: Concurrency Control & Database Consistency

#### Finding OA-SEC-CONC-01: Conversation Append Lock Bypass & Cross-Tenant Message Injection
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 6.5 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:L/A:N`)
- **CWE Identifier**: CWE-284 (Improper Access Control), CWE-662 (Improper Synchronization)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `internal/conversation/conversation.go:475-479`
- **Root Cause Analysis**:
  In `internal/conversation/conversation.go:appendIn`:
  ```go
  if _, err := q.Exec(ctx,
      `UPDATE conversations SET updated_at = updated_at WHERE id = ? AND user_id = ?`,
      in.ConversationID, in.UserID); err != nil {
      return Message{}, fmt.Errorf("conversation: lock for append: %w", err)
  }
  ```
  The method attempts to acquire a row lock on the target conversation. However, the return value `sql.Result` is discarded (`_, err := ...`), and `RowsAffected()` is never verified. If `in.ConversationID` belongs to another tenant, zero rows are updated, no error is returned, and execution proceeds directly to:
  ```go
  q.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = ?`, in.ConversationID)
  ```
  followed by inserting a new message referencing the foreign `in.ConversationID`.
- **Theoretical Attack Scenario**:
  An authenticated user discovers or guesses the ULID of another user's conversation. The attacker submits turns or messages targeting that conversation ID. Because `appendIn` fails to verify row ownership on the lock statement, the foreign message is successfully inserted into the victim's conversation transcript, incrementing `message_count` and desynchronizing sequence numbers.
- **Business & Technical Impact**:
  Cross-tenant transcript integrity violation, sequence desynchronization, and unauthorized data injection into another user's conversation.
- **Defensible Zero-Dependency Remediation**:
  Inspect `RowsAffected()` on the locking statement and abort with `ErrNotFound` if zero rows were updated (matching `internal/apikey/apikey.go:135` and `internal/conversation/attachment.go:138`):
  ```go
  // internal/conversation/conversation.go: appendIn
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

#### Finding OA-SEC-CONC-02: Multi-Node Startup Race Condition in Default Group Creation
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:N/A:H`)
- **CWE Identifier**: CWE-362 (Concurrent Execution using Shared Resource with Improper Synchronization)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `internal/server/bootstrap.go:43-69`
- **Root Cause Analysis**:
  In `internal/server/bootstrap.go:ensureAdmin` (lines 107-120), `settings.Lock(ctx, tx)` is explicitly acquired before counting users to serialize first-admin creation across cluster nodes. However, in `ensureGroup` (lines 43-69), `settings.Lock` is omitted. Under PostgreSQL default `Read Committed` transaction isolation, two instances booting concurrently on a fresh database both read `count == 0` from `groups.Count(ctx, tx)` and both execute `groups.Create(..., Name: "Default")`.
- **Theoretical Attack Scenario**:
  During multi-replica container deployment (e.g. Kubernetes replica sets or Docker Compose `--scale server=2`), two instances boot simultaneously against a fresh database. The second instance fails with unique constraint violation `ux_user_groups_name`, causing the container process to exit with fatal status.
- **Business & Technical Impact**:
  Automated cluster deployment failures and container restart loops.
- **Defensible Zero-Dependency Remediation**:
  Acquire `settings.Lock(ctx, tx)` at the start of `ensureGroup`:
  ```go
  // internal/server/bootstrap.go: ensureGroup
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
          // ... proceed with Default group creation ...
      })
  }
  ```

---

#### Finding OA-SEC-CONC-03: Missing Schema Migration Advisory Lock in Multi-Instance Deployments
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:N/A:H`)
- **CWE Identifier**: CWE-362 (Race Condition)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `internal/database/migrate.go:34-63`
- **Root Cause Analysis**:
  `internal/database/migrate.go:Migrate` queries `appliedVersions` and sequentially applies pending `.sql` migration files. There is no inter-process advisory locking or distributed coordination. When multiple instances start up simultaneously during a rolling deployment, both inspect `schema_migrations`, observe missing versions, and execute DDL statements concurrently.
- **Theoretical Attack Scenario**:
  During zero-downtime rolling upgrades against PostgreSQL, two containers execute `0001_initial.sql` concurrently. PostgreSQL raises `ERROR: relation "users" already exists` or duplicate key violations on `schema_migrations`, terminating the startup sequence of replica nodes.
- **Business & Technical Impact**:
  Deployment failure, partial schema application, or database inconsistency during rolling releases.
- **Defensible Zero-Dependency Remediation**:
  Acquire a PostgreSQL advisory lock when running against PostgreSQL:
  ```go
  // internal/database/migrate.go: Migrate
  if db.Dialect() == Postgres {
      const migrationLockID = 83921740 // Deterministic application lock ID
      if _, err := db.Exec(ctx, `SELECT pg_advisory_lock(?)`, migrationLockID); err != nil {
          return nil, fmt.Errorf("database: acquire migration lock: %w", err)
      }
      defer db.Exec(context.Background(), `SELECT pg_advisory_unlock(?)`, migrationLockID)
  }
  ```

---

#### Finding OA-SEC-CONC-04: Decoupled Quota Settle and Release Window Inducing False Quota Exhaustion
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 4.8 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:L`)
- **CWE Identifier**: CWE-662 (Improper Synchronization), CWE-682 (Incorrect Calculation)
- **OWASP Top 10 (2021)**: A04:2021 – Insecure Design
- **Code Locations**: `internal/server/server.go:200-211, 249-256`, `internal/chat/http.go:212`
- **Root Cause Analysis**:
  Before starting a turn, `quotaService.Reserve` allocates a worst-case estimate (e.g. 4,096 tokens). When the generation finishes, `recordTurn` (`server.go:253`) invokes `quotaService.Settle(ctx, user, quota.Estimate{}, actual)` with an empty estimate, adding the actual spent tokens (e.g. 500 tokens) to `usage_counters`. The 4,096-token reservation is only refunded when `defer release()` executes in `chat/http.go:212` after the HTTP response stream has terminated. During this interim window, `usage_counters` records `4096 + 500 = 4596` tokens.
- **Theoretical Attack Scenario**:
  A user operating within their legitimate quota quota sends prompts in rapid succession or across multiple browser tabs. A request arriving immediately following completion of a prior turn evaluates quota against the temporarily doubled counter, triggering false HTTP 429 `quota_exceeded` rejections.
- **Business & Technical Impact**:
  Erroneous denial of service to legitimate users and degradation of user experience.
- **Defensible Zero-Dependency Remediation**:
  Forward the reserved estimate to `recordTurn` and execute an atomic true-up via `quotaService.Settle(ctx, user, reserved.estimate, actual)`.

---

#### Finding OA-SEC-CONC-05: Non-Transactional Card Consumption and Quota Reset in Handlers
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 4.3 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:N`)
- **CWE Identifier**: CWE-662 (Improper Synchronization)
- **OWASP Top 10 (2021)**: A04:2021 – Insecure Design
- **Code Locations**: `internal/card/http.go:60-82`, `internal/card/card.go:162-174`
- **Root Cause Analysis**:
  In `internal/card/http.go:use`, `h.store.Spend` marks the card spent (`used_at = now`) in an isolated query. Afterward, `h.OnSpend` calls `quotaService.Reset` outside a transaction. As acknowledged in the code comment (lines 60-65), if a crash, database disconnect, or query timeout occurs between lines 73 and 77, the card is consumed permanently while the user receives no quota reset.
- **Theoretical Attack Scenario**:
  Under intermittent network congestion or database failovers, users redeeming pre-paid quota cards lose their cards without receiving their purchased allowance.
- **Business & Technical Impact**:
  Financial dispute, lost user assets, and administrative overhead.
- **Defensible Zero-Dependency Remediation**:
  Per `AGENTS.md` guidelines ("Store methods take a database.Queryer as their second argument"), refactor `Spend` to accept `database.Queryer` and execute `Spend` and `OnSpendTx` inside a unified `db.Tx`.

---

#### Finding OA-SEC-CONC-06: Unbounded `context.WithoutCancel` Posing Connection Pool Starvation
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.7 (`CVSS:3.1/AV:N/AC:H/PR:L/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-400 (Uncontrolled Resource Consumption)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `internal/admin/feedback.go:73`, `internal/feedback/http.go:144`
- **Root Cause Analysis**:
  In `internal/admin/feedback.go:73` and `internal/feedback/http.go:144`, `MarkSeen` is called using `context.WithoutCancel(r.Context())` without a deadline timeout. If a database lock stall or connection hang occurs, the query blocks indefinitely, permanently holding a database connection pool slot.
- **Theoretical Attack Scenario**:
  Multiple client disconnections during high database lock contention cause goroutines running `MarkSeen` to accumulate and exhaust the `sql.DB` connection pool.
- **Business & Technical Impact**:
  Gradual resource leakage and database connection pool starvation.
- **Defensible Zero-Dependency Remediation**:
  Wrap detached contexts with explicit deadlines (`context.WithTimeout`):
  ```go
  markCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
  defer cancel()
  _ = h.feedback.MarkSeen(markCtx, thread.Feedback.ID, true)
  ```

---

#### Finding OA-SEC-CONC-07: In-Memory Settings Cache Desynchronization Across Multi-Instance Nodes
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.1 (`CVSS:3.1/AV:N/AC:H/PR:H/UI:N/S:U/C:N/I:L/A:N`)
- **CWE Identifier**: CWE-662 (Improper Synchronization)
- **OWASP Top 10 (2021)**: A04:2021 – Insecure Design
- **Code Locations**: `internal/settings/settings.go:5-7, 395-417`
- **Root Cause Analysis**:
  `settings.Service` maintains an in-memory map `s.values` caching configuration keys. When settings are updated via `SetMany`, the local node updates its memory cache, but sibling instances sharing the database are not notified. Sibling nodes continue serving stale configuration (e.g. registration state, site title) until restarted.
- **Theoretical Attack Scenario**:
  An administrator disables user registration (`auth.registration = "false"`). Instance 1 applies the change immediately. However, requests routed to Instance 2 continue accepting registrations because its in-memory cache has not been invalidated.
- **Business & Technical Impact**:
  Administrative policy enforcement desynchronization across multi-replica deployments.
- **Defensible Zero-Dependency Remediation**:
  Implement lightweight polling comparing `SELECT MAX(updated_at) FROM settings` or use PostgreSQL `LISTEN/NOTIFY` to trigger cache invalidation.

---

### 3.3 Dimension 3: Input Validation & Injection Defenses

#### Finding OA-SEC-INJ-01: Inconsistent SQL `LIKE` Wildcard Escaping in Feedback Search
- **Severity**: **Low**
- **CVSS v3.1 Score**: 2.3 (`CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-20 (Improper Input Validation)
- **OWASP Top 10 (2021)**: A03:2021 – Injection
- **Code Locations**: `internal/feedback/feedback.go:340-346`
- **Root Cause Analysis**:
  In `internal/user/user.go:569-573` and `internal/reqlog/query.go:94-99`, user search queries are escaped using `escapeLike` (`\\`, `\%`, `\_`) with `ESCAPE '\'`. However, in `internal/feedback/feedback.go:344`, user-supplied search text is formatted directly as `fmt.Sprintf("%%%s%%", filter.Search)` without escaping `%` and `_` characters.
- **Theoretical Attack Scenario**:
  An administrator searching feedback threads with `_` or `%` triggers SQL pattern wildcard matching rather than literal substring search, returning unexpected results or causing suboptimal table scans.
- **Business & Technical Impact**:
  Minor search inconsistency and unexpected query behavior.
- **Defensible Zero-Dependency Remediation**:
  Sanitize search queries with `escapeLike` and append `ESCAPE '\'` to the query predicate.

---

### 3.4 Dimension 4: Frontend & Client Security

#### Finding OA-SEC-FE-01: Missing `target="_blank"` on Sanitized Operator Links (Window Navigation Hijack)
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 6.5 (`CVSS:3.1/AV:N/AC:L/PR:H/UI:R/S:U/C:N/I:H/A:N`)
- **CWE Identifier**: CWE-1022 (Use of Web-Link to Untrusted Target), CWE-601 (URL Redirection to Untrusted Site)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `web/src/lib/safe-intro.ts:61-76`, `web/test/safe-intro.test.ts:106`
- **Root Cause Analysis**:
  In `web/src/lib/safe-intro.ts:copySafeLink`, sanitized anchor tags receive `rel="noopener noreferrer"` but omit `target="_blank"`. In standard browser navigation, anchor tags without a target default to `target="_self"`. In contrast, `web/src/chat/markdown.ts:442` explicitly sets `link.target = '_blank'`.
- **Theoretical Attack Scenario**:
  An attacker compromising an administrative account or exploiting announcement configurations embeds a hyperlink to an external malicious domain in the landing page introduction. When users click the link, the existing browser window navigates away from Obsidian Arc to a phishing page designed to harvest credentials.
- **Business & Technical Impact**:
  Same-window phishing and user credential compromise.
- **Defensible Zero-Dependency Remediation**:
  In `web/src/lib/safe-intro.ts:copySafeLink`, set `target="_blank"`:
  ```typescript
  // web/src/lib/safe-intro.ts: copySafeLink
  target.setAttribute('target', '_blank');
  target.setAttribute('rel', 'noopener noreferrer');
  ```
  **Mandatory Test Suite Synchronization**: Update `web/test/safe-intro.test.ts:106` from `expect(a.hasAttribute('target')).toBe(false)` to `expect(a.getAttribute('target')).toBe('_blank')` to ensure `npm test` passes.

---

#### Finding OA-SEC-FE-02: Unbounded Dual-Path Recursion in DOM Sanitizer (Client-Side Denial of Service)
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-674 (Uncontrolled Recursion)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `web/src/lib/safe-intro.ts:38-59`, `web/src/views/SafeIntro.vue:15-20`
- **Root Cause Analysis**:
  `copySafeIntroChildren` recursively unpacks and processes DOM nodes across two branches:
  1. Unpacking non-whitelisted elements (`!introTags.has(tag)` at line 50).
  2. Descending into clean whitelisted elements (`clean` at line 56).
  Neither branch checks or increments a recursion depth counter.
- **Theoretical Attack Scenario**:
  An announcement containing thousands of deeply nested HTML elements (e.g. `<b><b>...</b></b>` or `<custom><custom>...</custom></custom>`) is rendered on the landing page. The browser JavaScript engine exceeds its maximum call stack size (~10,000 frames), throwing an unhandled `RangeError: Maximum call stack size exceeded` and crashing the page.
- **Business & Technical Impact**:
  Persistent client-side denial of service preventing visitors from accessing the site.
- **Defensible Zero-Dependency Remediation**:
  Add a `depth = 0` parameter and enforce `if (depth > 32) return;` across **both** recursive branches:
  ```typescript
  // web/src/lib/safe-intro.ts
  const MAX_INTRO_DEPTH = 32;

  function copySafeIntroChildren(source: Node, target: Node, depth = 0): void {
    if (depth > MAX_INTRO_DEPTH) return;
    for (const child of source.childNodes) {
      if (child.nodeType === Node.TEXT_NODE) {
        target.appendChild(document.createTextNode(child.textContent ?? ''));
        continue;
      }
      if (!(child instanceof Element)) continue;
      const tag = child.tagName.toUpperCase();
      if (droppedIntroTrees.has(tag)) continue;
      if (!introTags.has(tag)) {
        copySafeIntroChildren(child, target, depth + 1);
        continue;
      }
      const clean = document.createElement(tag.toLowerCase());
      if (tag === 'A') copySafeLink(child, clean);
      copySafeIntroChildren(child, clean, depth + 1);
      target.appendChild(clean);
    }
  }
  ```

---

#### Finding OA-SEC-FE-03: Client-Side Router Guard Incomplete Admin Check & Dead Auth Guard Function
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.1 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:N/A:N`)
- **CWE Identifier**: CWE-639 (Authorization Bypass Through Client State), CWE-561 (Dead Code)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `web/src/router/index.ts:68-71, 84-110, 119-121`, `web/src/views/admin/AdminPage.vue:230-233`
- **Root Cause Analysis**:
  The route `/admin/:section(.*)*` defines `meta: { auth: true, admin: true }`. In `router.beforeEach`, the guard evaluates `meta['auth']` but ignores `meta['admin']`. Function `mayAdminister()` (lines 119-121) is dead code. As documented in `web/src/router/index.ts:112-118`, this behavior is an intentional UX design choice: opening an admin link displays an informative `<UnauthorizedModal />` over the chat UI rather than bouncing the user silently to `/`.
- **Architectural Security Boundary**:
  All backend endpoints under `/api/admin/*` strictly enforce administrative authorization via `auth.RequireRole("admin")` and return HTTP 403 Forbidden. Client-side navigation poses zero risk of exposing backend data.
- **Business & Technical Impact**:
  Unprivileged authenticated users download the ~100 kB admin bundle chunk before being presented with the refusal modal.
- **Defensible Zero-Dependency Remediation**:
  Remove dead function `mayAdminister()` from `web/src/router/index.ts` to clean up codebase dead code while preserving the documented refusal modal UX.

---

### 3.5 Dimension 5: Network & Interface Security

#### Finding OA-SEC-NET-01: Server-Side Request Forgery (SSRF) & IMDS Exposure via Provider Base URL
- **Severity**: **High**
- **CVSS v3.1 Score**: 7.2 (`CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:H/I:H/A:N`)
- **CWE Identifier**: CWE-918 (Server-Side Request Forgery (SSRF))
- **OWASP Top 10 (2021)**: A10:2021 – Server-Side Request Forgery (SSRF)
- **Code Locations**: `internal/adapter/wire.go:37-69`, `internal/adapter/adapter.go:371-390`, `internal/provider/provider.go:431-435`
- **Root Cause Analysis**:
  In `internal/adapter/wire.go:NormalizeBaseURL`:
  ```go
  host := parsed.Hostname()
  loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
  if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (loopback || allowInsecure)) {
      return "", fmt.Errorf("base URL must use https...")
  }
  ```
  1. Any `https://` endpoint is unconditionally permitted, including cloud metadata endpoints (`https://169.254.169.254`, `https://[fd00:ec2::254]`) and internal private subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`).
  2. If an administrator checks `allow_insecure: true`, plain HTTP is permitted to any host, including `http://169.254.169.254` (AWS IMDSv1, GCP, DigitalOcean, OpenStack metadata) and AWS EC2 IPv6 IMDS (`http://[fd00:ec2::254]`).
  3. **Adversarial Discovery on AWS EC2 IPv6 IMDS**: AWS EC2 IPv6 IMDS resides on `fd00:ec2::254` in IPv6 Unique Local Address (ULA) space (`fd00::/8`). Standard Go calls such as `ip.IsLinkLocalUnicast()` return `false` on this address. Naive link-local filtering fails to protect AWS IPv6 instances.
  4. **Adversarial Discovery on DNS Rebinding**: In `internal/adapter/adapter.go:NewRegistry`, `http.Transport.DialContext` performs standard DNS lookups without dial-time IP validation. An attacker supplying a domain pointing to IMDS (e.g. `169.254.169.254.nip.io`) or employing DNS rebinding bypasses string-based hostname filtering.
  5. When model detection is triggered (`POST /api/admin/providers/{id}/detect-models`) or chat completion starts, the server queries `{base_url}/models` or `{base_url}/chat/completions`. Upstream HTTP 4xx/5xx responses reflect up to 400 bytes of their error body back to the client via `upstream.Message` (`internal/chat/chat.go:964-987`, `internal/adapter/wire.go:262-308`).
- **Theoretical Attack Scenario**:
  An administrator or compromised admin account targets AWS EC2 IMDS (`http://169.254.169.254/latest/meta-data/iam/security-credentials/`) by configuring a custom provider base URL. When model detection executes, the error response reflects temporary IAM instance credentials back to the client, leading to cloud infrastructure takeover.
- **Business & Technical Impact**:
  Exfiltration of cloud provider credentials, compromise of internal microservices (e.g. port 8090 Chat service), and lateral movement.
- **Defensible Zero-Dependency Remediation**:
  Implement a defense-in-depth, two-layer validation strategy:
  
  **Layer 1: URL Normalization Filter (`internal/adapter/wire.go`)**
  Block IPv4 metadata, IPv6 link-local, and AWS EC2 IPv6 IMDS:
  ```go
  // internal/adapter/wire.go
  func isBlockedMetadataHost(host string) bool {
      ip := net.ParseIP(host)
      if ip == nil {
          return false
      }
      return ip.IsLinkLocalUnicast() ||
          ip.IsLinkLocalMulticast() ||
          ip.Equal(net.ParseIP("169.254.169.254")) ||
          ip.Equal(net.ParseIP("fd00:ec2::254"))
  }
  ```
  Ensure loopback addresses (`localhost`, `127.0.0.1`, `::1`) remain permitted for local inference runtimes (Ollama, vLLM).

  **Layer 2: Dial-Time IP Resolution against DNS Rebinding (`internal/adapter/adapter.go`)**
  In `NewRegistry`, configure `DialContext` to resolve destination IP addresses and block metadata IPs prior to socket connection:
  ```go
  // internal/adapter/adapter.go: NewRegistry
  dialer := &net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: 30 * time.Second}
  transport := &http.Transport{
      DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
          host, port, err := net.SplitHostPort(addr)
          if err != nil {
              return nil, err
          }
          ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
          if err != nil {
              return nil, err
          }
          for _, ip := range ips {
              if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
                  ip.Equal(net.ParseIP("169.254.169.254")) ||
                  ip.Equal(net.ParseIP("fd00:ec2::254")) {
                  return nil, fmt.Errorf("connection to metadata or link-local address %s is blocked", ip)
              }
          }
          return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
      },
      // ... existing connection pool configuration ...
  }
  ```

---

#### Finding OA-SEC-NET-03: Upstream API Key and Sensitive Credential Reflection in Provider Error Responses
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N`)
- **CWE Identifier**: CWE-200 (Exposure of Sensitive Information), CWE-532 (Insertion of Sensitive Information into Log File)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `internal/chat/chat.go:964-987`, `internal/adapter/wire.go:262-308`
- **Root Cause Analysis**:
  In `internal/chat/chat.go:Describe`:
  ```go
  case adapter.ErrorUpstream:
      return "provider_error", upstream.Message
  default:
      return "provider_rejected", upstream.Message
  ```
  `upstream.Message` is returned directly to end users. If an upstream AI provider, reverse proxy, or local gateway returns an error body echoing the client's request headers (e.g. `{"error": "Failed authorization for Bearer sk-ant-api03-xyz..."}`), the master API key is emitted over SSE to unprivileged users, saved into the database `messages.error` column, and written to request logs.
- **Theoretical Attack Scenario**:
  An unprivileged user sends a request that triggers an authentication or validation error on a misconfigured upstream proxy. The proxy echoes the secret key in its response body. The unprivileged user reads the master provider API key from the chat error bubble.
- **Business & Technical Impact**:
  Leakage of administrative upstream API keys and unauthorized consumption of third-party AI quotas.
- **Defensible Zero-Dependency Remediation**:
  Scrub provider API keys and Bearer patterns before storing or returning `upstream.Message`:
  ```go
  // internal/adapter/wire.go
  func ScrubCredential(text, secret string) string {
      if secret != "" && strings.Contains(text, secret) {
          text = strings.ReplaceAll(text, secret, "[REDACTED]")
      }
      return text
  }
  ```

---

#### Finding OA-SEC-NET-05: Incomplete Reserved Header Denylist Permitting Hop-by-Hop Header Forwarding
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.7 (`CVSS:3.1/AV:N/AC:H/PR:H/UI:N/S:U/C:L/I:L/A:N`)
- **CWE Identifier**: CWE-444 (Inconsistent Interpretation of HTTP Requests), CWE-113 (HTTP Header Manipulation)
- **OWASP Top 10 (2021)**: A03:2021 – Injection
- **Code Locations**: `internal/provider/provider.go:76-83, 460-484`
- **Root Cause Analysis**:
  `internal/provider/provider.go` defines `reservedHeaders` to protect sensitive protocol headers. However, the list omits standard RFC 7230 hop-by-hop headers: `Connection`, `Transfer-Encoding`, `Upgrade`, `Proxy-Authorization`, `TE`, and `Trailer`. If configured on a provider, these headers can interfere with HTTP/1.1 keep-alive connection pooling in `http.Transport` or cause chunk framing desynchronization when routed through intermediate reverse proxies.
- **Theoretical Attack Scenario**:
  An administrator configuring custom headers for an internal provider sets `Connection: close` or `Transfer-Encoding: chunked`, leading to connection pool corruption or proxy parsing errors.
- **Business & Technical Impact**:
  Intermittent upstream communication failures and connection pool degradation.
- **Defensible Zero-Dependency Remediation**:
  Expand `reservedHeaders` map in `internal/provider/provider.go` to include standard hop-by-hop headers (`connection`, `transfer-encoding`, `upgrade`, `proxy-authorization`, `te`, `trailer`).

---

#### Finding OA-SEC-DEP-02: Overly Permissive Default `OBSIDIAN_TRUSTED_PROXIES` Enabling IP Spoofing
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:L`)
- **CWE Identifier**: CWE-345 (Insufficient Verification of Data Authenticity)
- **OWASP Top 10 (2021)**: A07:2021 – Identification and Authentication Failures
- **Code Locations**: `docker-compose.yml:47`, `internal/httpx/clientip.go:33-37`
- **Root Cause Analysis**:
  In `docker-compose.yml:47`:
  ```yaml
  OBSIDIAN_TRUSTED_PROXIES: "${OBSIDIAN_TRUSTED_PROXIES:-172.16.0.0/12,192.168.0.0/16,10.0.0.0/8,127.0.0.1/32}"
  ```
  The compose file trusts all private IPv4 subnets by default. When deployed within an internal corporate network, private cloud VPC, or Docker overlay network, incoming client requests originate from RFC 1918 addresses. `internal/httpx/clientip.go:ClientIP` treats every connecting client as a trusted proxy and trusts client-supplied `X-Forwarded-For` headers.
- **Theoretical Attack Scenario**:
  An attacker on an internal corporate network sends requests with forged `X-Forwarded-For: 8.8.8.8` headers. The server attributes the request to the forged IP address, bypassing IP-based login rate limiting, evading abuse tracking, and polluting audit logs.
- **Business & Technical Impact**:
  Bypass of authentication rate limiters and corruption of security audit logs.
- **Defensible Zero-Dependency Remediation**:
  Default `OBSIDIAN_TRUSTED_PROXIES` to loopback (`127.0.0.1/32`) and document explicit proxy subnet definitions in deployment guides.

---

### 3.6 Dimension 6: Resource Consumption, Denial of Service (DoS) & Deployment Infrastructure

#### Finding OA-SEC-NET-02: Missing Server `WriteTimeout` & Streaming Write Deadlines Enabling Slowloris / Slow-Read DoS
- **Severity**: **High**
- **CVSS v3.1 Score**: 7.5 (`CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H`)
- **CWE Identifier**: CWE-400 (Uncontrolled Resource Consumption), CWE-770 (Allocation of Resources Without Limits)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `cmd/server/main.go:94-107`, `internal/httpx/sse.go:121-126`, `internal/chat/http.go:185`
- **Root Cause Analysis**:
  In `cmd/server/main.go`:
  ```go
  srv := &http.Server{
      Addr:              cfg.Addr,
      Handler:           app.Handler(),
      ReadHeaderTimeout: 15 * time.Second,
      ReadTimeout:       5 * time.Minute,
      IdleTimeout:       120 * time.Second,
      // No WriteTimeout configured (defaults to 0, indefinite)
  }
  ```
  The code comment notes: *"Streaming handlers set their own deadlines through http.ResponseController."* However, an exhaustive audit reveals that `SetWriteDeadline` is **never called anywhere in the repository**. In `internal/httpx/sse.go:121-126`, `flush()` only calls `s.rc.Flush()`.
- **Theoretical Attack Scenario**:
  A remote attacker opens multiple streaming chat connections and shrinks the client TCP receive window to near zero, reading at 1 byte per minute (Slowloris / slow-read attack). Because no write deadline is enforced, the HTTP server goroutines, TCP sockets, response buffers, and upstream LLM streams remain open indefinitely. By launching dozens of concurrent slow-read streams, the attacker exhausts the server's socket descriptor limit and goroutine pool, denying service to legitimate users.
- **Business & Technical Impact**:
  Unauthenticated or low-cost Denial of Service, server process starvation, and excessive upstream billing for stalled streams.
- **Defensible Zero-Dependency Remediation**:
  In `internal/httpx/sse.go`, set a write deadline of 30 seconds before every flush:
  ```go
  // internal/httpx/sse.go
  const sseWriteDeadline = 30 * time.Second

  func (s *SSE) flush() error {
      _ = s.rc.SetWriteDeadline(time.Now().Add(sseWriteDeadline))
      if err := s.rc.Flush(); err != nil {
          return fmt.Errorf("sse: flush: %w", err)
      }
      return nil
  }
  ```

---

#### Finding OA-SEC-DEP-01: Hardcoded Insecure Default PostgreSQL Password in Docker Compose
- **Severity**: **High**
- **CVSS v3.1 Score**: 7.5 (`CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N`)
- **CWE Identifier**: CWE-1188 (Insecure Default Initialization of Resource), CWE-798 (Use of Hard-coded Credentials)
- **OWASP Top 10 (2021)**: A07:2021 – Identification and Authentication Failures
- **Code Locations**: `docker-compose.yml:35, 79`
- **Root Cause Analysis**:
  In `docker-compose.yml`:
  ```yaml
  OBSIDIAN_DB_DSN: postgres://obsidian:${POSTGRES_PASSWORD:-obsidian}@db:5432/obsidian?sslmode=disable
  POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-obsidian}
  ```
  While `OBSIDIAN_SECRET_KEY` uses mandatory parameter expansion `${OBSIDIAN_SECRET_KEY:?set OBSIDIAN_SECRET_KEY}`, `POSTGRES_PASSWORD` defaults to the well-known password `"obsidian"`.
- **Theoretical Attack Scenario**:
  Operators deploying via `docker compose up -d` without an `.env` file launch PostgreSQL with the default password `"obsidian"`. Any compromised container on the shared Docker bridge network, or any host/internal network actor if port 5432 is exposed, can authenticate directly to PostgreSQL, extracting password hashes, user identities, provider API keys, and chat logs.
- **Business & Technical Impact**:
  Direct database compromise and total confidentiality/integrity loss.
- **Defensible Zero-Dependency Remediation**:
  Make `POSTGRES_PASSWORD` mandatory in `docker-compose.yml`:
  ```yaml
  POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in environment}
  ```

---

#### Finding OA-SEC-NET-04: Unbounded Memory Accumulation and Quadratic String Copies in SSE Streams
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-400 (Uncontrolled Resource Consumption), CWE-770 (Allocation of Resources Without Limits)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `internal/adapter/anthropic.go:474, 484`, `internal/adapter/wire.go:326-352`
- **Root Cause Analysis**:
  1. In `internal/adapter/anthropic.go:474, 484`:
     ```go
     case "text_delta":
         text.WriteString(event.Delta.Text)
         result.Text = text.String()
     case "thinking_delta":
         reasoning.WriteString(event.Delta.Thinking)
         result.Reasoning = reasoning.String()
     ```
     `text.String()` and `reasoning.String()` are called on every single delta event chunk. For a stream with 2,500 deltas totaling 60 KB, repeatedly creating string copies generates $O(N^2)$ byte allocations, producing over 75 MB of temporary heap allocations per stream.
  2. In `internal/adapter/wire.go:326`, `readEventStream` does not wrap `body` with `io.LimitReader`. A malfunctioning or malicious upstream stream sending continuous data will consume memory until the process is killed by the OS Out-Of-Memory (OOM) killer.
- **Theoretical Attack Scenario**:
  Concurrent users requesting long completions from Anthropic models generate massive garbage collection CPU pauses and heap spikes, degrading overall server throughput.
- **Business & Technical Impact**:
  Excessive CPU load from garbage collection and risk of process OOM crash under concurrent load.
- **Defensible Zero-Dependency Remediation**:
  Update `result.Text` and `result.Reasoning` only once upon stream termination:
  ```go
  // internal/adapter/anthropic.go
  defer func() {
      result.Text = text.String()
      result.Reasoning = reasoning.String()
  }()
  ```
  Wrap response streams in `readEventStream` with `io.LimitReader(body, 64*1024*1024)`.

---

#### Finding OA-SEC-CONC-08: Orphaned Quota Rejection Penalty Permitting Chat Rate-Limit Bypass
- **Severity**: **Medium**
- **CVSS v3.1 Score**: 5.3 (`CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-770 (Allocation of Resources Without Limits), CWE-307 (Improper Restriction of Excessive Authentication Attempts)
- **OWASP Top 10 (2021)**: A04:2021 – Insecure Design
- **Code Locations**: `internal/quota/service.go:356-363`, `internal/server/server.go:191-197`
- **Root Cause Analysis**:
  In `internal/quota/service.go:356`, `RecordRejection` is explicitly designed to penalize quota-exhausted clients by incrementing the Requests-Per-Minute (RPM) rate window outside the reservation transaction. However, `RecordRejection` is **dead code and never called anywhere in the codebase**. In `internal/server/server.go:191-197`, when `quotaService.Reserve` fails due to allowance exhaustion, the transaction rolls back (rolling back the RPM counter bump), and `RecordRejection` is not invoked.
- **Theoretical Attack Scenario**:
  A client with an exhausted quota allowance loops thousands of requests per second against `POST /api/chat`. Because every request rolls back without penalizing the RPM counter, the client never triggers the 429 RPM rate limit, generating severe database lock contention and CPU overhead on the server.
- **Business & Technical Impact**:
  Denial of service through rate limiter evasion and resource exhaustion.
- **Defensible Zero-Dependency Remediation**:
  In `internal/server/server.go:191-197`, invoke `quotaService.RecordRejection`:
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

#### Finding OA-SEC-DEP-03: Missing Web Server Container Healthcheck Mechanism
- **Severity**: **Low**
- **CVSS v3.1 Score**: 3.1 (`CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-1384 (Improper Handling of Extreme Environmental Conditions)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `Dockerfile:69-71`, `docker-compose.yml:12-72`
- **Root Cause Analysis**:
  The `Dockerfile` omits `HEALTHCHECK` because the Distroless static base lacks `curl` or a shell. However, `docker-compose.yml` also lacks a healthcheck configuration for the `server` service.
- **Theoretical Attack Scenario**:
  The Go process encounters a deadlock, unhandled hang, or resource exhaustion condition. Because Docker has no healthcheck configured, the container remains marked "healthy" / running, preventing container orchestrators from restarting it.
- **Business & Technical Impact**:
  Extended service outages and lack of automated failure recovery.
- **Defensible Zero-Dependency Remediation**:
  Add a lightweight `obsidian-arc healthcheck` CLI subcommand to the Go binary to probe `http://127.0.0.1:8080/api/health` natively without external utilities.

---

#### Finding OA-SEC-DEP-04: Fragile Host Timezone Bind Mounts in Docker Compose
- **Severity**: **Low**
- **CVSS v3.1 Score**: 2.5 (`CVSS:3.1/AV:L/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:L`)
- **CWE Identifier**: CWE-1258 (Exposure of Sensitive System Information Due to Uncleared Debug Information)
- **OWASP Top 10 (2021)**: A05:2021 – Security Misconfiguration
- **Code Locations**: `docker-compose.yml:66-67, 82-83`
- **Root Cause Analysis**:
  `docker-compose.yml` bind mounts `/etc/localtime:/etc/localtime:ro` and `/etc/timezone:/etc/timezone:ro`. On Windows, macOS, and modern Linux distributions that do not possess `/etc/timezone`, Docker creates empty directories on the host, causing container startup failures or corrupted timezone lookups.
- **Theoretical Attack Scenario**:
  Deploying the compose file on non-Debian host operating systems results in mount failures or broken container clocks.
- **Business & Technical Impact**:
  Deployment incompatibility and timestamp calculation errors.
- **Defensible Zero-Dependency Remediation**:
  Remove the `/etc/localtime` and `/etc/timezone` volume mounts from `docker-compose.yml`, relying strictly on the environment variable `TZ: ${TZ:-Asia/Shanghai}`.

---

#### Finding OA-SEC-DEP-05: Database Connection Lacks Role Separation Between DDL and DML
- **Severity**: **Low**
- **CVSS v3.1 Score**: 2.5 (`CVSS:3.1/AV:N/AC:H/PR:H/UI:N/S:U/C:H/I:H/A:H`)
- **CWE Identifier**: CWE-272 (Least Privilege Violation)
- **OWASP Top 10 (2021)**: A01:2021 – Broken Access Control
- **Code Locations**: `docker-compose.yml:35, 78-80`, `internal/database/migrations/`
- **Root Cause Analysis**:
  The application runtime connects to PostgreSQL using the database owner role (`obsidian`), granting unrestricted DDL privileges (`CREATE`, `ALTER`, `DROP`) during everyday runtime operations.
- **Theoretical Attack Scenario**:
  If a flaw in application code were leveraged to execute arbitrary SQL, the attacker could alter or drop database tables.
- **Business & Technical Impact**:
  Violation of the principle of least privilege.
- **Defensible Zero-Dependency Remediation**:
  Document deployment architecture separating migration DDL credentials from runtime least-privilege DML roles.

---

## 4. Architectural Design Tradeoffs vs Hardening Recommendations

A rigorous security audit must differentiate between actionable security vulnerabilities, intentional architectural tradeoffs, and defense-in-depth hardening best practices.

```
+--------------------------------------------------------------------------------------------------+
|                            ARCHITECTURAL TAXONOMY & BOUNDARY ANALYSIS                            |
+------------------------------------+-------------------------------------------------------------+
| Category                           | Subsystems & Examples                                       |
+------------------------------------+-------------------------------------------------------------+
| Confirmed Security Vulnerabilities | SSRF / IMDS, Missing Write Deadlines, Insecure Password     |
| (Actionable Bugs)                  | Default, Append Lock Check Omission, Race in ensureGroup    |
+------------------------------------+-------------------------------------------------------------+
| Intentional Architectural Tradeoffs| Single-Binary Minimalism (Zero Dependencies), Wal Mode DB,  |
| (Deliberate Design Choices)        | Refusal Modal UX over Silent Redirect, Localhost Loopback   |
+------------------------------------+-------------------------------------------------------------+
| Defense-in-Depth Hardening         | Advisory Locks on Migrations, Native Healthcheck Flag,      |
| (Operational Enhancements)         | Timezone Mount Removal, Least-Privilege Database Roles      |
+------------------------------------+-------------------------------------------------------------+
```

### 4.1 The Governing Constraint: Single-Binary Minimalism
As established in `AGENTS.md`, Obsidian Arc is governed by the principle: *"This is a small program. One Go binary with the frontend embedded, one database, no sidecars, no cache tier, no broker. When two designs do the same job, the one with fewer moving parts wins."*

- **Zero Additional Dependencies**: Third-party security libraries, heavy ORMs, external WAF sidecars, or distributed caching services (e.g. Redis) are strictly prohibited.
- **Defensive Implication**: All security remediations proposed in this report strictly utilize the Go standard library, native PostgreSQL/SQLite capabilities, and existing project modules.

### 4.2 Localhost Loopback Exemption
In `internal/adapter/wire.go:58`, `loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"` deliberately permits plain `http://` schemes.
- **Design Tradeoff**: This exception is a core functional requirement supporting local inference engines (Ollama, vLLM) running on the same host.
- **Security Guidance**: Loopback access must **not** be disabled. Remediation for SSRF (OA-SEC-NET-01) must selectively target cloud metadata addresses (`169.254.169.254`, `fd00:ec2::254`) and link-local ranges while explicitly preserving loopback communication.

### 4.3 Client-Side Admin Route Refusal Modal vs Silent Redirect
In `web/src/router/index.ts:112-118`, the client-side router does not immediately redirect unauthorized users away from `/admin`.
- **Design Tradeoff**: The author documented: *"Not a redirect: an administrator's link opened by somebody else should say so over the product rather than bounce them silently to the chat, so the refusal is drawn by the admin screen itself."*
- **Security Guidance**: Server-side endpoints (`/api/admin/*`) enforce authentication and role authorization via `auth.RequireRole("admin")` with HTTP 403. The client-side behavior is an intentional UX decision that poses zero data confidentiality risk. Remediating OA-SEC-FE-03 requires pruning the unused dead function `mayAdminister()`.

---

## 5. Actionable Zero-Dependency Remediation Roadmap

The remediation roadmap is structured into three prioritized phases, ensuring all changes comply with `AGENTS.md` (no external packages, standard library only, dialect-free SQL, row locks for invariants, and 100% test suite compatibility).

```
+-------------------------------------------------------------------------------+
|                    SECURITY REMEDIATION ROADMAP (3 PHASES)                    |
+-------------------------------------------------------------------------------+
| PHASE 1: IMMEDIATE / HIGH SEVERITY (Sprint 1)                                 |
| - OA-SEC-NET-01: Block Cloud Metadata & IPv6 IMDS in Provider URLs & Transport|
| - OA-SEC-NET-02: Enforce ResponseController Write Deadlines on SSE Flushes     |
| - OA-SEC-DEP-01: Mandate Non-Default POSTGRES_PASSWORD in Docker Compose      |
+-------------------------------------------------------------------------------+
| PHASE 2: MEDIUM SEVERITY HARDENING (Sprint 2)                                 |
| - OA-SEC-CONC-01: Check RowsAffected in conversation.Store.appendIn           |
| - OA-SEC-CONC-02: Add settings.Lock to ensureGroup in bootstrap.go            |
| - OA-SEC-CONC-03: Acquire PostgreSQL Advisory Lock in database.Migrate        |
| - OA-SEC-AUTH-01: Integrate Rate Limiter into Profile Password Change Flow    |
| - OA-SEC-AUTH-02: Rotate Active Session Token on Password Change              |
| - OA-SEC-ADM-01: Strip Masked Secrets in Admin Settings Import Handler        |
| - OA-SEC-CONC-08: Connect RecordRejection on Quota Reserve Failure            |
| - OA-SEC-NET-03: Scrub Upstream Provider Credentials from Error Reflections   |
| - OA-SEC-NET-04: Optimize Anthropic Delta Strings & Bound Stream Readers      |
| - OA-SEC-FE-01: Add target="_blank" to Sanitized Links (Sync test assertion)  |
| - OA-SEC-FE-02: Enforce Dual-Path Recursion Depth Cap (depth <= 32) in Sanitizer|
| - OA-SEC-DEP-02: Narrow Default OBSIDIAN_TRUSTED_PROXIES to Loopback          |
+-------------------------------------------------------------------------------+
| PHASE 3: DEFENSE-IN-DEPTH / LOW SEVERITY (Sprint 3)                           |
| - OA-SEC-CONC-04: Atomic True-Up of Quota Settle and Reserved Estimate       |
| - OA-SEC-CONC-05: Wrap Card Spend and Quota Reset in Unified db.Tx            |
| - OA-SEC-CONC-06: Apply context.WithTimeout to Detached WithoutCancel Contexts|
| - OA-SEC-CONC-07: Implement Periodic Settings Cache Refresh                   |
| - OA-SEC-AUTH-03: Handle Transient SMTP Errors Gracefully in Verification     |
| - OA-SEC-INJ-01: Sanitize Feedback Search with escapeLike                     |
| - OA-SEC-NET-05: Expand Reserved Header Denylist to Block Hop-by-Hop Headers  |
| - OA-SEC-FE-03: Prune Dead Function mayAdminister in Vue Router               |
| - OA-SEC-DEP-03: Add Native Healthcheck CLI Probe to Go Binary                |
| - OA-SEC-DEP-04: Remove Fragile Host Timezone Bind Mounts                     |
| - OA-SEC-DEP-05: Publish Enterprise Least-Privilege PostgreSQL Role Guide     |
+-------------------------------------------------------------------------------+
```

---

## 6. Verification Protocol & Independent Audit Evidence

### 6.1 Verification Methodology
1. **Source Code Line Verification**: Every line cited in this report was verified directly against the working tree.
2. **Precondition Realism**: All findings were evaluated against active constraints to eliminate theoretical false positives.
3. **Convention Compliance**: All proposed fixes introduce zero external dependencies, maintain dialect neutrality with `?` placeholders, preserve row locking conventions, and avoid spanning transactions across provider calls.

### 6.2 Automated Test Suite Baseline
The automated test suite was executed across all 34 packages in the repository:
```bash
go test -count=1 ./...
```
**Outcome**: 100% Passed (34 packages tested, 0 failures, 0 skipped, 0 panics). All concurrency race tests (`internal/auth/verify_race_test.go`, `internal/conversation/append_concurrency_test.go`) and administrative route tests (`internal/server/security_test.go`) pass completely.
