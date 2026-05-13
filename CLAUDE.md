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
## 5. UI 规范
- **图标选用规则**：
  - 优先使用 emoji（兼容性最好）
  - 次选 SVG（需要清晰度时）
  - **禁止使用冷门 Unicode 符号**（如 U+23xx 系列），跨平台可能不可见
- **新增/修改页面时必须检查**：
  - 导航入口完整：桌面 TopNavBar + 移动 BottomTabBar 都有对应入口
  - 用户区域有可点击的个人/首页入口（TopNavBar 右侧）
  - 每个可操作元素有可见的图标或文字标签
