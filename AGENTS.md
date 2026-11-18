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
## 5. PostgreSQL 数据库与迁移
- 唯一数据库为 PostgreSQL；主服务、管理端、导入工具和集成测试使用同一套数据访问层。
- 运行连接使用 `DATABASE_URL`，测试连接使用 `DATABASE_URL_TEST`，二者必须是不同数据库。
- 测试数据库名以 `_test` 结尾或包含 `_test_`；测试通过 `internal/testdb` 创建独立 schema，禁止清空 `public` 或业务 schema。
- `make test` 会准备独立测试库；缺少配置或连接失败时必须失败，不能静默跳过集成验证。
- 迁移位于 `internal/data/migrations/postgres/schema/`，由 `schema_migrations` 跟踪文件名与校验和；已发布迁移不能修改，结构变更新增文件。
- 数据库密码仅通过环境变量或被忽略的本地配置提供，不写入代码或提交。

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

### 7.1 PostgreSQL 操作规则
- SQL 参数使用 `$1`、`$2` 等占位符；新增行获取 ID 使用 `RETURNING id`。
- 并发下的“首次创建”使用 `ON CONFLICT DO NOTHING`，避免读后覆盖已保存的学习记录。
- 所有数据库集成测试通过 `internal/testdb` 使用独立测试库和临时 schema；不要自行删除共享 schema。

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
- 两个服务共享 PostgreSQL 迁移锁，由 advisory lock 串行执行迁移
- 前端代码修改无需重启，Vite HMR 自动生效
