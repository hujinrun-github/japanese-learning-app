# Japanese Learning App

面向「边上班边学日语」人群的 Web 应用，覆盖 JLPT N5～N1 全级别。支持单词记忆（SM-2 间隔重复）、语法学习、课文阅读（振り仮名）、口语练习和 AI 写作批改。

## 技术栈

| 维度 | 选型 |
|---|---|
| 后端 | Go 1.22 + 标准库 `net/http` |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go 实现） |
| 前端 | React 18 + TypeScript + Vite |
| 认证 | JWT（HMAC-SHA256） |
| AI | Anthropic Claude API（写作批改，可选） |
| i18n | i18next + react-i18next（zh / ja / en） |

## 快速开始

### 环境要求

- Go ≥ 1.22
- Node.js ≥ 18（前端开发）

### 启动后端

```bash
# 最简启动（默认监听 :8080）
make run

# 或指定配置
JWT_SECRET=your-secret DB_PATH=./data/app.db go run ./backend/cmd/server/
```

### 导入种子数据

```bash
make seed-all    # 导入全部（词汇 + 语法 + 课文 + 口语 + 写作）
make seed        # 仅导入词汇（N5/N4）
```

### 启动前端

```bash
cd front/react
npm install
npm run dev      # http://localhost:5173，/api 代理至 :8080
```

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP 监听地址 |
| `DB_PATH` | `./data/app.db` | SQLite 数据库路径 |
| `JWT_SECRET` | `change-me-in-production` | 生产环境必须修改 |
| `LOG_LEVEL` | `INFO` | DEBUG / INFO / WARN / ERROR |
| `AI_API_KEY` | `""` | Claude API 密钥（空则使用 Stub 评分） |
| `SMTP_HOST` | `""` | SMTP 服务器（密码重置邮件，空则使用 Stub） |

## 功能模块

| 模块 | 说明 |
|---|---|
| **单词记忆** | SM-2 间隔重复算法，闪卡翻转复习，按 JLPT 等级筛选 |
| **语法学习** | N5-N1 语法点 + 即时测验（填空/选择），学习状态追踪 |
| **课文阅读** | 振り仮名（ruby）标注，中日翻译切换，已读标记 |
| **口语练习** | 影子跟读 + 自由朗读，波形评分 0-100 |
| **写作练习** | 输入练习 + AI 造句批改（语法/词汇/修正建议） |
| **学习总结** | 会话记录与聚合分析（强项/弱项/建议） |

## 项目结构

```
├── backend/cmd/server/    # 服务入口
├── internal/
│   ├── cli/               # CLI 子命令（导入种子数据等）
│   ├── config/            # 环境变量配置
│   ├── data/              # 数据访问层（Store + 迁移）
│   ├── httputil/          # HTTP 响应工具
│   └── module/            # 业务模块（word/grammar/lesson/speaking/writing/user/summary）
├── front/
│   └── react/             # React SPA（当前主前端）
├── data/seed/             # 种子数据 JSON
├── specs/                 # 需求文档
└── docs/architecture.md   # 架构文档
```

## Makefile 速查

```bash
make run           # 启动开发服务器
make build         # 编译 bin/server
make test          # 运行所有测试（含集成测试）
make lint          # 静态分析
make seed-all      # 导入全部种子数据
make front-build   # 编译前端 TypeScript
make clean         # 清理构建产物
```

## 运行测试

```bash
make test                        # 全部测试
go test ./internal/module/word/... -v   # 指定包
go test ./internal/data/... -run TestWordStore_GetByID -v  # 指定用例
```

测试策略：表格驱动 + 真实 SQLite（不使用 Mock），禁用缓存（`-count=1`）。
