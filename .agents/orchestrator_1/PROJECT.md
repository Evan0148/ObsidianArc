# Project: Obsidian Arc Security Audit & Risk Assessment

## Architecture & Scope
Obsidian Arc (Single Go binary + embedded Vue 3 SPA + Postgres 16 / SQLite 3).
Read-only source code audit across all subsystems:
- Backend: `cmd/`, `internal/server/`, `internal/auth/`, `internal/admin/`, `internal/apikey/`, `internal/conversation/`, `internal/database/`, `internal/httpx/`, `internal/provider/`, `internal/adapter/`, `internal/quota/`, `internal/id/`, `internal/config/`.
- Frontend: `web/src/` (`lib/safe-intro.ts`, `chat/markdown.ts`, `stores/`, router, components).
- Deployment: Dockerfile, docker-compose.yml, migrations.

## Feature / Dimension Inventory
| # | Dimension / Scope | Description | Assigned Agent | Status |
|---|---|---|---|---|
| 1 | Dimension 1: Auth & Access Control | Password hashing (Argon2id), session management, token/API key issue/verify, IDOR, /api/admin endpoint protection | Explorer 1 | PLANNED |
| 2 | Dimension 2: Concurrency & DB | Check-then-write row locks vs mutexes, short transactions, quota/spending race conditions, rollback handling | Explorer 2 | PLANNED |
| 3 | Dimension 3: Input & Injection | SQL placeholder usage, multi-dialect safety, path traversal, escaping | Explorer 2 | PLANNED |
| 4 | Dimension 4: Frontend & Client | innerHTML/v-html violations, markdown/math XSS, sensitive data storage/exposure in web/src | Explorer 3 | PLANNED |
| 5 | Dimension 5: Network & Interface | CORS/CSRF, SSE buffering/compression, reverse proxy / header spoofing | Explorer 3 | PLANNED |
| 6 | Dimension 6: DoS & Resources | Request size limits, attachments, slowloris/stream hangs, goroutine leaks | Explorer 3 | PLANNED |
| 7 | Synthesis & Report Generation | Author `docs/SECURITY_AUDIT.md` adhering to R3 structure, verified file:line references, defensible hardening guidance | Worker 1 | PLANNED |
| 8 | Multi-Agent Verification & Gate | Independent review, citation challenge, and forensic integrity audit | Reviewers & Challengers & Auditor | PLANNED |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|---|---|---|---|
| M1 | Multi-Track Exploration | 3 parallel Explorers covering Dimensions 1-6 with verified line citations | None | IN_PROGRESS |
| M2 | Report Drafting | Synthesize verified findings into docs/SECURITY_AUDIT.md | M1 | PLANNED |
| M3 | Rigorous Verification | Reviewers, Challengers, and Auditor verify citations, constraints, and integrity | M2 | PLANNED |
| M4 | Gate & Final Delivery | Gate clearance and final user notification | M3 | PLANNED |

## Code Layout & Constraint Enforcement
- Target Deliverable: `docs/SECURITY_AUDIT.md` ONLY.
- Working metadata: `.agents/` folder only.
- Strict read-only on all other codebase files.
