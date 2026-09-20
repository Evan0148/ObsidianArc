# Task Dispatch: Explorer 3 (Dimension 4 - Frontend/Client, Dimension 5 - Network, Dimension 6 - DoS)

## Mandatory Context
Read `E:/Project/ObsidianArc/.agents/ORIGINAL_REQUEST.md` and `E:/Project/ObsidianArc/AGENTS.md` before starting work.
Existing report for reference: `E:/Project/ObsidianArc/SECURITY_AUDIT_REPORT.md` (read-only reference; independently verify every claim and check for new/unreported issues).

## Working Directory
`E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3/`

## Assigned Scope
Deep-dive read-only security audit of:
**Dimension 4: Frontend & Client Security**:
1. Check for `innerHTML` and `v-html` occurrences in `web/src/`. Verify whether `lib/safe-intro.ts` is the ONLY place and whether its allowlist DOM construction is safe from DOM XSS.
2. Markdown parsing & rendering (`web/src/chat/markdown.ts`, `OaMarkdown.vue`): AST building, link sanitization (`javascript:`, `data:` URI filtering, `target="_blank" rel="noopener noreferrer"`), image tags, HTML tags in markdown.
3. Math rendering: KaTeX/MathML safety in `web/src/chat/math/`.
4. Client-side storage: LocalStorage, SessionStorage, cookies for session tokens or sensitive data.
5. Vue Router navigation guards (`web/src/router/`): admin route protection, redirection attacks.

**Dimension 5: Network & Interface Security**:
1. Server-Side Request Forgery (SSRF): Upstream provider Base URL configuration (`internal/adapter/`, `internal/provider/`). Can an admin or user configure arbitrary Base URLs pointing to localhost, 127.0.0.1, internal VPCs, or cloud metadata endpoints (169.254.169.254)?
2. CORS & CSRF: Cross-origin request policies, SameSite cookie protection, custom header checks.
3. SSE streaming & HTTP compression: `internal/httpx/sse.go`, `internal/httpx/compress.go`. Ensure `text/event-stream` is never buffered/compressed, check flush behavior and chunking.
4. Reverse proxy headers: `X-Forwarded-For`, `X-Real-IP`, client IP extraction in rate limiting or audit logs.

**Dimension 6: Resource Consumption & DoS**:
1. Request size limits: `http.MaxBytesReader` usage on API endpoints, attachment upload size limits, avatar uploads.
2. Slowloris and streaming timeouts: `http.Server` timeouts (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`), streaming connection termination when client disconnects.
3. Goroutine leaks: Background tasks, context cancellation propagation (`r.Context()`), channel operations in streaming or provider communication.
4. Deployment security: `Dockerfile` and `docker-compose.yml` configurations (default credentials, unprivileged user execution, health checks).

## Output Requirements
Write your detailed findings report to `E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3/handoff.md` and update `progress.md`.
Format findings with:
- Title & Severity (Critical / High / Medium / Low / Informational)
- CWE / OWASP category
- Exact File:Line citation
- Technical description
- Attack scenario & exploitability
- Defensible remediation fitting AGENTS.md rules.

## 2026-09-19T14:57:35Z
You are Explorer 3 on the Obsidian Arc Security Audit team.
Your working directory is: E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3/
Read your task dispatch file at: E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3/DISPATCH.md
Also read E:/Project/ObsidianArc/.agents/ORIGINAL_REQUEST.md and E:/Project/ObsidianArc/AGENTS.md.
Existing report for reference: E:/Project/ObsidianArc/SECURITY_AUDIT_REPORT.md (verify every claim against the actual code).

Your task:
Deeply audit Dimension 4: Frontend & Client Security, Dimension 5: Network & Interface Security, Dimension 6: Resource Consumption & DoS.
Inspect files across web/src/ (safe-intro.ts, markdown.ts, math/, stores/, router/), internal/httpx/, internal/server/, internal/provider/, internal/adapter/, cmd/server/, Dockerfile, docker-compose.yml.
Check innerHTML/v-html violations, markdown/math XSS, SSRF via provider Base URLs, SSE buffering/compression, timeouts (ReadHeaderTimeout, WriteTimeout), attachment limits, goroutine leaks, container defaults.
Verify exact line numbers in each file.
Write your complete findings report to E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3/handoff.md and progress in progress.md.
Send a message back to parent when done.
