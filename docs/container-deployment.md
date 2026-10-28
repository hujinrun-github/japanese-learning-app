# 容器化部署

生产部署使用 GitHub Actions 构建四个镜像并推送到 GitHub Container Registry（GHCR），服务器只负责拉取镜像和运行 Docker Compose。

## 容器

| 容器 | 端口 | 作用 |
|---|---:|---|
| `server` | 30081 | 学习端 Go API、迁移和健康检查 |
| `admin-api` | 30082 | 管理后台 Go API |
| `learner-web` | 35173 | 学习端 Nginx 静态文件和 API 反向代理 |
| `admin-web` | 35174 | 管理端 Nginx 静态文件和 API 反向代理 |

PostgreSQL、MinIO 等外部依赖不会由本 Compose 文件创建；通过 `DATABASE_URL`、`MINIO_*` 和 `SHADOWING_DATABASE_URL` 连接现有服务。

## GitHub 配置

Actions 需要以下配置：

- `SERVER_HOST`、`SERVER_USER`、`APP_DIR`
- `SERVER_SSH_KEY` 或 `SERVER_PASSWORD`
- `ADMIN_TOKEN`、`JWT_SECRET`

根据环境继续配置 `RELATIONAL_STORE`、`DATABASE_URL`、`AUDIO_OBJECT_STORE`、`MINIO_*`、邮件和 AI 相关变量。Actions 使用本次运行自带的 `GITHUB_TOKEN` 同时推送和拉取镜像，服务器拉取完成后会自动执行 `docker logout ghcr.io`，因此不需要额外配置 `GHCR_READ_TOKEN`。如果需要在服务器上手动拉取私有镜像，再使用具有 `read:packages` 权限的 PAT 执行 `docker login ghcr.io`。

## 部署流程

推送到 `master` 后，流水线按以下顺序执行：

1. 运行 Go 和前端质量检查。
2. 使用 `deploy/Dockerfile` 构建四个目标镜像。
3. 将镜像以提交 SHA 和 `latest` 标签推送到 GHCR。
4. SSH 到服务器，拉取最新代码和指定 SHA 镜像。
5. 停止旧的宿主机进程和旧 Compose 容器。
6. 执行 `docker compose -f deploy/docker-compose.yml up -d`。
7. 检查四个 `/healthz` 地址后才报告部署成功。

首次切换时，流水线会停止原先由 `nohup go run` 和 `npm run dev` 启动的 30081、30082、35173、35174 进程；`data/` 目录通过 bind mount 保留 SQLite 数据和本地音频文件。

## 服务器手动检查

```bash
cd /path/to/japanese-learning-app
export REGISTRY_IMAGE=ghcr.io/hujinrun-github/japanese-learning-app
export IMAGE_TAG=<commit-sha>
docker login ghcr.io
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.yml pull
docker compose -f deploy/docker-compose.yml up -d
docker compose -f deploy/docker-compose.yml ps
```

日志可通过以下命令查看：

```bash
docker compose -f deploy/docker-compose.yml logs -f server admin-api learner-web admin-web
```
