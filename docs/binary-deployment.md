# 二进制部署

生产部署使用 GitHub Actions 在 runner 上完成质量检查、前端构建和 Linux amd64 Go 二进制构建，然后把一个压缩发布包通过 SCP 上传到服务器。服务器不需要下载镜像，也不需要重新编译 Go 或前端代码。

## 发布包

每次发布包包含：

- `bin/server`：学习端 API 二进制
- `bin/admin`：管理端 API 二进制
- `front/dist`：学习端前端静态文件
- `front/admin/dist`：管理端前端静态文件
- 两个 Vite preview 配置和锁文件：仅用于线上提供静态文件与 API 代理

服务器上的发布目录为 `${APP_DIR}/releases/<commit-sha>`，`${APP_DIR}/current` 是当前版本的符号链接。数据库保存在 PostgreSQL，本地音频仍保存在 `${APP_DIR}/data`，不会随版本切换删除。

## GitHub 配置

Actions 需要以下配置：

- `SERVER_HOST`、`SERVER_USER`、`APP_DIR`
- `SERVER_SSH_KEY` 或 `SERVER_PASSWORD`
- `ADMIN_TOKEN`、`JWT_SECRET`

根据环境继续配置 `DATABASE_URL`（必填）、`AUDIO_OBJECT_STORE`、`MINIO_*`、邮件和 AI 相关变量。

生产学习端通过 `https://japanese.jinrunlab.site` 访问，管理端通过 `https://japanese-admin.jinrunlab.site` 访问。未配置 `APP_BASE_URL` 时，部署脚本默认使用学习端公网地址。

### FlowSpace 联动生产配置

本版生产发布要求启用 FlowSpace 联动。可以设置 GitHub 配置 `FLOWSPACE_BASE_URL`、`FLOWSPACE_SOURCE_INSTANCE_ID`，以及 Secret `FLOWSPACE_INTEGRATION_MASTER_KEY`；SSH 部署会把这三项传给学习端 API。

Actions 中未设置的字段，从服务器私有文件 `/home/ubuntu/.config/japanese-learning-app/flowspace.env` 补齐。该文件位于发布目录和 Git 仓库之外，必须由部署用户 `ubuntu` 持有、权限为 `0600`，使用不加引号的 `KEY=value` 行；部署只读取上述三个白名单字段，不会 source 或执行文件内容。文件中的密钥不应复制到公开日志或仓库，已有连接使用的主密钥需跨发布保持不变。

流水线在停止任何现有进程前检查三项配置均完整，URL 必须为 HTTPS 且不含凭据、查询或 fragment，实例 ID 不含空白，主密钥为 64 个十六进制字符。缺失或无效时发布失败，旧服务继续运行；不会默默以“未配置联动”状态上线。

发布前先确认 FlowSpace 接收端已部署并完成 control/tenant 迁移。配对与验收方式见 [FlowSpace 联动说明](spec/flowspace-integration.md)。

线上机器首次部署还需要安装：

- `tar`、`curl`、`lsof`
- Node.js；如果机器没有已安装的前端 Vite 依赖，还需要 npm
- 学习端、管理端的前端依赖（如果缺失，流水线首次部署会根据发布包中的锁文件自动安装）

## 部署流程

推送到 `master` 后，流水线按以下顺序执行：

1. 运行 Go 和前端质量检查。
2. 下载 kuromoji 词典并构建两个前端。
3. 交叉构建 Linux amd64 的两个 Go 二进制。
4. 将二进制和静态文件打成压缩包并上传为 Actions artifact。
5. 通过 SCP 把压缩包上传到服务器 `/tmp/release.tar.gz`。
6. SSH 到服务器，将发布包解压到新的 SHA 目录并原子切换 `current` 链接。
7. 停止旧的四个端口进程，启动两个 Go 二进制和两个 Vite preview 静态服务。
8. 检查 `30081`、`30082` 的 `/healthz` 和两个前端首页，成功后流水线结束。

前端 preview 使用发布包中的 Vite 配置，只读取 CI 生成的静态目录，不会在每次发布时重新下载词典或重新构建。线上不需要执行 `git fetch`，发布包就是本次部署的唯一代码来源。

## 服务器手动检查

```bash
cd /path/to/japanese-learning-app
readlink -f current
ls -lh current/bin current/front/dist current/front/admin/dist

curl --fail http://127.0.0.1:30081/healthz
curl --fail http://127.0.0.1:30082/healthz
curl --fail http://127.0.0.1:35173/
curl --fail http://127.0.0.1:35174/
```

日志位于 `${APP_DIR}/logs/`：

```bash
tail -f logs/server.log logs/admin.log logs/learner-front.log logs/admin-front.log
```

如需回滚，可将 `current` 链接切换到 `releases/` 下的上一版 SHA，然后按同样的端口重启流程启动服务；`data/` 目录不需要回滚。

## 仅更新学习端前端

Go API 与当前线上版本兼容时，可以手动运行 **Deploy learner frontend** 工作流。
它只上传学习端的 `index.html` 和构建后的 `assets`，保留现有词典、公开文件和旧版本资源。
该工作流首次加入或修改时会自动运行，之后可通过 Actions 的 Run workflow 手动触发。

安装前会校验发布文件，备份当前首页后原子替换，并检查正在运行的前端是否返回新首页；
检查失败会恢复原首页。首页备份保存在 `${APP_DIR}/frontend-backups/<commit-sha>/`。
此流程复用现有服务器配置，适用于完整发布包上传缓慢时的前端修复。
