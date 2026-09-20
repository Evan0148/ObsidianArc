# Comprehensive Security Audit Report: Explorer 3
**Scope**: Dimension 4 (Frontend & Client Security), Dimension 5 (Network & Interface Security), Dimension 6 (Resource Consumption & DoS)  
**Target**: Obsidian Arc (`github.com/OnyxAxisOwO/ObsidianArc`)  
**Auditor**: Explorer 3 (Teamwork Preview Security Audit Team)  
**Date**: September 2026  
**Integrity Mode**: Read-Only Defensive Audit (Zero Source Modifications, Strict Evidence Chains)

---

## 1. Observation

A line-by-line inspection of the codebase was conducted across frontend (`web/src/`) and backend (`internal/adapter/`, `internal/provider/`, `internal/httpx/`, `internal/server/`, `internal/chat/`, `cmd/server/`, `Dockerfile`, `docker-compose.yml`). Below are verbatim observations with exact file paths and line numbers:

### 1.1 Frontend & Client Security Observations (Dimension 4)
1. **Zero `v-html` and Single Detached `innerHTML`**:
   - `grep -r "v-html" web/src/`: Zero active occurrences. All hits occur in explanatory comments (`web/src/views/SafeIntro.vue:5`, `web/src/views/admin/AdminFeedback.vue:324`, `web/src/components/OaMarkdown.vue:7`).
   - `grep -r "innerHTML" web/src/`: Exactly one assignment exists in `web/src/lib/safe-intro.ts:32`:
     ```typescript
     const source = document.createElement('template');
     source.innerHTML = html;
     ```
     This assignment targets a detached, unattached `<template>` element.
2. **Safe Link Target Attribute Omission**:
   - In `web/src/lib/safe-intro.ts:61-76` (`copySafeLink`):
     ```typescript
     function copySafeLink(source: Element, target: HTMLElement): void {
       const href = source.getAttribute('href');
       if (href) {
         try {
           const protocol = new URL(href, window.location.href).protocol;
           if (protocol === 'http:' || protocol === 'https:' || protocol === 'mailto:' || protocol === 'tel:') {
             target.setAttribute('href', href);
           }
         } catch {
           // A malformed link remains text.
         }
       }
       const title = source.getAttribute('title');
       if (title) target.setAttribute('title', title);
       target.setAttribute('rel', 'noopener noreferrer');
     }
     ```
     `target="_blank"` is omitted. In contrast, `web/src/chat/markdown.ts:442` explicitly sets `link.target = '_blank'`.
   - In `web/test/safe-intro.test.ts:106`, the test explicitly asserts:
     ```typescript
     expect(a.hasAttribute('target')).toBe(false);
     ```
3. **Unbounded Recursion in DOM Sanitizer**:
   - In `web/src/lib/safe-intro.ts:38-59` (`copySafeIntroChildren`):
     ```typescript
     function copySafeIntroChildren(source: Node, target: Node): void {
       for (const child of source.childNodes) {
         ...
         if (!introTags.has(tag)) {
           copySafeIntroChildren(child, target);
           continue;
         }
         const clean = document.createElement(tag.toLowerCase());
         if (tag === 'A') copySafeLink(child, clean);
         copySafeIntroChildren(child, clean);
         target.appendChild(clean);
       }
     }
     ```
     There is no depth counter. Nested DOM trees recursively invoke `copySafeIntroChildren` through two separate paths (unpacking unwhitelisted elements at line 50, and descending into whitelisted elements at line 56).
4. **AST-Based Markdown & MathML Construction**:
   - In `web/src/chat/markdown.ts:101-106`:
     ```typescript
     export function safeHref(value: string): string | null {
       const href = String(value ?? '').trim();
       return /^(?:https?:\/\/|mailto:)[^\s]+$/i.test(href) ? href : null;
     }
     ```
     Disallows `javascript:`, `data:`, and local schemes.
   - In `web/src/chat/markdown.ts:117-122`: Images are converted to plain links or text, never `<img>` tags.
   - In `web/src/chat/markdown.ts:190-198`: `<` only matches autolinks (`<http...>` or `<mailto:...>`). All other tags are appended as literal text nodes via `document.createTextNode` (`markdown.ts:421`).
   - In `web/src/chat/math.ts:14-18, 686-694`: MathML is rendered via `document.createElementNS('http://www.w3.org/1998/Math/MathML', tag)` and `textContent`. Lexer errors fall back to `container.textContent = latex`.
5. **Client-Side Storage**:
   - `localStorage` holds only UI preferences: `oa:language`, `oa:theme`, `oa:accent`, `oa:rail:collapsed`, `oa:sidebar:width`, `oa:dismissed-announcement`, and terminal settings (`oa:terminal:history`, `oa:terminal:prefs`).
   - `sessionStorage` is never accessed.
   - Session authentication relies exclusively on `HttpOnly`, `SameSite=Lax` cookies; session state in `web/src/stores/session.ts` is held in memory (`ref<Account | null>(null)`).
6. **Vue Router Navigation Guards**:
   - In `web/src/router/index.ts:68-71`: `/admin/:section(.*)*` has `meta: { auth: true, admin: true }`.
   - In `web/src/router/index.ts:84-110`: `router.beforeEach` only checks `record.meta['auth']` (line 86). It does not check `record.meta['admin']`.
   - In `web/src/router/index.ts:119-121`: `export function mayAdminister(): boolean { return isAdmin.value; }` is exported but never referenced anywhere.
   - In `web/src/views/admin/AdminPage.vue:230-233`: Direct navigation to `/admin` by non-admins renders:
     ```vue
     <template v-if="!isAdmin">
       <ChatLayout />
       <UnauthorizedModal />
     </template>
     ```

---

### 1.2 Network & Interface Security Observations (Dimension 5)
1. **Server-Side Request Forgery via Provider Base URL**:
   - In `internal/adapter/wire.go:37-69` (`NormalizeBaseURL`):
     ```go
     host := parsed.Hostname()
     loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
     if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (loopback || allowInsecure)) {
         return "", fmt.Errorf("base URL must use https (plain http needs the provider's own opt-in, and is always allowed for localhost)")
     }
     ```
     - For `https://`, any host is accepted, including IPv4 link-local metadata `https://169.254.169.254`, IPv6 link-local addresses (`fe80::/10`), AWS EC2 IPv6 IMDS `https://[fd00:ec2::254]`, and RFC 1918 subnets (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`).
     - If `allowInsecure: true`, plain `http://` is accepted to `http://169.254.169.254` (AWS/GCP/DigitalOcean metadata) and `http://[fd00:ec2::254]`.
   - In `internal/adapter/adapter.go:371-390` (`NewRegistry`):
     ```go
     transport := &http.Transport{
         DialContext: (&net.Dialer{
             Timeout:   cfg.DialTimeout,
             KeepAlive: 30 * time.Second,
         }).DialContext,
         ...
     }
     ```
     `DialContext` performs standard DNS resolution without dial-time IP verification, leaving socket connections susceptible to DNS rebinding or wildcard domain mapping (e.g. `169.254.169.254.nip.io`).
2. **Upstream Error Reflection**:
   - In `internal/adapter/wire.go:262-308` (`extractErrorMessage`), up to 400 characters of the upstream response body are returned.
   - In `internal/chat/chat.go:964-987` (`Describe`):
     ```go
     case adapter.ErrorUpstream:
         return "provider_error", upstream.Message
     default:
         return "provider_rejected", upstream.Message
     ```
     If an upstream provider or custom proxy returns an error echoing headers (e.g. `Bearer sk-ant-api03-...`), this sensitive text is returned directly to end users, saved in the database `messages.error` column, and written to request logs.
3. **Hop-by-Hop Headers in Reserved Headers Denylist**:
   - In `internal/provider/provider.go:76-83`:
     ```go
     var reservedHeaders = map[string]bool{
         "authorization":     true,
         "x-api-key":         true,
         "anthropic-version": true,
         "content-type":      true,
         "content-length":    true,
         "host":              true,
     }
     ```
     Standard RFC 7230 hop-by-hop headers (`Connection`, `Transfer-Encoding`, `Upgrade`, `Proxy-Authorization`, `TE`, `Trailer`) are not denylisted.
4. **CORS & CSRF Defenses**:
   - In `internal/httpx/middleware.go:258-307` (`SameOrigin`): Rejects state-changing methods if `Sec-Fetch-Site` is `cross-site` or `same-site`, and validates `Origin` against `permitted` and `r.Host`.
   - Applied globally in `internal/server/server.go:859`.
   - Session cookies are set with `SameSite: http.SameSiteLaxMode` and `HttpOnly: true` (`internal/auth/service.go:795, 807`).
5. **SSE Streaming & HTTP Compression**:
   - In `internal/httpx/compress.go:57-66`: `text/event-stream` is strictly excluded from `compressible`.
   - In `internal/httpx/compress.go:130-146`: `gzipWriter.Flush()` unrolls `Unwrap()` to call `http.NewResponseController(w.ResponseWriter).Flush()`.
   - In `internal/httpx/sse.go:37-45`: `NewSSE` sets `Content-Type: text/event-stream; charset=utf-8`, `Cache-Control: no-cache, no-transform`, and `X-Accel-Buffering: no`.
6. **Reverse Proxy Headers & Client IP**:
   - In `internal/httpx/clientip.go:98-142` (`ClientIP`): Traverses `X-Forwarded-For` from right to left, stopping at the first untrusted IP (preventing left-side header spoofing).
   - In `docker-compose.yml:47`:
     ```yaml
     OBSIDIAN_TRUSTED_PROXIES: "${OBSIDIAN_TRUSTED_PROXIES:-172.16.0.0/12,192.168.0.0/16,10.0.0.0/8,127.0.0.1/32}"
     ```
     Defaults to trusting all RFC 1918 private subnets.

---

### 1.3 Resource Consumption & DoS Observations (Dimension 6)
1. **Server Timeout Configuration & Missing Write Deadlines**:
   - In `cmd/server/main.go:94-107`:
     ```go
     srv := &http.Server{
         Addr:              cfg.Addr,
         Handler:           app.Handler(),
         ReadHeaderTimeout: 15 * time.Second,
         ReadTimeout:       5 * time.Minute,
         IdleTimeout:       120 * time.Second,
         MaxHeaderBytes:    64 << 10,
         ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
     }
     ```
     `WriteTimeout` is unset (defaults to 0 / indefinite). The comment states: *"Streaming handlers set their own deadlines through http.ResponseController."*
   - In `internal/httpx/sse.go:121-126`:
     ```go
     func (s *SSE) flush() error {
         if err := s.rc.Flush(); err != nil {
             return fmt.Errorf("sse: flush: %w", err)
         }
         return nil
     }
     ```
     `SetWriteDeadline` is **never called anywhere in the repository** (`grep -r "SetWriteDeadline" .` returns zero results).
2. **Quadratic String Allocation in Streaming**:
   - In `internal/adapter/anthropic.go:474, 484`:
     ```go
     case "text_delta":
         text.WriteString(event.Delta.Text)
         result.Text = text.String()
     case "thinking_delta":
         reasoning.WriteString(event.Delta.Thinking)
         result.Reasoning = reasoning.String()
     ```
     `text.String()` and `reasoning.String()` are executed on every single delta event chunk.
3. **Unbounded Response Stream Consumption**:
   - In `internal/adapter/wire.go:326-352`: `readEventStream` consumes `body io.Reader` directly without wrapping it in `io.LimitReader`.
4. **Request Body Bounds & Upload Guards**:
   - `httpx.DecodeJSON` (`internal/httpx/httpx.go:178-226`): Strictly enforces `http.MaxBytesReader`, returns 413, disallows unknown fields, and rejects multiple JSON objects.
   - Chat payloads bounded to 256 KiB (`internal/chat/http.go:129`).
   - Attachment upload bounded to `ceiling*4/3 + 16*1024` with default 6 MiB (`internal/chat/http.go:505, 515`).
   - Attachment decoding concurrency bounded by buffered channel `h.decoding` (`internal/chat/http.go:537-542`).
   - User avatar bounded to 8 KiB (`internal/user/user.go:119`).
5. **Deployment & Containerization**:
   - In `docker-compose.yml:35, 79`:
     ```yaml
     OBSIDIAN_DB_DSN: postgres://obsidian:${POSTGRES_PASSWORD:-obsidian}@db:5432/obsidian?sslmode=disable
     POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-obsidian}
     ```
     Hardcodes `"obsidian"` as the default PostgreSQL password.
   - In `Dockerfile:54, 63`: Container builds upon `gcr.io/distroless/static-debian12:nonroot` and runs under unprivileged UID 65532 (`nonroot:nonroot`).
   - In `Dockerfile:69-71` & `docker-compose.yml:12-72`: `server` service lacks a container health check.
   - In `docker-compose.yml:66-67, 82-83`: Bind mounts `/etc/localtime` and `/etc/timezone`.

---

## 2. Logic Chain

### 2.1 Logic Chain: Frontend & Client Security
1. **DOM Sanitizer Link Phishing (OA-SEC-FE-01)**:
   - *Premise 1*: `web/src/lib/safe-intro.ts:61-76` sanitizes links and sets `rel="noopener noreferrer"`, but omits `target="_blank"`.
   - *Premise 2*: In modern browsers, anchor tags without a target default to `target="_self"`.
   - *Premise 3*: An operator or compromised admin configuring a landing page announcement can insert a link pointing to an external domain.
   - *Inference*: When users click the link, the existing Obsidian Arc tab navigates away to the external URL, enabling phishing attacks mimicking the login screen.
2. **DOM Sanitizer Unbounded Recursion DoS (OA-SEC-FE-02)**:
   - *Premise 1*: `web/src/lib/safe-intro.ts:38-59` recursively traverses DOM trees via two distinct branches: unpacking unwhitelisted wrappers (line 50) and recursing into whitelisted tags (line 56).
   - *Premise 2*: Neither branch increments or checks a recursion depth counter.
   - *Premise 3*: JavaScript engines enforce a fixed call stack size (~10,000 frames).
   - *Inference*: Supplying HTML with deep nesting causes `RangeError: Maximum call stack size exceeded`, crashing the visitor's browser tab before the UI renders.
3. **Admin Route Refusal UX vs Guard (OA-SEC-FE-03)**:
   - *Premise 1*: `web/src/router/index.ts:84-110` checks `meta['auth']` but omits `meta['admin']`. Function `mayAdminister()` at line 119 is dead code.
   - *Premise 2*: Documented intent (`web/src/router/index.ts:115-117` and `web/src/views/admin/AdminPage.vue:227-233`) deliberately defers refusal to `AdminPage.vue` so that unauthorized users see an explicit refusal modal (`<UnauthorizedModal />`) over the chat instead of being bounced silently.
   - *Premise 3*: Backend `/api/admin/*` endpoints strictly require `admin` role and return 403.
   - *Inference*: No unauthorized server data is accessible. The only cost is downloading the ~100 kB admin bundle chunk before displaying the refusal modal.

### 2.2 Logic Chain: Network & Interface Security
1. **SSRF via Provider Base URL & DNS Rebinding (OA-SEC-NET-01)**:
   - *Premise 1*: `NormalizeBaseURL` (`internal/adapter/wire.go:37-69`) permits any `https://` endpoint without IP filtering, and permits `http://` to any address if `allowInsecure` is checked.
   - *Premise 2*: Cloud environments (AWS, GCP, DigitalOcean, OpenStack) expose metadata services on IPv4 link-local `169.254.169.254` and AWS EC2 IPv6 IMDS on `fd00:ec2::254` (ULA).
   - *Premise 3*: `http.Transport.DialContext` in `internal/adapter/adapter.go:371-390` performs standard DNS lookups without validating resolved IPs at socket dial time.
   - *Inference*: An admin can configure Base URLs pointing directly to IMDS or use wildcard DNS (`169.254.169.254.nip.io`) to query metadata and internal VPC services. Upstream error bodies reflect up to 400 bytes back to the client (`internal/chat/chat.go:964-987`), facilitating credential exfiltration.
2. **Upstream Credential Reflection (OA-SEC-NET-03)**:
   - *Premise 1*: When an upstream request fails with HTTP 5xx (`ErrorUpstream`) or 400 (`ErrorInvalidRequest`), `internal/chat/chat.go:964-987` emits `upstream.Message` verbatim.
   - *Premise 2*: Intermediary proxies or self-hosted AI gateways often echo failed `Authorization: Bearer <key>` headers in error bodies.
   - *Inference*: Upstream master API keys can be echoed to unprivileged chat users, stored in database error logs, and displayed on screen.
3. **Hop-by-Hop Header Forwarding (OA-SEC-NET-05)**:
   - *Premise 1*: `internal/provider/provider.go:76-83` denylists protocol headers but omits RFC 7230 hop-by-hop headers (`Connection`, `Transfer-Encoding`, `Upgrade`, etc.).
   - *Inference*: Custom provider headers could corrupt HTTP/1.1 connection reuse or cause chunk desynchronization across reverse proxies.
4. **IP Spoofing via Trusted Proxies Default (OA-SEC-DEP-02)**:
   - *Premise 1*: `docker-compose.yml:47` configures `OBSIDIAN_TRUSTED_PROXIES` to default to all RFC 1918 subnets (`172.16.0.0/12, 192.168.0.0/16, 10.0.0.0/8, 127.0.0.1/32`).
   - *Premise 2*: In internal corporate networks or Docker bridge/overlay setups, client connections arrive from RFC 1918 addresses.
   - *Inference*: `ClientIP` treats every connecting client as a trusted proxy and trusts `X-Forwarded-For`, allowing clients to bypass IP-based rate limits and falsify audit logs.

### 2.3 Logic Chain: Resource Consumption & DoS
1. **Slow-Read DoS via Missing Write Deadlines (OA-SEC-NET-02)**:
   - *Premise 1*: `cmd/server/main.go:97-99` leaves `http.Server.WriteTimeout` at 0 (unbounded), delegating write deadlines to handlers.
   - *Premise 2*: In `internal/httpx/sse.go:121-126`, `flush()` calls `s.rc.Flush()` but never calls `s.rc.SetWriteDeadline()`.
   - *Inference*: A malicious client opening an SSE stream can restrict its TCP receive window to near zero. Because no write deadline is enforced, the connection, goroutine, and upstream stream remain open indefinitely, enabling socket exhaustion DoS.
2. **Quadratic Memory in Streaming Delts (OA-SEC-NET-04)**:
   - *Premise 1*: In `internal/adapter/anthropic.go:474, 484`, `text.String()` and `reasoning.String()` are called on every token delta.
   - *Premise 2*: Copying an accumulating string of length $N$ across $k$ deltas requires $\sum_{i=1}^k i \cdot \text{chunk\_size} = O(N^2)$ byte allocations.
   - *Inference*: A 3,000-token stream produces tens of megabytes of short-lived heap allocations, generating severe GC pressure and potential out-of-memory crashes under concurrent load.
3. **Hardcoded PostgreSQL Default Password (OA-SEC-DEP-01)**:
   - *Premise 1*: `docker-compose.yml:35, 79` defaults `POSTGRES_PASSWORD` to `"obsidian"`.
   - *Inference*: Deployments launched without an explicit `.env` file run PostgreSQL with well-known default credentials, exposing database contents to adjacent containers or local network actors.

---

## 3. Caveats

1. **Localhost Inference Engines Requirement**:
   - The permission of plain HTTP for loopback (`localhost`, `127.0.0.1`, `::1`) in `NormalizeBaseURL` is an intentional architectural requirement supporting local inference runtimes (Ollama, vLLM). Blocking loopback would break local model functionality; remediation must specifically target cloud metadata and link-local addresses while preserving loopback connectivity.
2. **Refusal UX Design Decision**:
   - The lack of an immediate redirect in `router.beforeEach` for `/admin` is not an oversight but an intentional UX design choice to render `<UnauthorizedModal />` over the chat UI. Eliminating the chunk download requires creating a separate unauthenticated refusal route or accepting the UX trade-off.
3. **Test Suite Coupling in Sanitizer Tests**:
   - Setting `target="_blank"` on links in `web/src/lib/safe-intro.ts:copySafeLink` will break `web/test/safe-intro.test.ts:106` unless `expect(a.hasAttribute('target')).toBe(false)` is updated in tandem to `expect(a.getAttribute('target')).toBe('_blank')`.

---

## 4. Conclusion & Findings Table

| Vulnerability ID | Title | Dimension | Severity | CVSS v3.1 | CWE | OWASP Top 10 | Primary Location |
|---|---|---|---|---|---|---|---|
| **OA-SEC-NET-01** | SSRF & Cloud IMDS Exposure via Provider Base URL | Dim 5: Network | **High** | 7.2 | CWE-918 | A10:2021 – SSRF | `internal/adapter/wire.go:37-69`, `internal/adapter/adapter.go:371-390` |
| **OA-SEC-NET-02** | Missing Server `WriteTimeout` & Streaming Deadlines (Slow-Read DoS) | Dim 6: DoS | **High** | 7.5 | CWE-400, CWE-770 | A05:2021 – Security Misconfig | `cmd/server/main.go:97-102`, `internal/httpx/sse.go:121-126` |
| **OA-SEC-DEP-01** | Hardcoded Insecure Default PostgreSQL Password in Docker Compose | Dim 6: Deploy | **High** | 7.5 | CWE-1188, CWE-798 | A07:2021 – Auth Failures | `docker-compose.yml:35, 79` |
| **OA-SEC-NET-03** | Upstream API Key and Credential Reflection in Error Responses | Dim 5: Network | **Medium** | 5.3 | CWE-200, CWE-532 | A01:2021 – Broken Access Control | `internal/chat/chat.go:964-987`, `internal/adapter/wire.go:262-308` |
| **OA-SEC-NET-04** | Unbounded Memory Accumulation & Quadratic String Copies in SSE | Dim 6: DoS | **Medium** | 5.3 | CWE-400, CWE-770 | A05:2021 – Security Misconfig | `internal/adapter/anthropic.go:474, 484`, `internal/adapter/wire.go:326-352` |
| **OA-SEC-FE-01** | Missing `target="_blank"` on Sanitized Operator Links (Same-Window Phishing) | Dim 4: Frontend | **Medium** | 6.5 | CWE-1022, CWE-601 | A01:2021 – Broken Access Control | `web/src/lib/safe-intro.ts:61-76`, `web/test/safe-intro.test.ts:106` |
| **OA-SEC-FE-02** | Unbounded Recursion in DOM Sanitizer (Client Denial of Service) | Dim 4: Frontend | **Medium** | 5.3 | CWE-674 | A05:2021 – Security Misconfig | `web/src/lib/safe-intro.ts:38-59`, `web/src/views/SafeIntro.vue:15-20` |
| **OA-SEC-DEP-02** | Overly Permissive Default `OBSIDIAN_TRUSTED_PROXIES` (IP Spoofing) | Dim 5: Network | **Medium** | 5.3 | CWE-345 | A07:2021 – Auth Failures | `docker-compose.yml:47`, `internal/httpx/clientip.go:33-37` |
| **OA-SEC-NET-05** | Incomplete Reserved Header Denylist Permitting Hop-by-Hop Headers | Dim 5: Network | **Low** | 3.7 | CWE-444, CWE-113 | A03:2021 – Injection | `internal/provider/provider.go:76-83, 460-484` |
| **OA-SEC-FE-03** | Client-Side Router Incomplete Admin Check & Dead Auth Guard Function | Dim 4: Frontend | **Low** | 3.1 | CWE-639, CWE-561 | A01:2021 – Broken Access Control | `web/src/router/index.ts:68-71, 84-110, 119-121` |
| **OA-SEC-DEP-03** | Missing Web Server Container Healthcheck Mechanism | Dim 6: Deploy | **Low** | 3.1 | CWE-1384 | A05:2021 – Security Misconfig | `Dockerfile:69-71`, `docker-compose.yml:12-72` |
| **OA-SEC-DEP-04** | Fragile Host Timezone Bind Mounts in Docker Compose | Dim 6: Deploy | **Low** | 2.5 | CWE-1258 | A05:2021 – Security Misconfig | `docker-compose.yml:66-67, 82-83` |
| **OA-SEC-DEP-05** | Database Connection Lacks Role Separation Between DDL and DML | Dim 6: Deploy | **Low** | 2.5 | CWE-272 | A01:2021 – Least Privilege | `docker-compose.yml:35, 78-79`, `internal/database/migrations` |

---

## 5. Detailed Technical Breakdown & Zero-Dependency Remediation

### 5.1 OA-SEC-NET-01: SSRF & Cloud IMDS Exposure
- **Description**: `NormalizeBaseURL` permits any `https://` destination (or `http://` if `allow_insecure: true`), allowing connections to IPv4 metadata (`169.254.169.254`), IPv6 link-local (`fe80::/10`), and AWS EC2 IPv6 IMDS (`fd00:ec2::254` ULA). Furthermore, `DialContext` lacks IP verification at socket creation, leaving the server vulnerable to DNS rebinding.
- **Remediation**:
  1. In `internal/adapter/wire.go`, add `isBlockedMetadataHost`:
     ```go
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
     Enforce `if isBlockedMetadataHost(parsed.Hostname()) { return "", fmt.Errorf(...) }` in `NormalizeBaseURL`.
  2. In `internal/adapter/adapter.go:NewRegistry`, configure `http.Transport.DialContext` to resolve hostnames via `net.DefaultResolver.LookupIP` and verify that none of the resolved addresses match link-local or metadata targets prior to establishing the TCP socket.

### 5.2 OA-SEC-NET-02: Missing Server `WriteTimeout` & Streaming Deadlines (Slow-Read DoS)
- **Description**: `cmd/server/main.go` leaves `WriteTimeout` unset (0), and `internal/httpx/sse.go:flush()` never invokes `s.rc.SetWriteDeadline()`. A client reading 1 byte per minute keeps TCP connections and goroutines pinned indefinitely.
- **Remediation**:
  In `internal/httpx/sse.go`, apply a write deadline of 30 seconds before every flush:
  ```go
  const sseWriteDeadline = 30 * time.Second

  func (s *SSE) flush() error {
      _ = s.rc.SetWriteDeadline(time.Now().Add(sseWriteDeadline))
      if err := s.rc.Flush(); err != nil {
          return fmt.Errorf("sse: flush: %w", err)
      }
      return nil
  }
  ```

### 5.3 OA-SEC-DEP-01: Hardcoded Insecure Default PostgreSQL Password
- **Description**: `docker-compose.yml:35, 79` sets `POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-obsidian}`. Operators starting the stack without `.env` run with known credentials.
- **Remediation**:
  Make parameter expansion mandatory:
  ```yaml
  POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in environment}
  ```

### 5.4 OA-SEC-NET-03: Upstream Credential Reflection
- **Description**: In `internal/chat/chat.go:964-987`, `Describe` returns `upstream.Message` verbatim for `ErrorUpstream` and `default`. If an upstream echoes `Authorization: Bearer <key>`, secret tokens leak to users and logs.
- **Remediation**:
  In `internal/chat/chat.go` and `internal/adapter/wire.go`, scrub secret keys (`resolved.Upstream.APIKey`) and `Bearer [A-Za-z0-9._-]+` patterns from error messages prior to saving or returning them.

### 5.5 OA-SEC-NET-04: Quadratic String Copies in Long-Running SSE Streams
- **Description**: In `internal/adapter/anthropic.go:474, 484`, `result.Text = text.String()` and `result.Reasoning = reasoning.String()` are called on every token delta, generating $O(N^2)$ memory allocations. Additionally, `internal/adapter/wire.go:326` reads response bodies without `io.LimitReader`.
- **Remediation**:
  In `internal/adapter/anthropic.go`, populate `result.Text` and `result.Reasoning` once after the event loop completes. In `internal/adapter/wire.go:readEventStream`, wrap `body` with `io.LimitReader(body, 64*1024*1024)`.

### 5.6 OA-SEC-FE-01: Missing `target="_blank"` on Sanitized Operator Links
- **Description**: `web/src/lib/safe-intro.ts:61-76` sets `rel="noopener noreferrer"` but omits `target="_blank"`. External links navigate within the existing tab.
- **Remediation**:
  Set `target.setAttribute('target', '_blank')` in `copySafeLink`. Simultaneously update `web/test/safe-intro.test.ts:106` to `expect(a.getAttribute('target')).toBe('_blank')` to preserve test suite integrity.

### 5.7 OA-SEC-FE-02: Unbounded Recursion in DOM Sanitizer
- **Description**: `copySafeIntroChildren` in `web/src/lib/safe-intro.ts:38-59` lacks recursion depth bounding along both its unpacking branch (line 50) and its allowed element branch (line 56).
- **Remediation**:
  Add a `depth = 0` parameter and enforce `if (depth > 32) return;`. Increment `depth + 1` across both recursive calls.

### 5.8 OA-SEC-DEP-02: Overly Permissive Default `OBSIDIAN_TRUSTED_PROXIES`
- **Description**: `docker-compose.yml:47` trusts all RFC 1918 subnets by default, allowing IP spoofing on internal networks.
- **Remediation**:
  Default `OBSIDIAN_TRUSTED_PROXIES` to `127.0.0.1/32` and document explicit configuration for container reverse proxies.

### 5.9 OA-SEC-NET-05: Incomplete Reserved Header Denylist
- **Description**: `internal/provider/provider.go:76-83` omits hop-by-hop headers (`Connection`, `Transfer-Encoding`, `Upgrade`, etc.).
- **Remediation**:
  Expand `reservedHeaders` map to denylist hop-by-hop headers according to RFC 7230.

### 5.10 OA-SEC-FE-03: Client-Side Router Guard vs Refusal UX
- **Description**: `router.beforeEach` omits `record.meta['admin']` check, and `mayAdminister()` is dead code (`web/src/router/index.ts:119`). Refusal modal is rendered in `AdminPage.vue:230-233`.
- **Remediation**:
  Remove dead function `mayAdminister()` from `web/src/router/index.ts` to clean up code, preserving the documented informative refusal modal UX.

### 5.11 OA-SEC-DEP-03 to OA-SEC-DEP-05: Deployment Ergonomics & Least Privilege
- **OA-SEC-DEP-03**: Implement a native `-health` flag in the Go binary to allow Docker healthchecks on distroless images without external utilities.
- **OA-SEC-DEP-04**: Remove `/etc/localtime` and `/etc/timezone` bind mounts from `docker-compose.yml:66-67, 82-83`.
- **OA-SEC-DEP-05**: Document least-privilege PostgreSQL roles separating DDL migration privileges from runtime DML operations.

---

## 6. Verification Method

To independently verify all findings and test suite compatibility:
1. **Source Line Verification**:
   - Inspect `internal/adapter/wire.go:37-69` and `internal/adapter/adapter.go:371-390` to verify lack of IP/metadata blocking.
   - Inspect `cmd/server/main.go:97-102` and `internal/httpx/sse.go:121-126` to verify missing `WriteTimeout` and absent `SetWriteDeadline`.
   - Inspect `web/src/lib/safe-intro.ts:38-59, 61-76` to verify dual-path unbounded recursion and missing `target="_blank"`.
   - Inspect `internal/chat/chat.go:964-987` to verify unredacted `upstream.Message` propagation in `Describe`.
   - Inspect `docker-compose.yml:35, 47, 79` to verify default passwords and trusted proxy subnets.
2. **Automated Unit & Integration Test Suite**:
   Run the project test suites:
   ```bash
   go test -v ./internal/httpx/...
   go test -count=1 ./...
   ```
3. **Frontend Tests**:
   ```bash
   cd web && npm test
   ```
   Confirm all assertions pass and notice `web/test/safe-intro.test.ts:106` asserts `expect(a.hasAttribute('target')).toBe(false)`.
