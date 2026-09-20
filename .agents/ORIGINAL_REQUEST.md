# Original User Request

## Initial Request — 2026-09-19T14:55:42Z

对 Obsidian Arc 项目（Go 后端 + Vue 3 前端）展开全量源码阅读与全方位安全风险评估，在严格不修改既有代码的前提下，输出包含漏洞定级、精确代码定位、攻击场景推演及防御加固指南的专业安全审计报告至 docs/SECURITY_AUDIT.md。

Working directory: E:/Project/ObsidianArc
Integrity mode: development

## Requirements

### R1. 全量只读代码安全审查
完整阅读并审查仓库内所有关键代码，严格保持既有代码只读（禁止对已有源码、配置、静态资源进行任何修改）：
- 后端架构：cmd/、internal/server/、internal/auth/、internal/admin/、internal/apikey/、internal/conversation/、internal/database/、internal/httpx/。
- 前端架构：web/src/（特别是 web/src/lib/safe-intro.ts、web/src/chat/markdown.ts、web/src/stores/、路由鉴权及组件渲染）。
- 部署与环境：Docker、PostgreSQL/SQLite 兼容适配层及静态配置。

### R2. 全方位多维度安全风险建模与漏洞挖掘
系统性评估以下核心安全维度：
1. 身份认证与访问控制：密码哈希与存储、会话管理、Token/API Key 签发与验证、水平与垂直越权（用户隔离、管理端 /api/admin 端点防护）。
2. 并发控制与数据库一致性：检查是否严格遵守 AGENTS.md 规范（Check-then-write 必须使用数据库行级锁，禁止单一 sync.Mutex；事务绝对不跨外部 Provider 调用；防重放与超额扣费/并发竞争）。
3. 输入校验与注入防御：SQL 占位符使用（防注入与多方言适配）、路径遍历、危险字符处理与上下文解析。
4. 前端渲染与客户端安全：严格检查 innerHTML、v-html 违规情况（全库除 safe-intro.ts 白名单外必须为零）、Markdown/Math 渲染防 XSS、敏感信息在前端存储/内存中的暴露。
5. 网络与接口安全：CORS/CSRF 策略、流式 SSE (text/event-stream) 缓冲区与压缩配置（防缓冲阻塞）、反向代理与 Header 伪造防范。
6. 资源消耗与服务拒绝 (DoS)：未限制大小的请求体/附件上传、流式连接挂起/慢速连接攻击、Goroutine 泄漏与死锁风险。

### R3. 标准化安全评估报告与防御性加固建议
将审计成果写入 docs/SECURITY_AUDIT.md，报告必须具备高专业度与清晰结构：
- 执行摘要：项目安全全貌评估、风险分布矩阵与核心结论。
- 漏洞发现明细表：按严重性评级（Critical / High / Medium / Low / Informational）分类：
  - 漏洞名称与 CWE/OWASP 分类
  - 精确代码位置（文件路径与行号引用，如 internal/auth/service.go:L120-L135）
  - 漏洞成因分析与理论攻击场景/触发条件
  - 潜在业务影响与可利用性分析
- 防御加固与修复方案：遵循 Obsidian Arc 极简架构（零额外依赖、无 ORM、LF 换行符、行级锁），提供具体的修复模式或代码指导。

## Acceptance Criteria

### 代码完整性
- [ ] 既有源代码文件完全未被修改（git diff HEAD -- ':!docs/SECURITY_AUDIT.md' 保持为空）。
- [ ] 审查过程中不产生任何多余的临时文件或测试垃圾文件。

### 覆盖率与完整度
- [ ] 核心 Go 模块（auth, admin, apikey, conversation, database, httpx, server）与 Vue 核心模块（safe-intro, markdown, stores, router）均完成全量阅读与审计，无遗漏关键链路。
- [ ] 6 个核心安全维度（认证越权、并发锁、输入注入、XSS/客户端、网络传输、DoS/资源耗尽）均包含明确的审计结论与佐证。

### 报告质量与客观验证
- [ ] 审计报告保存于 docs/SECURITY_AUDIT.md。
- [ ] 报告中引用的每一个文件路径与代码行号均真实存在于项目中，不存在虚构或幻觉代码。
- [ ] 明确区分“确定性漏洞”、“设计权衡”与“防御强化建议”，避免无依据的泛化误报。
- [ ] 修复建议完全契合项目的技术栈与设计规范（无额外第三方库，数据库锁替代内存锁）。
