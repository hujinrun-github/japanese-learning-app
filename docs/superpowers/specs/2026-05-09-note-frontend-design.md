# 笔记系统前端设计方案

> 日期：2026-05-09
> 后端设计：docs/superpowers/specs/2026-05-08-note-system-design.md

---

## 1. 核心定位

为已完成的笔记后端 API 构建 React 前端界面。用户可通过 Web 界面完成笔记的创建、编辑、搜索、关联管理和 SRS 复习。

---

## 2. 页面路由

| 路由 | 页面组件 | 说明 |
|------|----------|------|
| `/notes` | NoteListPage | 搜索栏 + 类型/标签过滤 + 排序 + 分页列表 |
| `/notes/new` | NoteEditorPage | 创建模式，TipTap 富文本编辑器 |
| `/notes/:id` | NoteDetailPage | Markdown 渲染 + 关联展示 + SRS 操作 |
| `/notes/:id/edit` | NoteEditorPage | 编辑模式，复用编辑器组件 |
| `/notes/archive` | NoteArchivePage | 已毕业笔记列表 |

所有路由在 `ProtectedLayout` 内，需登录。

---

## 3. 导航入口

- **顶部导航栏** (`TopNavBar`): 新增"笔记"链接 `/notes`
- **首页** (`HomePage`): 模块卡片区新增笔记卡片（📒），显示笔记总数和待复习数
- **底部导航栏**: 不新增 tab，保持6个不变

---

## 4. 组件树

```
pages/note/
├── NoteListPage.tsx          — 列表页：搜索 + 过滤 + 排序 + 分页
│   └── NoteCard.tsx          —   列表项卡片（标题、类型、标签、SRS状态）
├── NoteEditorPage.tsx        — 编辑/创建页：标题 + 类型 + 标签 + 编辑器
│   └── NoteEditor.tsx        —   TipTap 富文本编辑器（复用）
├── NoteDetailPage.tsx        — 详情页：渲染内容 + 关联 + SRS操作
│   └── NoteLinkPanel.tsx     —   关联管理面板（出向+入向链接）
├── NoteArchivePage.tsx       — 归档页：已毕业笔记列表
└── NoteReviewCard.tsx        — 统一复习队列中的笔记卡片

components/layout/
├── TopNavBar.tsx             — 新增笔记链接
└── BottomTabBar.tsx          — 不变

pages/home/
└── HomePage.tsx              — 新增笔记模块卡片

pages/word/
└── WordDetailPage.tsx        — [新建] 单词详情页 + "创建笔记"按钮 + related_notes

pages/grammar/
└── GrammarDetailPage.tsx     — [修改] 新增"创建笔记"按钮 + related_notes 展示

App.tsx                       — 新增5条路由
types/api.ts                  — 新增笔记相关类型
i18n/locales/{zh,en,ja}.ts   — 新增笔记翻译键
```

---

## 5. API 对接

### 5.1 笔记 CRUD

| 前端操作 | 方法 | 路径 |
|----------|------|------|
| 获取笔记列表 | `GET` | `/api/v1/notes?type=&tag=&sort=&order=&page=&size=` |
| 创建笔记 | `POST` | `/api/v1/notes` |
| 获取笔记详情 | `GET` | `/api/v1/notes/{id}` |
| 更新笔记 | `PUT` | `/api/v1/notes/{id}` |
| 删除笔记 | `DELETE` | `/api/v1/notes/{id}` |

列表响应格式：
```json
{
  "data": {
    "items": [{ Note }],
    "total": 50,
    "page": 1,
    "size": 20
  }
}
```

详情响应格式：
```json
{
  "data": {
    "id": 1,
    "type": "word",
    "title": "...",
    "content": "# Markdown...",
    "source_text": "...",
    "reference_id": 123,
    "reference_type": "word",
    "tags": ["N5", "日常"],
    "mastery_level": 0,
    "next_review_at": null,
    "ease_factor": 2.5,
    "interval": 0,
    "review_history": [],
    "created_at": "...",
    "updated_at": "...",
    "links": [{ NoteLink }],
    "backlinks": [{ NoteLink }]
  }
}
```

### 5.2 搜索

| 前端操作 | 方法 | 路径 |
|------|------|------|
| 全文搜索 | `GET` | `/api/v1/notes/search?q=keyword` |

响应直接返回 `Note[]` 数组（最多50条）。

### 5.3 关联管理

| 前端操作 | 方法 | 路径 | Body |
|------|------|------|------|
| 创建关联 | `POST` | `/api/v1/notes/{id}/links` | `{"target_note_id": 2, "relation": "related"}` |
| 删除关联 | `DELETE` | `/api/v1/notes/{id}/links/{linkId}` | - |

### 5.4 SRS 复习

| 前端操作 | 方法 | 路径 | Body |
|------|------|------|------|
| 加入复习 | `POST` | `/api/v1/notes/{id}/promote` | - |
| 退出复习 | `DELETE` | `/api/v1/notes/{id}/promote` | - |
| 评分 | `POST` | `/api/v1/notes/{id}/review` | `{"rating": "easy"\|"normal"\|"hard"}` |
| 回炉重练 | `POST` | `/api/v1/notes/{id}/recycle` | - |
| 笔记复习队列 | `GET` | `/api/v1/notes/review-queue` | - |
| 已归档列表 | `GET` | `/api/v1/notes/archive?page=&size=` | - |

### 5.5 统一复习队列

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/review/queue?level=N5` | 合并 word + note 卡片 |

响应为 `ReviewCard[]`，`card_type` 区分 `"word"` / `"note"`。

### 5.6 标签

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/notes/tags` | 返回用户所有标签列表 |

### 5.7 跨模块关联笔记

word/grammar 详情接口已返回 `related_notes` 字段（`NoteDigest[]`，最多5条），前端直接展示即可。

---

## 6. TypeScript 类型定义

```typescript
// 新增到 types/api.ts

type NoteType = 'word' | 'grammar' | 'sentence'

type LinkRelation = 'related' | 'uses_word' | 'uses_grammar' | 'context'

interface Note {
  id: number
  type: NoteType
  title: string
  content: string          // Markdown
  source_text: string
  reference_id?: number
  reference_type?: string  // 'word' | 'grammar' | 'lesson'
  tags: string[]
  // SRS
  mastery_level: number
  next_review_at?: string
  ease_factor: number
  interval: number
  review_history: ReviewEvent[]
  created_at: string
  updated_at: string
}

interface ReviewEvent {
  rating: string
  reviewed_at: string
}

interface NoteLink {
  id: number
  note_id: number
  target_note_id: number
  relation: LinkRelation
  target_note?: NoteDigest
}

interface NoteDetail extends Note {
  links: NoteLink[]       // outgoing
  backlinks: NoteLink[]   // incoming
}

interface NoteDigest {
  id: number
  title: string
  type: NoteType
}

interface NoteListResponse {
  items: Note[]
  total: number
  page: number
  size: number
}

interface ReviewCard {
  card_type: 'word' | 'note'
  word_card?: WordCard
  note_card?: Note          // Note 对象本身，包含 title/content/tags 等
  is_new: boolean
}
```

---

## 7. 关键交互流

### 7.1 一键创建笔记（从单词/语法详情页）

```
[单词详情页] → 点击"📒 创建笔记"按钮
  → POST /api/v1/notes { type: "word", title: word.kanji_form,
      reference_id: word.id, reference_type: "word", content: "", tags: [] }
  → 创建成功，获取 note.id
  → navigate(`/notes/${note.id}/edit`)
```

### 7.2 笔记列表页

```
[笔记列表页]
  ├── 顶栏搜索框 → GET /api/v1/notes/search?q=...
  ├── 类型过滤 (单词/语法/句子) → GET /api/v1/notes?type=...
  ├── 标签过滤 → GET /api/v1/notes?tag=...
  ├── 排序切换 (创建时间/更新时间/复习时间)
  └── 点击卡片 → navigate(`/notes/${id}`)
```

### 7.3 笔记详情页 SRS 操作

```
[详情页]
  ├── 未加入复习 → 显示"加入复习"按钮 → POST promote
  ├── 复习中 → 显示 mastery 进度条 + next_review_at + "退出复习"按钮
  └── 已毕业 → 显示"已毕业"徽标 + "回炉重练"按钮 → POST recycle
```

### 7.4 统一复习队列中的笔记卡片

```
[复习队列页面] → 遍历 cards[]
  ├── card_type === "word" → 渲染现有 WordCard 翻牌界面
  └── card_type === "note" → 渲染 NoteReviewCard
      ├── 正面：标题 + 类型标签 + source_text
      ├── 翻牌后：渲染 content (Markdown → HTML)
      └── 评分按钮：easy / normal / hard
```

---

## 8. 富文本编辑器

使用 **TipTap**（基于 ProseMirror），底层存储 Markdown。

### 依赖

```json
{
  "@tiptap/react": "^2.x",
  "@tiptap/starter-kit": "^2.x",
  "@tiptap/extension-placeholder": "^2.x"
}
```

### NoteEditor 组件设计

```tsx
// 创建和编辑复用同一组件
interface NoteEditorProps {
  initialNote?: Note    // 有值 = 编辑模式，无值 = 创建模式
  // 一键创建时预填字段
  prefilled?: {
    type?: NoteType
    title?: string
    reference_id?: number
    reference_type?: string
  }
}
```

工具栏：粗体 / 斜体 / 标题 / 列表 / 引用 / 代码块（TipTap starter-kit 默认工具栏）

编辑器上方：标题输入框、类型选择（word/grammar/sentence）、标签输入、来源文本输入框

### Mode 切换逻辑

| 路由 | 模式 | 操作 |
|------|------|------|
| `/notes/new?type=word&title=食べる&ref_id=123&ref_type=word` | 创建（来自单词页一键创建） | POST 创建 → 跳转编辑 |
| `/notes/new` | 创建（空白） | POST 创建 → 跳转详情 |
| `/notes/:id/edit` | 编辑 | PUT 更新 → 跳转详情 |

---

## 9. 导航改动

### TopNavBar

在现有链接列表中加入笔记：

```tsx
{ to: '/notes', key: 'nav.notes', icon: '📒' }
```

### HomePage

在 MODULE_CONFIG 数组中新增：

```tsx
{ key: 'note', labelKey: 'home.modules.note', icon: '📒', to: '/notes' }
```

笔记模块的 `due_count` 来自笔记复习队列数量。

---

## 10. 搜索体验

列表页顶栏搜索框：

- 输入关键词 → 回车触发搜索 → 调用 `GET /api/v1/notes/search?q=`
- 搜索模式下显示搜索结果（替换当前列表），清除搜索回到列表
- 搜索结果按 FTS5 相关性排序

---

## 11. 关联管理交互

在详情页的 NoteLinkPanel 中：

- **查看关联**：展开出向链接和入向链接列表，每条显示笔记标题 + 类型 + 关联类型，点击跳转
- **添加关联**：搜索框 + 下拉选择目标笔记 → 选择关系类型 → POST 创建关联
- **删除关联**：每条关联旁有删除按钮 → DELETE 删除

---

## 12. 文件结构

```
front/react/src/
├── pages/note/
│   ├── NoteListPage.tsx
│   ├── NoteListPage.module.css
│   ├── NoteEditorPage.tsx
│   ├── NoteEditorPage.module.css
│   ├── NoteEditor.tsx              — TipTap 编辑器组件
│   ├── NoteEditor.module.css
│   ├── NoteDetailPage.tsx
│   ├── NoteDetailPage.module.css
│   ├── NoteCard.tsx
│   ├── NoteCard.module.css
│   ├── NoteLinkPanel.tsx
│   ├── NoteLinkPanel.module.css
│   ├── NoteReviewCard.tsx
│   ├── NoteReviewCard.module.css
│   ├── NoteArchivePage.tsx
│   └── NoteArchivePage.module.css
├── types/api.ts                    — 新增笔记类型
├── components/layout/
│   ├── TopNavBar.tsx               — 新增笔记链接
│   └── BottomTabBar.tsx            — 不变
├── pages/home/
│   └── HomePage.tsx                — 新增笔记模块卡片
├── pages/word/                     — 新增创建笔记按钮（如需新建 WordDetailPage）
├── pages/grammar/                  — 新增创建笔记按钮（如需新建 GrammarDetailPage）
├── App.tsx                         — 新增路由
└── i18n/locales/{zh,en,ja}.ts      — 新增翻译键
```

---

## 13. i18n 翻译键

```
nav.notes                        — 笔记
note.list.title                  — 我的笔记
note.list.empty                  — 暂无笔记
note.list.search                 — 搜索笔记...
note.list.filterType             — 类型
note.list.filterTag              — 标签
note.list.sortCreated            — 创建时间
note.list.sortUpdated            — 更新时间
note.list.sortReview             — 复习时间
note.type.word                   — 单词
note.type.grammar                — 语法
note.type.sentence               — 句子
note.create.title                — 新建笔记
note.create.blank                — 空白笔记
note.edit.title                  — 编辑笔记
note.editor.title                — 标题
note.editor.type                 — 类型
note.editor.tags                 — 标签
note.editor.sourceText           — 来源文本
note.editor.content              — 内容
note.editor.save                 — 保存
note.editor.saving               — 保存中...
note.detail.notFound             — 笔记不存在
note.detail.delete               — 删除
note.detail.deleteConfirm        — 确认删除？
note.detail.edit                 — 编辑
note.detail.sourceText           — 来源文本
note.detail.reference            — 关联内容
note.detail.links                — 出向链接
note.detail.backlinks            — 回链
note.detail.noLinks              — 暂无关联
note.link.add                    — 添加关联
note.link.remove                 — 删除关联
note.link.searchTarget           — 搜索笔记...
note.link.targetNote             — 目标笔记
note.link.relation               — 关联类型
note.link.relation.related       — 一般关联
note.link.relation.usesWord      — 用到了单词
note.link.relation.usesGrammar   — 用到了语法
note.link.relation.context       — 上下文
note.srs.promote                 — 加入复习
note.srs.demote                  — 退出复习
note.srs.graduated               — 已毕业
note.srs.recycle                 — 回炉重练
note.srs.mastery                 — 掌握度
note.srs.nextReview              — 下次复习
note.srs.rating.easy             — 简单
note.srs.rating.normal           — 正常
note.srs.rating.hard             — 困难
note.archive.title               — 归档笔记
note.archive.empty               — 暂无归档
note.review.title                — 笔记
note.review.flip                 — 点击查看内容
home.modules.note                — 笔记
word.createNote                  — 创建笔记
grammar.createNote               — 创建笔记
note.createFromWord              — 已关联到单词"{title}"
note.createFromGrammar           — 已关联到语法"{title}"
```

---

## 14. 核心取舍

1. **TipTap 而非纯 Markdown 编辑器**。用户选择所见即所得，降低使用门槛。TipTap 支持 Markdown 输入/输出，后端存储 Markdown 格式。

2. **不新增底部 tab**。底部已有6个 tab 较满，笔记入口通过首页卡片 + 顶部导航栏提供。

3. **一键创建笔记**。从单词/语法详情页创建笔记时，自动 POST 创建关联笔记并跳转编辑页，减少用户操作步骤。

4. **搜索优先于过滤**。列表页顶部搜索栏为主交互，类型和标签为辅助过滤。

5. **NoteEditor 组件复用**。创建和编辑共用同一个 TipTap 编辑器组件，通过 `initialNote` prop 区分模式。

6. **统一复习队列复用现有翻牌 UI**。word 卡片保持现有翻牌逻辑，note 卡片新增 NoteReviewCard 组件，复用相同的评分按钮样式。

7. **Markdown 渲染**。详情页使用轻量 Markdown 渲染库（如 `marked` 或 `react-markdown`）将内容转为 HTML 显示，不引入 TipTap 只读模式。

---

## 15. 前置依赖

以下项目需要与笔记前端同步或先行完成：

1. **WordDetailPage 新建**。当前前端只有 `WordReviewPage`（翻牌复习），没有单词详情页。后端 `GET /api/v1/words/{id}` 已就绪（含 `related_notes`）。需新建 `WordDetailPage` 以承载"创建笔记"按钮。

2. **GrammarDetailPage 修改**。当前已有 `GrammarDetailPage`，需新增"创建笔记"按钮 + `related_notes` 展示区。

3. **首页笔记统计**。当前 `GET /api/v1/users/stats` 不包含笔记数据。HomePage 需单独调用 `GET /api/v1/notes/review-queue` 获取笔记 `due_count`，并调用 `GET /api/v1/notes?size=1` 获取 `total`。

4. **统一复习队列页面**。当前 `/words/review` 只处理 word 卡片。需决定是修改现有页面使其支持 `ReviewCard[]`，还是新建 `/review` 页面。建议修改现有 `WordReviewPage`，替换数据源为 `/api/v1/review/queue`，根据 `card_type` 渲染不同卡片。

5. **npm 依赖**。需安装：
   - `@tiptap/react` `@tiptap/starter-kit` `@tiptap/extension-placeholder` — 富文本编辑器
   - `marked` 或 `react-markdown` — Markdown 渲染（详情页）
