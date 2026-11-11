# LecDrive

LEC 团队图片空间，基于 Cloudreve 社区版的二开仓库。领域术语与当前边界见 [CONTEXT.md](CONTEXT.md)。

## 源码组织

- Go 后端保留上游目录与模块路径，便于后续同步。
- React 前端位于 `assets/`，作为普通源码目录随本仓提交。
- 后端上游为 `cloudreve/cloudreve`，初始提交为 `9f1e0b8`。
- 前端上游为 `cloudreve/frontend`，初始提交为 `19da0fe1ecd40971fafa813983d769fdce41573c`。
- 上游 README 与主仓 GPL-3.0 许可证保留；前端来自上游后端固定的源码版本。

## 构建与运行

需要 Go 1.26.5（支持 Go 自动下载工具链）、Node.js 22+（本地已验证 24.14.1）、Corepack 和 zip。前端使用独立的 Yarn 1.22.22 锁文件，不加入平台根 pnpm workspace。

在平台根目录执行：

```sh
pnpm lec-drive:build
pnpm boxpro:connect           # 保持隧道，另开终端运行应用
pnpm lec-drive:dev:boxpro
```

访问 `http://127.0.0.1:5212`，通过 LecSSO 登录。BoxPro 上运行身份服务、PostgreSQL 与 RustFS，本机仅运行 Drive；配置与临时缓存放在平台忽略目录 `.build/boxpro-dev/`，详见平台 `deploy/boxpro/README.md`。

图片上传后右键「获取直链」即可复制，访客无需登录。开发直链依赖本机应用与 SSH 隧道，公网 CDN 尚未配置。界面固定简体中文；当前仅开放团队空间：成员上传、查看和复制直链，只有管理员可删除或恢复。管理员删除后保留 30 天，期间原直链继续有效，回收站不能提前清空。

## LecSSO 配置

创建传统 Web 客户端，回调登记为 `<LEC_DRIVE_ORIGIN>/session/lec-sso/callback`。进程环境需要：

| 配置 | 用途 |
| --- | --- |
| `LEC_SSO_ISSUER` | LecSSO OIDC issuer，默认要求 HTTPS。 |
| `LEC_SSO_CLIENT_ID`、`LEC_SSO_CLIENT_SECRET` | Drive 独立客户端凭据，密钥仅交给后端。 |
| `LEC_DRIVE_ORIGIN` | Drive 完整站点来源，不带路径。 |
| `LEC_SSO_ORGANIZATION` | 限制可以登录的组织；工作室环境设为 `lec-studio`。 |
| `LEC_DRIVE_ADMIN_SUBJECTS` | 管理员 subject 白名单，逗号分隔；未列出的用户按普通组同步。 |
| `LEC_SSO_ALLOW_INSECURE_LOCALHOST` | 非生产环境显式设为 `1`，且 issuer 和 Drive 均为回环地址时，允许本机 HTTP 隧道。 |
| `CR_SETTING_register_enabled` | 工作室设为 `0`，关闭本地注册入口。 |

SSO 身份按 issuer 与 subject 唯一保存，不与同邮箱的本地账号自动合并。授权码使用 PKCE，校验 state、nonce、签名、issuer、audience、有效期和组织，兑换成功后签发原有 Drive 会话。SSO 首次登录不会自动授予管理员。

数据库连接及会话密钥使用权限为 `0600` 的 INI 文件，避免上游环境变量覆盖日志输出数据库凭据。独立无 SSO 预览仍可运行 `pnpm lec-drive:dev`，使用单独的本地 SQLite 数据，不与 BoxPro 开发环境混用。

## 验证

```sh
go test ./pkg/lecsso ./service/user
```

前端 Vite 构建通过。当前上游前端类型基线有 127 条诊断，本次集成未增加诊断；完整类型检查仍需单独清理上游问题。

## 团队文件与删除规则

每位成员仍使用独立的 SSO 账号。首次登录按 issuer 与工作室组织创建或复用无登录入口的团队资产账号，文件操作与总容量统一使用该账号；管理员权限继续取自实际成员账号。已有个人文件原位迁入团队子目录，保留文件编号和直链，有未完成上传时会拒绝迁移。

普通成员不能删除文件或目录。管理员删除后进入 30 天回收期，可以恢复，原直链继续可用。禁止提前清空、跳过回收站永久删除、单独删除团队直链或版本、强制回收仍被文件引用的底层对象。到期后链接和恢复失效，由现有定时任务回收对象；实体回收可能晚于到期时间一个任务调度周期。

运行 `go test ./inventory ./pkg/filemanager/manager ./service/user ./pkg/lecsso` 验证组织隔离、成员权限、原位迁移、文件与目录的直链保留、恢复和到期行为。

二开版本使用 `4.19.1+lec.1` 标记本次团队字段迁移。请通过平台 `pnpm lec-drive:build` 构建，使后端注入与前端一致的版本号；上游按版本标记决定是否执行表结构迁移，沿用旧后端版本号不会新增字段。
