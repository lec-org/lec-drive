# LecDrive

LEC 个人与团队图片空间，基于 Cloudreve 社区版的二开仓库。领域术语与当前边界见 [CONTEXT.md](CONTEXT.md)。

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

图片上传后右键「获取直链」即可复制，访客无需登录。开发直链依赖本机应用与 SSH 隧道，公网 CDN 尚未配置。界面固定简体中文；团队资产及成员权限尚未实现。

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
