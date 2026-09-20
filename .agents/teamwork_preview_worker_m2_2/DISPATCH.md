# Task Dispatch: Worker (Iteration 2 - Working Tree Remediation & Deliverable Polish)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Context & Inputs
1. Read `E:/Project/ObsidianArc/.agents/ORIGINAL_REQUEST.md` and `E:/Project/ObsidianArc/AGENTS.md`.
2. Read the Forensic Auditor's report at:
   `E:/Project/ObsidianArc/.agents/teamwork_preview_auditor_m3_1/handoff.md`
3. Read the Iteration 2 Explorer reports:
   - Explorer 1 (r2): `E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_1_r2/handoff.md`
   - Explorer 2 (r2): `E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_2_r2/handoff.md`
   - Explorer 3 (r2): `E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3_r2/handoff.md`
4. Deliverable target: `E:/Project/ObsidianArc/docs/SECURITY_AUDIT.md`.
5. Working directory: `E:/Project/ObsidianArc/.agents/teamwork_preview_worker_m2_2/`.

## Strict Constraints
1. STRICTLY READ-ONLY for all existing code, configurations, and assets. Zero modifications to existing files. (`git diff HEAD -- ':!docs/SECURITY_AUDIT.md'` must be completely empty).
2. The ONLY file to be created or modified outside `.agents/` is `docs/SECURITY_AUDIT.md`.
3. Do NOT delete or damage `docs/SECURITY_AUDIT.md` or `.agents/`.
4. Use LF line endings in `docs/SECURITY_AUDIT.md`.

## Remediation Mandate:
Execute the 4-stage remediation protocol established by the Explorers:

### Stage 1: Restore Working Tree Cleanliness
Discard tracked working tree drift to match `HEAD`:
```bash
git restore .
```
*(Note: `git restore .` acts strictly on tracked files; untracked deliverable `docs/SECURITY_AUDIT.md` and `.agents/` are completely unaffected).*

### Stage 2: Remove Root Untracked Test Debris
Remove `node_modules/` in the repository root if present:
```powershell
if (Test-Path "node_modules") { Remove-Item -Recurse -Force "node_modules" }
```

### Stage 3: Polish `docs/SECURITY_AUDIT.md` Citations & LF Line Endings
In `docs/SECURITY_AUDIT.md`:
- Update line 140, 432, 434: change `internal/feedback/http.go:142` to `internal/feedback/http.go:144` to reflect the exact line in commit `3cd9ea3`.
- Confirm strict LF line endings (`\n`) throughout `docs/SECURITY_AUDIT.md`.

### Stage 4: Comprehensive Verification
Verify all gates:
1. `git diff HEAD -- ':!docs/SECURITY_AUDIT.md'` produces ZERO output.
2. `git status --porcelain` shows ONLY `?? .agents/` and `?? docs/SECURITY_AUDIT.md`.
3. `node -e "const fs = require('fs'); const buf = fs.readFileSync('docs/SECURITY_AUDIT.md'); console.log('Has CR:', buf.includes(13));"` outputs `Has CR: false`.
4. `go test ./...` passes 100%.
5. `npm test --prefix web` passes 100%.
6. `npm run typecheck --prefix web` passes 100%.

Write your handoff report to `E:/Project/ObsidianArc/.agents/teamwork_preview_worker_m2_2/handoff.md` and send a message to parent.

## 2026-09-19T15:22:02Z
You are the Remediation Worker on Iteration 2 of the Obsidian Arc Security Audit team.
Your working directory is: E:/Project/ObsidianArc/.agents/teamwork_preview_worker_m2_2/
Read your task dispatch file at: E:/Project/ObsidianArc/.agents/teamwork_preview_worker_m2_2/DISPATCH.md
Read E:/Project/ObsidianArc/.agents/ORIGINAL_REQUEST.md and E:/Project/ObsidianArc/AGENTS.md.
Read Explorer handoff reports at:
- E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_1_r2/handoff.md
- E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_2_r2/handoff.md
- E:/Project/ObsidianArc/.agents/teamwork_preview_explorer_m1_3_r2/handoff.md
And Auditor report at: E:/Project/ObsidianArc/.agents/teamwork_preview_auditor_m3_1/handoff.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

STRICT CONSTRAINTS:
1. STRICTLY READ-ONLY for all existing code, configurations, and assets. Zero modifications to existing files. (`git diff HEAD -- ':!docs/SECURITY_AUDIT.md'` must be completely empty).
2. The ONLY file to be created or modified outside .agents/ is docs/SECURITY_AUDIT.md.
3. Do NOT delete docs/SECURITY_AUDIT.md or .agents/.
4. Ensure docs/SECURITY_AUDIT.md uses strict LF line endings.

Task:
Execute the 4-stage remediation protocol:
1. Restore tracked files to HEAD: git restore .
2. Clean untracked root node_modules/ if present.
3. Polish docs/SECURITY_AUDIT.md (update internal/feedback/http.go:142 -> 144 at lines 140, 432, 434) and verify LF line endings.
4. Run verification gates (git diff HEAD -- ':!docs/SECURITY_AUDIT.md' is empty, git status porcelain is clean, go test ./... passes, npm test --prefix web passes).
Write your handoff report to E:/Project/ObsidianArc/.agents/teamwork_preview_worker_m2_2/handoff.md and send a completion message back to parent.

