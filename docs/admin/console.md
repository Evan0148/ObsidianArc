# 管理控制台

后台的**终端**页是一个命令行控制台，能做后台界面能做的每一件事。它不是一个新的权限体系：每条命令都是后台 API 的调用方，走的是和界面完全相同的那套接口、那套鉴权中间件、那套逐路由的权限判断。

同一个控制台还可以通过 SSH 访问，不需要浏览器。

## 它能做什么

控制台完整覆盖全部 58 个后台管理接口与 40 个用户端接口，共 104 条命令。按功能分组：

| 分组 | 命令 | 适用对象 |
| --- | --- | --- |
| 账户 (Accounts) | `user list` `show` `create` `edit` `delete` `passwd` `keys` `key-revoke` `chats` `transcript` `cards` | 管理员 (`users`) |
| 用户组 (Groups) | `group list` `show` `create` `edit` `delete` `members` `assign` | 管理员 (`groups`) |
| 服务商与模型 (Catalogue) | `provider list` `show` `create` `edit` `delete` `detect` · `model list` `show` `create` `edit` `delete` `order` `import` | 管理员 (`providers`/`models`) |
| 运维 (Operations) | `dash` `res` · `health status` `probe` `reset` · `usage summary` `rpm` `records` `reset` · `quota list` `set` `delete` · `code list` `create` `redemptions` `delete` · `log list` `facets` `prune` | 管理员 (`dashboard`/`resources`/`availability`/`usage`/`codes`/`logs`) |
| 实例 (Instance) | `setting list` `get` `set` `import` · `notice list` `create` `edit` `delete` · `security events` `review` · `attachment purge` · `meta` `refs` `member-options` | 管理员 (`settings`/`announcements`/`security`) |
| 会话 (Session) | `help` `clear` `exit` `whoami` `version` `history` `watch` `lang` `format` `echo` | 所有用户 |
| 个人对话 (Chat) | `chat list` `show` `rename` `delete` `delete-all` `archive` `unarchive` | 所有用户 (Anyone) |
| 项目管理 (Projects) | `project list` `show` `create` `edit` `delete` | 所有用户 (Anyone) |
| 额度与卡密 (Credit) | `credit show` `history` `cards` `use` `redeem` | 所有用户 (Anyone) |
| API 密钥 (Keys) | `key list` `create` `edit` `delete` | 所有用户 (Anyone) |
| 偏好与资料 (Profile) | `pref list` `set` `wallpaper-clear` · `me show` `edit` `passwd` `verify` | 所有用户 (Anyone) |
| 备份与恢复 (Backup) | `backup export` `backup import` | 所有用户 (Anyone) |

完整列表以 `help` 为准——它只列出**你当前有权执行**的命令，不会把未授权的命令呈现出来。每个命令均可通过 `<command> --help` 或 `help <command>` 查看中英双语的详细参数、用法和示例。

## 权限

控制台不会扩大任何人的权限，这一点是由结构保证的，而不是靠另写一份检查：命令并不直接读写数据库，而是在进程内向后台 API 发起一次请求，因此 `auth.RequireAdmin` 和该路由自己的权限包装照常执行。

后果是：

- 只持有 `users` 授权的管理员，在控制台里同样只能操作账户。`model list` 会被拒绝，`setting set` 也会。
- 他们看不到这些命令。`help`、Tab 补全和网页端的命令索引都会把无权执行的命令隐藏掉。
- 超级管理员在控制台里拥有和后台界面一样的完整范围，不多也不少。

被拒绝时的提示会直接说明缺少哪一项授权：

```
permission denied: this command needs the "models" grant
```

## 危险操作

所有不可逆的命令（每一个 `delete`、`user passwd`、`log prune`、`attachment purge`、`health reset`、`usage reset`、`setting import`）都必须显式加 `--yes`（或 `-y`）才会执行。缺少时命令拒绝执行并说明原因，不会有确认弹窗。

## 网页终端

后台左侧栏进入**终端**。中间区域会变成一个可交互的终端。

- 顶部是标签栏，可以开多个标签页。标签数量变多时会自行收缩，不会顶出容器。双击可重命名，`×` 关闭。
- 右上角是设置按钮：字体、字号、行高、字符间距、光标样式与闪烁、回滚缓冲区行数、时间戳、连字。设置保存在浏览器本地。
- 终端的透明度和模糊**不在这里设置**，它跟随全局的壁纸设置，和后台其他面板保持一致。

键盘：

| 按键 | 作用 |
| --- | --- |
| `Enter` | 执行 |
| `Shift+Enter` | 换行（粘贴多行 JSON 时用得上） |
| `↑` / `↓` | 翻阅历史 |
| `Tab` | 补全 |
| `Ctrl+L` | 清屏 |
| `Esc` / `Ctrl+C` | 取消正在执行的命令 |
| `Alt+T` / `Alt+W` | 新建 / 关闭标签页 |

`Ctrl+T` 和 `Ctrl+W` 归浏览器所有，因此新建和关闭标签页用 `Alt`。

## 通过 SSH 连接

设置 `OBSIDIAN_SSH_ADDR` 后（例如 `:2222`），管理员可以直接连接控制台：

```bash
ssh 管理员用户名@你的域名 -p 2222
```

**连上的是控制台，不是服务器。** 这个监听端口只能执行控制台命令，没有 shell，没有文件传输，没有端口转发——服务端代码里根本没有通向 `os/exec` 的路径。

认证使用和网页登录相同的密码，共用同一套失败次数限制，所以在两个入口猜密码花的是同一份预算。非管理员账号会被拒绝，提示和密码错误完全一致。

也可以只跑一条命令，这让控制台可以被脚本调用：

```bash
ssh 管理员用户名@你的域名 -p 2222 'user list --json' | jq '.users[].username'
```

所有命令都支持 `--json`，输出的是后台 API 的原始载荷。

主机密钥在首次启动时生成并保存在数据目录（`ssh_host_ed25519_key`，权限 0600），指纹会打印在启动日志里，也可以用 `help ssh` 查看——首次连接时请核对。数据目录丢失会导致指纹变化，客户端的 known_hosts 校验会因此报警。

相关配置项见[配置参考](../guide/configuration)。

## 审计

每一条改变状态的命令都会记入安全事件（`console_command`），包含执行人、来源（`web` 或 `ssh`）、IP 和命令原文。`--password`、`--api-key` 这类敏感参数在记录前已被替换为 `***`。

纯查询类的会话命令（`help`、`whoami`、`version` 等）不记录。

在[安全](../admin/overview)页或用 `security events --event console_command` 查看。

## help

`help` 是这个控制台的说明书，中英双语，按当前会话语言显示：

| 命令 | 作用 |
| --- | --- |
| `help` | 按分组列出你能执行的命令 |
| `help <命令>` | 用法、参数、选项、示例、所需权限、调用的接口 |
| `help <名词>` | 列出该名词下的所有动作，如 `help user` |
| `help -k <关键字>` | 搜索命令，中英文均可 |
| `help keys` | 键盘快捷键 |
| `help ssh` | 如何通过 SSH 连接，含主机指纹 |

任意命令加 `--help` 等同于 `help <命令>`。

SSH 会话的语言取自客户端传来的 `LANG`：

```bash
ssh -o SetEnv=LANG=zh_CN.UTF-8 管理员用户名@你的域名 -p 2222 'help user edit'
```

网页终端跟随界面语言。会话内也可以用 `lang zh` / `lang en` 临时切换。
