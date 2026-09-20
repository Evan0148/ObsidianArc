# Progress — Explorer 1 (Dimension 1 Security Audit)

- Last visited: 2026-09-19T15:02:00Z
- Status: Deep-dive audit of Dimension 1 (Authentication & Access Control) complete.
- Explored components:
  - `internal/auth/password.go`, `internal/config/config.go` (Argon2id hashing, parameters, bounded semaphore, constant-time compare, dummy verify)
  - `internal/auth/session.go`, `internal/id/id.go`, `internal/auth/service.go`, `internal/auth/http.go` (session generation, SHA-256 hashing at rest, cookie attributes, invalidation, rotation)
  - `internal/auth/limiter.go`, `internal/auth/signup.go`, `internal/auth/verify.go` (rate limiting, attempt backoff, verification tokens)
  - `internal/apikey/apikey.go`, `internal/apikey/http.go` (API key generation, prefix, salted SHA-256 storage, owner row lock, IDOR scoping)
  - `internal/admin/admin.go`, `internal/admin/permissions.go`, `internal/admin/people.go`, `internal/admin/instance.go` (/api/admin route protection, RBAC, population locking, settings import)
  - `internal/conversation/conversation.go`, `internal/conversation/attachment.go`, `internal/card/card.go`, `internal/project/http.go`, `internal/feedback/http.go` (multi-tenant scoping, IDOR verification)
  - `internal/server/server.go`, `internal/server/bootstrap.go`, `internal/server/security_test.go` (route mounting, first-time admin bootstrap, security regression tests)
  - `internal/consolessh/server.go` (SSH console authentication, Reauthorize on each command)
  - `web/src/router/index.ts` (client-side route guard, dead code mayAdminister)
- Current task: Writing comprehensive handoff report to `handoff.md`.
