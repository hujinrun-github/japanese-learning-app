# Notion 笔记关联设计方案

## 1. 现状结论

本平台已经具备完整的本地笔记能力，不需要重新建设笔记 CRUD：

- 后端模型、服务和 HTTP API：`internal/module/note/`。
- SQLite 持久化和迁移：`internal/data/note_store.go`、`internal/data/migrations/006_notes.sql`。
- React 页面：`front/react/src/pages/note/`，路由为 `/notes`、`/notes/new`、`/notes/:id`。
- 桌面 TopNavBar 和移动 BottomTabBar 都已经提供“笔记”入口。
- 笔记支持单词/语法/句子类型、标签、全文搜索、双向关联和复习队列。

当前缺少的是外部笔记同步：代码中没有 Notion API 客户端、同步目标、绑定关系或同步状态表。因此本方案只扩展“同步层”，不替换现有本地笔记模型。

## 2. 参考 all_note 的取舍

参考 `/Users/tylerhu/GitProject/all_note/README.md` 的 Notion 设计，保留以下原则：

1. Notion Data Source 是同步目标，而不是每篇笔记各自保存一套连接配置。
2. 一篇本地笔记最多绑定一个同步目标；新建笔记默认不同步。
3. 默认方向为本地 → Notion；Notion → 本地通过用户显式触发的手动拉取完成。
4. token 不进入浏览器、不写入数据库；数据库只保存允许读取 token 的环境变量名。
5. 每个外部页面由 page id 唯一识别，避免标题变化造成重复导入。
6. 删除采用“发现 → 用户确认”的两阶段流程，不因一次 API 查询遗漏就直接删除本地笔记。

针对本平台做两项调整：

- `user_id` 必须出现在同步目标、绑定和同步状态中，避免不同账号看到或操作彼此的 Notion 配置。
- 本地笔记继续保留 `type`、`reference_id`、复习数据等学习字段；只有标题、标签和正文同步到 Notion，学习状态不写入 Notion。

## 3. MVP 范围

### 3.1 本期实现

- 一个用户可以创建多个 Notion 同步目标，每个目标对应一个 Data Source。
- 在笔记详情页选择“不同步”或一个 Notion 目标。
- 测试目标连接。
- 同步单篇笔记到 Notion：首次创建 page，后续更新同一 page。
- 按目标批量推送已绑定笔记。
- 从目标手动拉取：导入新页面、更新已有页面。
- 标签过滤：配置 `sync_tags` 后，手动拉取只处理包含这些标签的页面。
- 展示最近同步时间、方向和错误信息。
- 发现 Notion 页面被归档/删除后，列出待确认项；用户可以确认删除本地笔记或恢复 Notion 页面。

### 3.2 暂不实现

- Notion OAuth 多租户授权。
- 实时 webhook、后台定时同步和冲突合并编辑器。
- 图片、文件、数据库嵌套视图等复杂 Notion blocks 的完整 round-trip。
- 把本地 SM-2 复习字段写入 Notion。

## 4. 用户流程

### 4.1 配置同步目标

1. 用户在 Notion 创建 Internal Integration，复制 token。
2. 用户在 Notion 创建或选择一个 Data Source，并将 Integration 添加到 Connections。
3. 部署者把 token 写入后端环境变量，例如 `JLA_NOTION_TOKEN`，重启服务。
4. 用户在“设置 → Notion 同步”填写目标名称、Data Source ID、token 环境变量名和标题属性名。
5. 点击“测试连接”，后端使用该环境变量访问 Data Source，返回可理解的错误。

Data Source ID 支持直接粘贴 ID 或 Notion URL；后端提取并规范化 32 位/UUID 格式。

### 4.2 绑定并推送

1. 打开本地笔记详情。
2. 在“Notion 同步”面板选择一个目标；选择“不同步”即可解除绑定。
3. 点击“同步此笔记”推送当前本地内容。
4. 后续修改本地笔记只更新已绑定的 page，不创建重复页面。

### 4.3 手动拉取

1. 在同步目标面板点击“从 Notion 拉取”。
2. Notion 新页面导入为本地 `sentence` 笔记，标题为空时跳过并记录错误。
3. 带有本地 ID 属性的页面更新对应本地笔记；没有 ID 属性时按 `target_id + page_id` 的外部声明查找。
4. 两端都修改时以 Notion 内容为准，并在同步结果中显示更新数量。
5. Notion 页面被删除/归档时进入待确认列表，用户确认后才软删除本地笔记。

## 5. 数据模型

### 5.1 SQLite 迁移（建议 `014_note_notion_sync.sql`）

```sql
CREATE TABLE IF NOT EXISTS note_sync_targets (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id               INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type                  TEXT NOT NULL CHECK(type = 'notion'),
    name                  TEXT NOT NULL,
    data_source_id        TEXT NOT NULL,
    token_env             TEXT NOT NULL DEFAULT 'JLA_NOTION_TOKEN',
    title_property        TEXT NOT NULL DEFAULT 'Name',
    tags_property         TEXT NOT NULL DEFAULT 'Tags',
    flow_id_property     TEXT NOT NULL DEFAULT '',
    sync_tags_json        TEXT NOT NULL DEFAULT '[]',
    enabled               INTEGER NOT NULL DEFAULT 1,
    auto_push             INTEGER NOT NULL DEFAULT 0,
    created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at            DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_note_sync_targets_user_identity
    ON note_sync_targets(user_id, data_source_id);

CREATE TABLE IF NOT EXISTS note_sync_bindings (
    note_id       INTEGER PRIMARY KEY REFERENCES notes(id) ON DELETE CASCADE,
    target_id     INTEGER NOT NULL REFERENCES note_sync_targets(id) ON DELETE RESTRICT,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_note_sync_bindings_target
    ON note_sync_bindings(target_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS note_sync_states (
    note_id           INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_id         INTEGER NOT NULL REFERENCES note_sync_targets(id) ON DELETE CASCADE,
    external_page_id  TEXT NOT NULL,
    external_url      TEXT NOT NULL DEFAULT '',
    local_hash        TEXT NOT NULL DEFAULT '',
    external_hash     TEXT NOT NULL DEFAULT '',
    last_direction    TEXT CHECK(last_direction IN ('push', 'pull', 'delete', 'restore')),
    last_synced_at    DATETIME,
    last_error        TEXT NOT NULL DEFAULT '',
    pending_delete    INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(note_id, target_id),
    UNIQUE(target_id, external_page_id)
);

CREATE INDEX IF NOT EXISTS idx_note_sync_states_pending_delete
    ON note_sync_states(target_id, pending_delete, updated_at DESC);
```

`user_id` 不在绑定表中重复保存，通过 `notes.user_id` 和 `note_sync_targets.user_id` 的服务层校验保证隔离；如果后续改为直接 SQL JOIN，可补充冗余列和复合外键。

PostgreSQL 使用同样的实体，类型分别替换为 `BIGINT GENERATED ... AS IDENTITY`、`BOOLEAN`、`TIMESTAMPTZ` 和 `JSONB/TEXT[]`，并作为新的有序 migration 文件加入 `internal/data/migrations/postgres/schema/`。不修改已有 `notes` 表，避免影响学习进度和现有迁移 checksum。

### 5.2 同步配置安全

- `token_env` 只允许 `JLA_NOTION_TOKEN` 或 `JLA_*_NOTION_TOKEN` 格式，拒绝任意环境变量名。
- token 只在后端进程内读取，日志中不得打印 token、Authorization header 或完整 Notion response。
- API 返回目标时只返回 `token_env` 和 `has_token`，绝不返回 token。
- Data Source ID、page id 和 URL 可以保存；它们本身不是授权凭据。
- 如果未来需要真正的多用户 OAuth，应新增加密密钥和 token rotation 方案，不把明文 token 直接塞入现有表。

## 6. API 设计

所有接口都挂在现有认证中间件下，建议使用独立前缀 `/api/v1/note-sync`，避免与 `/api/v1/notes/{id}` 的路径匹配产生歧义。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/note-sync/targets` | 当前用户的目标列表（脱敏） |
| `POST` | `/api/v1/note-sync/targets` | 创建目标 |
| `PUT` | `/api/v1/note-sync/targets/{id}` | 更新非身份配置；已绑定目标禁止更换 Data Source ID |
| `DELETE` | `/api/v1/note-sync/targets/{id}` | 无绑定时删除，仍有绑定时返回 409 |
| `POST` | `/api/v1/note-sync/targets/test` | 测试 token 和 Data Source 权限，不落库 |
| `GET` | `/api/v1/notes/{id}/sync-binding` | 查询笔记当前绑定 |
| `PUT` | `/api/v1/notes/{id}/sync-binding` | 设置或替换绑定，`target_id: null` 表示不同步 |
| `DELETE` | `/api/v1/notes/{id}/sync-binding` | 解除绑定并保留历史状态为 suppress |
| `GET` | `/api/v1/notes/{id}/sync-state` | 查询 page id、URL、最后同步结果 |
| `POST` | `/api/v1/note-sync/notes/{id}/push` | 推送单篇笔记 |
| `POST` | `/api/v1/note-sync/targets/{id}/push` | 批量推送该目标已绑定笔记 |
| `POST` | `/api/v1/note-sync/targets/{id}/pull` | 手动拉取目标内容 |
| `GET` | `/api/v1/note-sync/targets/{id}/deletions` | 待确认删除列表 |
| `POST` | `/api/v1/note-sync/targets/{id}/deletions/{noteID}/confirm` | 确认本地软删除 |
| `POST` | `/api/v1/note-sync/targets/{id}/deletions/{noteID}/restore` | 恢复 Notion page 并重新同步 |

请求/响应示例：

```json
POST /api/v1/note-sync/targets
{
  "name": "我的日语 Notion",
  "data_source_id": "https://www.notion.so/...",
  "token_env": "JLA_NOTION_TOKEN",
  "title_property": "Name",
  "tags_property": "Tags",
  "sync_tags": ["日语", "N5"]
}
```

```json
{
  "id": 1,
  "type": "notion",
  "name": "我的日语 Notion",
  "data_source_id": "...",
  "token_env": "JLA_NOTION_TOKEN",
  "has_token": true,
  "enabled": true
}
```

同步接口统一返回统计和逐条错误，避免批量同步因单篇失败而丢失整体结果：

```json
{
  "created": 2,
  "updated": 5,
  "imported": 1,
  "skipped": 1,
  "pending_delete": 0,
  "errors": [{"note_id": 12, "message": "title property is missing"}]
}
```

## 7. Notion API 适配

新增 `internal/notion/`（纯标准库）或 `internal/integration/notion/` 包，职责保持窄小：

- `Client`：统一设置 `Authorization: Bearer`、`Notion-Version: 2025-09-03`、超时和 429/5xx 重试。
- `QueryDataSource`：分页查询 Data Source，处理 `has_more/next_cursor`。
- `RetrievePageBlocks`：读取页面正文 blocks。
- `CreatePage` / `UpdatePage` / `RestorePage`：页面写入和恢复。
- `Blocks`：只支持段落、标题、bulleted/numbered list、to-do、quote、code、divider；遇到不支持的 block 返回可识别错误，禁止静默覆盖。

字段映射：

| 本地字段 | Notion 位置 | 规则 |
|---|---|---|
| `title` | 配置的 title property（默认 `Name`） | 必须是 `title` 类型 |
| `tags` | 配置的 tags property（默认 `Tags`） | `multi_select`；不存在时可只读同步并给出警告 |
| `content` | 页面 children blocks | Markdown 子集转 blocks |
| `source_text` | 正文开头的引用块 | 使用 `> 来源：...`，避免新增数据库属性 |
| `id` | 可选 flow ID property | `rich_text`，用于重建本地状态 |

禁止把 `mastery_level`、`next_review_at`、`ease_factor` 等学习调度字段写入 Notion，避免外部编辑破坏复习逻辑。

## 8. 分层实现位置

遵循当前 `Handler → Service → Adapter/Store` 结构：

```text
internal/module/notesync/
  model.go       # SyncTarget、Binding、SyncState、SyncResult
  service.go     # 绑定校验、推送/拉取策略、冲突和删除策略
  handler.go     # /api/v1/note-sync/* 及笔记绑定端点

internal/integration/notion/
  client.go      # HTTP 客户端
  blocks.go      # Markdown ↔ Notion blocks
  config.go      # token_env、Data Source ID、属性配置校验

internal/data/
  note_sync_store.go
  migrations/014_note_notion_sync.sql

internal/data/postgres/
  note_sync_store.go
  migrations/schema/006_note_notion_sync.sql
```

`NoteStoreInterface` 不扩展同步方法；同步服务通过已有的 `NoteStoreInterface` 读取/更新笔记，并依赖新的 `NoteSyncStoreInterface` 保存同步元数据。这样不会把 Notion 细节污染本地笔记 CRUD。

## 9. 前端设计

### 9.1 设置页

在 `front/react/src/pages/settings/SettingsPage.tsx` 新增“Notion 同步”卡片：

- 目标列表和“新建目标”。
- Data Source ID 文本框（支持粘贴 URL）。
- token 环境变量文本框 + `datalist`，默认 `JLA_NOTION_TOKEN`；不使用固定下拉框。
- 标题属性名、标签属性名、拉取标签输入。
- “保存”“测试连接”“删除目标”按钮。
- token 只显示“已配置/未配置”，不回显值。

### 9.2 笔记详情页

在 `NoteDetailPage` 增加“Notion 同步”区域：

- 下拉选择目标（包含“不同步”）。
- 显示绑定 page URL、上次同步方向/时间、错误。
- “同步此笔记”按钮。
- 目标被删除/禁用时显示明确的重新绑定提示。

### 9.3 笔记列表页

增加可选的同步状态徽标：未绑定、已同步、待处理错误、Notion 待确认删除。图标使用已有 emoji/SVG 体系，不引入冷门 Unicode 符号。

### 9.4 移动端

不新增顶层导航入口；同步能力放在现有笔记详情和设置页，确保 TopNavBar 与 BottomTabBar 的“笔记”入口继续一致。

## 10. 冲突、删除和幂等

- 推送前以 `(target_id, note_id)` 查找状态；有 `external_page_id` 就调用 Update，否则 Create 并记录 page id。
- `local_hash` 对规范化后的 `title + tags + source_text + content` 计算 SHA-256；`external_hash` 对拉取后的同一表示计算。
- 拉取时 Notion 优先：更新本地内容并刷新 `local_hash/external_hash`。
- Notion 查询不到曾同步的 page 时只写 `pending_delete = 1`；用户确认后调用本地软删除。
- 解除绑定保留已有同步状态，后续 pull 不会凭旧 Flow ID 自动重新绑定。
- 批量接口支持重复调用；同一 page 不得创建第二条本地笔记或第二个 Notion page。
- 所有跨用户查询必须同时校验 `user_id`，不能只按自增 ID 查询。

## 11. 测试计划（表格驱动优先）

### 后端

- `config_test.go`：token_env 白名单、Data Source URL/ID 规范化。
- `notion/client_test.go`：使用 `httptest.Server` 验证请求头、分页、429 重试、401/404/429 错误映射。
- `blocks_test.go`：支持 block 的 Markdown 往返和不支持 block 的拒绝策略。
- `note_sync_store_test.go`：目标/绑定/状态的用户隔离、唯一约束、待确认删除。
- `notesync/service_test.go`：创建/更新/导入/冲突/解除绑定/幂等；使用表格驱动测试和 fake Notion gateway。
- Handler 集成测试：未认证 401、跨用户 404/403、目标被绑定时删除 409、批量返回逐条错误。

### 前端

- 设置页：保存后 token 不回显，测试连接成功/失败提示。
- 详情页：绑定、解除绑定、单篇同步和错误状态。
- 导航回归：现有 `/notes` 入口在桌面和移动端保持可达。

验证命令：

```bash
make test
make web
```

## 12. 分阶段交付

1. **Phase 1：模型与只读配置**
   - SQLite/PostgreSQL migration、目标 CRUD、绑定 CRUD、连接测试。
2. **Phase 2：单篇推送**
   - Notion client、Markdown blocks、Create/Update、详情页按钮和状态。
3. **Phase 3：手动拉取**
   - 新页面导入、标签过滤、Notion 优先冲突策略。
4. **Phase 4：安全删除与批量操作**
   - pending delete、确认/恢复、批量 push/pull、错误汇总。
5. **Phase 5：可选增强**
   - 自动推送、OAuth、多租户 token 加密、更多 Notion blocks。

每个阶段独立迁移、测试和回滚；未配置 Notion token 时，本地笔记所有功能必须保持可用。

