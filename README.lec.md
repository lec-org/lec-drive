# LecDrive

LEC 个人与团队图片空间，基于 Cloudreve 社区版的二开仓库。领域术语与当前边界见 [CONTEXT.md](CONTEXT.md)。

## 源码组织

- Go 后端保留上游目录与模块路径，便于后续同步。
- React 前端位于 `assets/`，作为普通源码目录随本仓提交。
- 后端上游为 `cloudreve/cloudreve`，初始提交为 `9f1e0b8`。
- 前端上游为 `cloudreve/frontend`，初始提交为 `19da0fe1ecd40971fafa813983d769fdce41573c`。
- 上游 README 与主仓 GPL-3.0 许可证保留；前端来自上游后端固定的源码版本。

## 本地预览

需要 Go 1.26.5（支持 Go 自动下载工具链）、Node.js 22+（本地已验证 24.14.1）、Corepack 和 zip。前端使用独立的 Yarn 1.22.22 锁文件，不加入平台根 pnpm workspace。

在平台根目录执行：

```sh
pnpm lec-drive:build
pnpm lec-drive:dev
```

访问 `http://localhost:5212`，首次进入按页面提示创建本地管理员。预览仅监听回环地址；SQLite、上传文件和配置保存在本仓忽略的 `data/` 目录中，停止后重启继续复用。

当前预览提供社区版的文件管理、上传和外链能力，界面固定中文。LecSSO 登录、团队资产权限与 CDN 接入仍待二开。
