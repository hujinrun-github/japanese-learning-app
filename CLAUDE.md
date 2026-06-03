# ==================================
# japanese-learning-app 项目上下文总入口
# ==================================

# --- 核心原则导入 (最高优先级) ---
# 明确导入项目宪法，确保AI在思考任何问题前，都已加载核心原则。
@./constitution.md

# --- 核心使命与角色设定 ---
你是一个资深的Go语言工程师，正在协助我开发一个名为 "japanese-learning-app" 的日语学习应用。
你的所有行动都必须严格遵守上面导入的项目宪法。

---
## 1. 技术栈与环境
- **语言**: Go (版本 >= 1.24)
- **构建与测试**:
  - 使用 `Makefile` 进行标准化操作。
  - 运行所有测试: `make test`
  - 构建Web服务: `make web`

---
## 2. Git与版本控制
- **Commit Message规范**: 严格遵循 Conventional Commits 规范。
  - 格式: `<type>(<scope>): <subject>`
  - 当被要求生成commit message时，必须遵循此格式。

---
## 3. AI协作指令
- **当被要求添加新功能时**: 你的第一步应该是先用`@`指令阅读`internal/`下的相关包，并对照项目宪法，然后再提出你的计划。
- **当被要求编写测试时**: 你应该优先编写**表格驱动测试（Table-Driven Tests）**。
- **当被要求构建项目时**: 你应该优先提议使用`Makefile`中定义好的命令。

---
## 4. 数据导入规范
- **导入新词库前必须校验数据格式**，确保与已有数据（N5）对齐：
  - 每个词必须有中文释义（meaning），不能只有英文
  - 每个词必须有 ≥1 条例句（examples_json 非空），例句格式：`{japanese, chinese}`
  - 字段必须齐全：`kanji_form`, `reading`, `meaning`, `part_of_speech`, `examples_json`
  - 校验脚本位置：`scripts/validate_words.py`，导入前先跑，不通过则拒绝导入

---
## 5. 数据库迁移系统问题（已知缺陷）
- **迁移系统无追踪机制**（`internal/data/db.go`）：每次启动重新执行所有 `.sql` 文件，仅靠错误字符串 `"duplicate column name"` 判断跳过。
  - `CREATE TABLE IF NOT EXISTS` → 静默成功，日志显示 "applied"
  - `ALTER TABLE ADD COLUMN` → 列已存在时报错 "duplicate column name"，被正确跳过
  - **002_seed.sql** 的 `INSERT OR IGNORE INTO words` 在 004 的 UNIQUE 索引之前执行，每次启动会重复插入 60 条种子词，产生重复数据
- **003_fix_writing_questions.sql** 包含 `DROP TABLE + CREATE TABLE`，每次启动会重建 `writing_questions` 表
- **用户进度数据（word_records、grammar_records 等）未被迁移破坏**，不会丢失
- **修复重复词**（保留最小 ID 的原始行）：
  ```sql
  DELETE FROM words WHERE id NOT IN (
    SELECT MIN(id) FROM words GROUP BY kanji_form, reading
  );
  ```
  **注意**：仅在应用首次启动后需要执行，后续若再重启会再次产生重复
- **TODO**: 添加 `schema_migrations` 表实现真正的迁移追踪；将 002 的 UNIQUE 约束提前或让 seed 真正幂等

---
## 6. UI 规范
- **图标选用规则**：
  - 优先使用 emoji（兼容性最好）
  - 次选 SVG（需要清晰度时）
  - **禁止使用冷门 Unicode 符号**（如 U+23xx 系列），跨平台可能不可见
- **新增/修改页面时必须检查**：
  - 导航入口完整：桌面 TopNavBar + 移动 BottomTabBar 都有对应入口
  - 用户区域有可点击的个人/首页入口（TopNavBar 右侧）
  - 每个可操作元素有可见的图标或文字标签

---
## 7. 经验教训：可避免的问题（开发前必读）

### 7.1 SQLite 操作铁律
- **`ALTER TABLE ADD COLUMN` 不支持函数表达式作为 DEFAULT**。如 `DEFAULT (datetime('now'))` 会报错 `non-constant default`。正确做法：加可空列 `ALTER TABLE t ADD COLUMN c DATETIME`，再 `UPDATE t SET c = datetime('now') WHERE c IS NULL`
- **多进程共享 SQLite 必须加 `PRAGMA busy_timeout = 5000`**，否则同时启动会报 `SQLITE_BUSY`。在 `OpenDB()` 中 Ping 之后、WAL 之前设置
- `db.Exec()` 支持多语句（用 `;` 分隔），但迁移文件按文件为单位执行，一个文件内的多语句可以正常工作

### 7.2 音频/文件缓存
- **文件名 = `sha256(text)[:16].wav`**，相同文本产生相同文件名
- **重新生成后必须绕过浏览器缓存**：播放 URL 加 `?t=Date.now()`，否则浏览器命中缓存播放旧文件
- 前端 `crypto.subtle.digest('SHA-256')` 和 Go `sha256.Sum256()` 的 hex 编码前 16 位完全一致（前提是输入相同的 UTF-8 文本）

### 7.3 TTS 配置
- **不确定的值用文本框 + datalist，不要用下拉框**。只有 Provider（vllm/sbv）是真正的固定值。Model、Voice、Speaker、Style 都因服务器而异
- `language="Japanese"` 已在 `NewTTSClient` 中硬编码，但 Qwen TTS 对单词输入仍会产生呼吸声 → 用 `TrimWAVSilence` 后处理裁剪
- 默认 TTS Provider 应是 `vllm`（Qwen TTS），不要设 `sbv`（style-bert-vits2 需要额外部署），在 `openModal()` 中设置

### 7.4 前端状态管理
- React 的 `<>...</>` fragment 不支持 key，遍历时用 `<Fragment key={id}>...</Fragment>`
- 操作有状态的按钮（如播放/停止）需要在事件中 `e.stopPropagation()`，避免触发父级行点击
- 重新生成后先 setState 再 `onRegenerated()` 刷新列表，保证 `regenResult` 在重渲染前已设置

### 7.5 用 sed 修改 Go 代码
- Go 用 tab 缩进，sed 插入的行也需要用 tab
- 改完后必须 `grep` 或 `cat -A` 验证：代码是否在函数内、缩进是否正确
- **改完立即用 `go build` 验证编译**，不要等用户报错

### 7.6 后端迁移/重启流程
- 修改 Go 代码后需要重启对应服务（`make start-backend` / `make start-admin`）
- 两个服务同时启动可能抢迁移锁 → `busy_timeout` 能解决，但最好分开启动间隔 3 秒
- 前端代码修改无需重启，Vite HMR 自动生效
