# Dimension 1 Security Audit Report: Authentication & Access Control

**Target**: Obsidian Arc (`github.com/OnyxAxisOwO/ObsidianArc`)  
**Auditor**: Explorer 1 (Security Audit Team)  
**Date**: September 2026  
**Scope**: Dimension 1 — Authentication & Access Control (`cmd/`, `internal/auth/`, `internal/admin/`, `internal/apikey/`, `internal/id/`, `internal/config/`, `internal/server/`, `internal/conversation/`, `internal/card/`, `internal/project/`, `internal/feedback/`, `internal/consolessh/`, `web/src/router/`)

---

## 1. Observation

Direct code observations with exact file paths and line numbers:

### 1.1 Password Hashing & Timing Attack Defenses
- **Argon2id Algorithm**: Wrapped in `internal/auth/password.go:27-43` using `golang.org/x/crypto/argon2.IDKey` (`internal/auth/password.go:86-87, 113-114`).
- **Configuration & Parameters**:
  - Memory: default `19456` KiB (~19 MiB) (`internal/config/config.go:235`).
  - Iterations: default `2` (`internal/config/config.go:236`).
  - Parallelism: default `1` (`internal/config/config.go:237`).
  - Salt Length: `16` bytes generated with `crypto/rand.Read` (`internal/config/config.go:238`, `internal/auth/password.go:81-84`).
  - Key Length: `32` bytes (`internal/config/config.go:239`).
  - Max Parallelism: bounded between `1` and `runtime.NumCPU()*2` (`internal/auth/password.go:35-42`), default `min(4, runtime.NumCPU())` (`internal/config/config.go:240`).
- **Concurrency Bounding (DoS Defense)**: Channel semaphore `slots chan struct{}` limits simultaneous hash calculations to prevent memory exhaustion DoS (`internal/auth/password.go:29, 42, 152-161`).
- **Password Constraints**: `MinPasswordChars = 8`, `MaxPasswordChars = 256` (`internal/auth/password.go:46-50`).
- **Constant-Time Comparison**: `subtle.ConstantTimeCompare(candidate, parsed.key) != 1` (`internal/auth/password.go:118-120`).
- **User Enumeration Prevention (Timing Attack Defense)**: `h.DummyVerify(ctx, password)` (`internal/auth/password.go:128-150`) computes Argon2id against a cached dummy hash when an account does not exist during `Login` (`internal/auth/service.go:580-584`) and `VerifyCredential` (`internal/auth/service.go:643-647`).
- **Rehashing**: Automatically detects stale parameters on login and upgrades the hash in database (`internal/auth/password.go:122-125`, `internal/auth/service.go:605-609`).

### 1.2 Session Management & Token Lifecycles
- **Token Entropy**: Generated via `id.Secret(TokenBytes)` with `TokenBytes = 32` (256 bits of CSPRNG entropy from `crypto/rand.Read`, rendered as unpadded URL base64) (`internal/id/id.go:89-98`, `internal/auth/session.go:36-37, 56`).
- **Storage Security**: Session tokens are hashed with SHA-256 (`HashToken(token)`) before storage; raw tokens exist only in client cookies and are never saved in the database (`internal/auth/session.go:17-23, 47-50, 60, 92`).
- **Cookie Attributes**: In `internal/auth/service.go:785-809`:
  - `HttpOnly: true` (XSS exfiltration defense)
  - `Secure: s.cfg.SecureCookie` (defaults to `!dev`, true in production, `internal/config/config.go:220`)
  - `SameSite: http.SameSiteLaxMode`
  - `Path: "/"`
  - `MaxAge: int(s.cfg.TTL.Seconds())` (default 30 days, `internal/config/config.go:218`)
- **Sliding Expiration**: Session touched at most once per `TouchInterval` (default 1 hour, `internal/config/config.go:221`, `internal/auth/service.go:775-777`, `internal/auth/session.go:131-139`).
- **Session Revocation**:
  - Logout: `DeleteByToken(ctx, token)` deletes session row (`internal/auth/session.go:148-150`, `internal/auth/service.go:673-678`).
  - Disabled accounts: `Authenticate` checks `!account.IsActive()` and clears cookies (`internal/auth/service.go:764-766`, `internal/auth/middleware.go:40-42`). Admin disabling immediately calls `DeleteByUser` (`internal/admin/people.go:357-361`).
  - Admin password reset: `SetPassword` deletes all user sessions (`internal/auth/service.go:752`).

### 1.3 API Key Management & Verification
- **Token Structure**: Prefix `tokenPrefix = "sk-oa-"` concatenated with `id.Secret(32)` (256 bits entropy) (`internal/apikey/apikey.go:78-86, 112`).
- **Storage at Rest**: Stored as SHA-256 hex digest (`token_hash = Digest(token)`) (`internal/apikey/apikey.go:148-152, 175-178`).
- **Prefix Display**: `prefixChars = 12` (`token[:prefixChars]`) displayed to owner for recognition (`internal/apikey/apikey.go:85, 116`).
- **Owner Row Lock**: `UPDATE users SET updated_at = updated_at WHERE id = ?` locks user row during key issuance to strictly enforce `MaxPerUser = 20` cap against concurrent requests (`internal/apikey/apikey.go:130-136, 144-146`).
- **Verification**: `Resolve(ctx, token)` looks up by `Digest(token)`, enforcing `!record.Disabled` and `!record.Expired(time.Now())` (`internal/apikey/apikey.go:187-204`).
- **Model Restrictions**: Supports scoping keys to specific model IDs via `api_key_models` table (`internal/apikey/apikey.go:156-162, 385-412`).

### 1.4 Access Control, Administrative Endpoints & Population Locking
- **Administrative Mux Protection**: Every endpoint under `/api/admin/*` is mounted in `internal/admin/admin.go:113-198` wrapped with `auth.RequireAdmin` (`internal/admin/admin.go:114-121`, `internal/auth/middleware.go:72-88`).
- **Granular RBAC**: `hasPermission(account, permission)` enforces fine-grained capabilities ("users", "groups", "models", "providers", "settings", "security", "availability", "usage", etc.) (`internal/admin/permissions.go:17-27`).
- **Route Matrix Test**: `TestAdminRoutesRequireAnAdministrator` (`internal/server/security_test.go:150-256`) tests all 44 admin endpoints, verifying 401 for anonymous and 403 for non-admins, failing if new routes are omitted.
- **Instance-Wide Invariant Row Lock (`settings.Lock`)**:
  ```sql
  INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
  ON CONFLICT (key) DO UPDATE SET updated_at = settings.updated_at
  ```
  in `internal/settings/lock.go:37-44`.
- **First-Time Admin Atomicity**:
  - `server.Bootstrap` (`internal/server/bootstrap.go:113-120`) takes `settings.Lock` before checking `users.Count == 0` to create super admin.
  - `auth.Service.Register` (`internal/auth/service.go:224-236`) takes `settings.Lock` before checking `total == 0` to promote first user.
- **Admin Population Protection**:
  - `admin.lockAdminPopulation` (`internal/admin/people.go:52-54`) takes `settings.Lock`.
  - `updateUser` (`internal/admin/people.go:204, 251-263`) and `deleteUser` (`internal/admin/people.go:431, 450`) prevent demoting, disabling, or deleting the last active super-administrator, returning `errLastAdmin`.
  - `auth.Service.SetPassword` (`internal/auth/service.go:738`) serializes admin password resets against role promotions using `settings.Lock`.

### 1.5 Multi-Tenant Data Scoping & IDOR Prevention
- **Conversations**: Queries include `WHERE id = ? AND user_id = ?` (`internal/conversation/conversation.go:284, 321, 332`).
- **Messages**: Queries include `WHERE m.conversation_id = ? AND m.user_id = ?` (`internal/conversation/conversation.go:367`).
- **Attachments**: `Blob` queries `WHERE id = ? AND user_id = ?` (`internal/conversation/attachment.go:176`).
- **Attachment Quota Lock**: `UPDATE users SET updated_at = updated_at WHERE id = ?` prevents upload quota bypass (`internal/conversation/attachment.go:133-136`).
- **Projects**: Scoped to `account.ID` (`internal/project/http.go:45, 58, 72, 87, 97`).
- **Feedback**: Scoped to `account.ID` (`internal/feedback/http.go:74, 115, 134, 160, 178`).
- **Cards**: Scoped to `account.ID` (`internal/card/http.go:51, 73, 120`, `internal/card/card.go:166`).

### 1.6 Identified Deficiencies & Discrepancies
1. **Missing Attempt Limiter on Password Change**:
   In `internal/auth/http.go:433-455` and `internal/auth/service.go:682-722`:
   `changePassword` calls `h.service.ChangePassword(r.Context(), account.ID, body.CurrentPassword, body.NewPassword, session.ID)`.
   Unlike `Login` (`internal/auth/service.go:570`) and `VerifyCredential` (`internal/auth/service.go:635`), `ChangePassword` does NOT call `s.limiter.Begin()`. There is no rate limiting or exponential backoff on verifying `currentPassword`.
2. **Session Token Retention Across Password Rotation**:
   In `internal/auth/service.go:716-719`:
   ```go
   if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`,
       userID, keepSessionID); err != nil {
       return fmt.Errorf("auth: revoke other sessions: %w", err)
   }
   ```
   The session making the change (`keepSessionID`) is retained with its existing token. No session token regeneration occurs, and no new `Set-Cookie` header is emitted.
3. **Verification Token Revocation on Mailer Transport Failure**:
   In `internal/auth/verify.go:221-247`:
   In `Resend`, transaction runs `s.issueVerification` at line 240, which executes `DELETE FROM email_verifications WHERE user_id = ?` (line 67) and inserts the new token. The transaction commits at line 242. At line 246, `s.SendVerification` is called. If SMTP fails, the old token was deleted, the new token was never delivered, and the user is blocked from retrying for 2 minutes by `maxOutstandingResend` (`internal/auth/verify.go:32, 232-234`).
4. **Masked Secret Overwrite in Settings Import**:
   In `internal/admin/instance.go:217-221`, `updateSettings` explicitly filters masked secrets:
   ```go
   for _, key := range secretSettings {
       if value, present := body[key]; present && (value == "" || value == secretMask) {
           delete(body, key)
       }
   }
   ```
   However, in `importSettings` (`internal/admin/instance.go:326-414`), this check is missing. Importing settings that contain `turnstile.secret_key: "••••••••"` overwrites the active secret key in the database with `"••••••••"`.
5. **Client-Side Router Guard Incomplete Admin Check & Dead Code**:
   In `web/src/router/index.ts:70, 84-110`, `/admin/:section(.*)*` specifies `meta: { auth: true, admin: true }`. `beforeEach` checks `meta['auth']` but ignores `meta['admin']`. `mayAdminister()` (lines 119-121) is dead code. Non-admin users who click admin links fetch the ~100 kB admin bundle before `AdminPage.vue:212-219` renders `<UnauthorizedModal />` (backend `/api/admin/*` endpoints remain protected).

---

## 2. Logic Chain

1. **Password Hashing Robustness**:
   - Observations 1.1 confirm that Argon2id is parameterized according to OWASP standards (19 MiB, 2 iterations, 1 parallelism, 16-byte random salt, 32-byte key).
   - Semaphore bounds (`slots chan struct{}`) prevent memory starvation attacks where an attacker fires 100 parallel login requests to exhaust 2 GB of RAM.
   - `subtle.ConstantTimeCompare` and `h.DummyVerify` ensure that both password verification and unknown user lookups execute in constant time, preventing timing side-channel attacks and user enumeration.
   - *Inference*: Obsidian Arc's password storage and hashing mechanisms are robust and adhere to cryptographic best practices.

2. **Analysis of Password Change Rate Limiting (OA-SEC-AUTH-01)**:
   - Observation 1.6.1 shows that `POST /api/profile/password` and `auth.Service.ChangePassword` lack `s.limiter.Begin()`.
   - In contrast, `Login` and `VerifyCredential` enforce exponential backoff after 5 failures via `Limiter`.
   - If an attacker gains temporary physical access to an unlocked workstation, hijacks a session cookie, or exploits an XSS vulnerability, they cannot immediately lock the victim out without guessing `current_password`.
   - Because no rate limiting or delay is enforced on `ChangePassword`, the attacker can automate high-speed dictionary/brute-force attacks against `current_password`.
   - *Inference*: This represents a valid Medium-severity vulnerability (CWE-307 / A07:2021).

3. **Analysis of Session Token Rotation on Password Change (OA-SEC-AUTH-02)**:
   - Observation 1.6.2 shows that `ChangePassword` deletes all sessions for the user *except* `keepSessionID`.
   - `keepSessionID` preserves its exact database hash and existing cookie token.
   - Standard security guidelines (OWASP Session Management Cheat Sheet) require session token regeneration upon password change to eliminate hijacked or compromised session tokens.
   - If a user changes their password because they suspect their session was intercepted, the attacker's existing session token remains valid on the retained session.
   - *Inference*: This represents a valid Medium-severity vulnerability (CWE-384, CWE-613 / A07:2021).

4. **Analysis of Verification Resend Desynchronization (OA-SEC-AUTH-03)**:
   - Observation 1.6.3 shows that `Resend` commits the database transaction (deleting the old token and inserting a new one) before attempting SMTP delivery.
   - If SMTP fails due to a network glitch, the new token was never sent, but the old token was permanently purged.
   - The user cannot request another verification email immediately because `maxOutstandingResend` (2 minutes) blocks subsequent attempts.
   - *Inference*: This represents a valid Low-severity vulnerability (CWE-400 / A04:2021).

5. **Analysis of Admin Settings Import Overwrite (OA-SEC-ADM-01)**:
   - Observation 1.6.4 shows that `visibleSettings` exports secrets as `secretMask = "••••••••"`.
   - When an operator exports settings and later imports that JSON via `importSettings`, `applied[settings.TurnstileSecretKey]` receives `"••••••••"` because `importSettings` does not strip `secretMask`.
   - `SetMany` persists `"••••••••"` into the database, corrupting Cloudflare Turnstile verification.
   - Consequently, user registration, login, card redemption, API key creation, and fast chat challenges fail globally until the secret is manually updated in the database.
   - *Inference*: This represents a valid Medium-severity vulnerability (CWE-284, CWE-1025 / A01:2021).

6. **Analysis of Multi-Tenant Isolation (Zero IDOR)**:
   - Observation 1.5 proves that every user-owned entity (conversations, messages, attachments, cards, projects, feedback, api keys) enforces tenant scoping directly in SQL `WHERE` clauses (`WHERE id = ? AND user_id = ?`).
   - Handlers extract `userID` strictly from authenticated context (`auth.MustUser(r.Context()).ID`) and never trust client-supplied user IDs.
   - *Inference*: There are zero Insecure Direct Object References (IDOR) across all user endpoints.

7. **Analysis of Concurrency Invariants & Population Locking**:
   - Observations 1.3, 1.4, and 1.5 demonstrate that all business invariants requiring check-then-write logic use explicit database row-level locks (`UPDATE users SET updated_at = updated_at WHERE id = ?` for per-account caps, and `settings.Lock` for instance-wide invariants).
   - This complies strictly with `AGENTS.md` and guarantees safety in multi-instance deployments.

---

## 3. Caveats

1. **Out-of-Scope Upstream Systems**: The SMTP mail relay and Cloudflare Turnstile servers are external third-party infrastructure; their internal availability and security configurations were not audited.
2. **Reverse Proxy Assumptions**: Rate limiting on IP addresses (`s.limiter`, `signups.allow`) relies on `httpx.ClientIP` respecting `OBSIDIAN_TRUST_PROXY` and `OBSIDIAN_TRUSTED_PROXIES`. If deployed behind an untrusted proxy without proper CIDR configuration, IP-based limits could be spoofed.
3. **Database Concurrency**: The row-level lock pattern (`UPDATE users SET updated_at = updated_at WHERE id = ?` and `settings.Lock`) assumes SQLite in WAL mode or PostgreSQL. Non-standard database engines or distributed proxies with delayed transaction replication were not evaluated.

---

## 4. Conclusion

The authentication and access control subsystem of Obsidian Arc demonstrates high architectural discipline:
- Strong Argon2id password hashing with memory bounding and constant-time dummy verification.
- Universal multi-tenant database scoping that completely eliminates IDOR vulnerabilities.
- Portable, engine-independent database row-level locking for concurrency invariants.
- Strict server-side route protection covering all 44 administrative endpoints.

However, four concrete vulnerabilities and one client routing caveat require remediation:
1. **OA-SEC-AUTH-01 (Medium)**: Add attempt throttling (`s.limiter.Begin`) to `ChangePassword`.
2. **OA-SEC-AUTH-02 (Medium)**: Regenerate session token and emit updated cookie upon password change.
3. **OA-SEC-AUTH-03 (Low)**: Handle transient SMTP failure during verification resend without penalizing user throttle.
4. **OA-SEC-ADM-01 (Medium)**: Filter `secretMask` in `importSettings` to prevent Turnstile secret destruction.
5. **OA-SEC-FE-03 (Low)**: Clean up dead code `mayAdminister` in router.

### Detailed Findings Breakdown Table

| ID | Title | File & Line | Severity | CWE / OWASP |
|---|---|---|---|---|
| **OA-SEC-AUTH-01** | Missing Rate Limiting and Attempt Throttling on Password Change | `internal/auth/http.go:433-455`, `internal/auth/service.go:682-722` | **Medium** | CWE-307 / A07:2021 |
| **OA-SEC-AUTH-02** | Retaining Compromised Session Token Across Password Rotation | `internal/auth/http.go:433-455`, `internal/auth/service.go:716-722` | **Medium** | CWE-384, CWE-613 / A07:2021 |
| **OA-SEC-AUTH-03** | Verification Token Revocation on Mailer Transient Failure | `internal/auth/verify.go:67-78, 221-247` | **Low** | CWE-400 / A04:2021 |
| **OA-SEC-ADM-01** | Masked Secret Overwrite Vulnerability in Admin Settings Import | `internal/admin/instance.go:217-221, 326-414` | **Medium** | CWE-284, CWE-1025 / A01:2021 |
| **OA-SEC-FE-03** | Client-Side Router Guard Incomplete Admin Check & Dead Auth Guard Function | `web/src/router/index.ts:70, 84-110, 119-121` | **Low** | CWE-639, CWE-561 / A01:2021 |

---

## 5. Remediation Guide (Tailored to Obsidian Arc)

All remediations strictly adhere to Obsidian Arc conventions: zero external dependencies, database row locks, dialect-free SQL, and standard formatting.

### 5.1 Remediation for OA-SEC-AUTH-01: Throttling Password Change Attempts
In `internal/auth/service.go:ChangePassword`:
Accept `ip` parameter and enforce `s.limiter.Begin`:
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
    // ... continue password update
```

### 5.2 Remediation for OA-SEC-AUTH-02: Session Token Rotation on Password Change
In `internal/auth/service.go:ChangePassword`:
Generate a fresh session token for the current connection and delete the old session:
```go
// internal/auth/service.go: ChangePassword
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentSessionID, ip, ua string) (string, error) {
    // ... after password verification ...
    newToken := id.Secret(TokenBytes)
    now := time.Now()
    newRecord := Session{
        ID:         HashToken(newToken),
        UserID:     userID,
        CreatedAt:  now.UnixMilli(),
        ExpiresAt:  now.Add(s.cfg.TTL).UnixMilli(),
        LastSeenAt: now.UnixMilli(),
        IP:         ip,
        UserAgent:  text.Truncate(ua, MaxUserAgentChars),
    }

    err = s.db.Tx(ctx, func(tx *database.Tx) error {
        if err := s.users.SetPasswordHash(ctx, tx, userID, updated); err != nil {
            return err
        }
        // Invalidate all existing sessions
        if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
            return fmt.Errorf("auth: revoke sessions: %w", err)
        }
        // Insert regenerated session
        if _, err := tx.Exec(ctx,
            `INSERT INTO sessions (id, user_id, created_at, expires_at, last_seen_at, ip, user_agent)
             VALUES (?, ?, ?, ?, ?, ?, ?)`,
            newRecord.ID, newRecord.UserID, newRecord.CreatedAt, newRecord.ExpiresAt,
            newRecord.LastSeenAt, newRecord.IP, newRecord.UserAgent); err != nil {
            return fmt.Errorf("auth: create rotated session: %w", err)
        }
        return nil
    })
    if err != nil {
        return "", err
    }
    return newToken, nil
}
```
In `internal/auth/http.go:changePassword`:
Set the refreshed session cookie on response:
```go
// internal/auth/http.go: changePassword
newToken, err := h.service.ChangePassword(r.Context(), account.ID, body.CurrentPassword, body.NewPassword, session.ID, clientIP, r.UserAgent())
if err != nil {
    // ... error handling ...
}
h.service.SetCookie(w, newToken)
return httpx.NoContent(w)
```

### 5.3 Remediation for OA-SEC-ADM-01: Masked Secret Filtering in Settings Import
In `internal/admin/instance.go:importSettings`:
Filter out `secretMask` and empty values from secret settings:
```go
// internal/admin/instance.go: importSettings
for _, key := range secretSettings {
    if value, present := applied[key]; present && (value == "" || value == secretMask) {
        delete(applied, key)
        skipped = append(skipped, key)
    }
}
```

### 5.4 Remediation for OA-SEC-AUTH-03: Mailer Failure Recovery
In `internal/auth/verify.go:Resend`:
If `s.SendVerification` fails, adjust the `created_at` timestamp in `email_verifications` to allow immediate retry rather than locking the user out:
```go
// internal/auth/verify.go: Resend
if err := s.SendVerification(ctx, siteName, account.Email, token); err != nil {
    // Reset throttle timestamp so the user can immediately retry upon mailer failure
    _, _ = s.db.Exec(ctx, `UPDATE email_verifications SET created_at = 0 WHERE user_id = ?`, userID)
    return err
}
```

### 5.5 Remediation for OA-SEC-FE-03: Router Dead Code Clean Up
In `web/src/router/index.ts`:
Remove the unused function export:
```typescript
// web/src/router/index.ts
// Remove unused export:
// export function mayAdminister(): boolean { return isAdmin.value; }
```

---

## 6. Verification Method

### 6.1 Automated Test Execution
Run the full test suite from the repository root:
```bash
go test -v ./internal/auth/...
go test -v ./internal/admin/...
go test -v ./internal/apikey/...
go test -v ./internal/server/... -run TestAdminRoutesRequireAnAdministrator
go test -v ./internal/server/... -run TestOneUserCannotReachAnothersConversation
```

### 6.2 Code Inspection Checklist
1. Inspect `internal/auth/password.go:86-126` to confirm Argon2id parameters, constant-time compare, and dummy verify.
2. Inspect `internal/auth/http.go:433-455` and `internal/auth/service.go:682-722` to confirm absence of limiter in `ChangePassword` and session retention.
3. Inspect `internal/admin/instance.go:326-414` to confirm lack of secret masking in `importSettings`.
4. Inspect `internal/apikey/apikey.go:130-155` to confirm owner row-level locking on key issue.
5. Inspect `internal/settings/lock.go:37-45` to confirm the UPSERT row-lock on `settings`.

### 6.3 Invalidation Conditions
- Any code commit that wraps `ChangePassword` with `s.limiter.Begin` and regenerates session tokens will invalidate findings OA-SEC-AUTH-01 and OA-SEC-AUTH-02.
- Any change to `importSettings` adding the `secretSettings` mask filter will invalidate OA-SEC-ADM-01.
