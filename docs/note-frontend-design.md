# 笔记系统 - 前端设计方案

## 1. 概述

基于 `note-system-design.md` 的后端设计，前端需要实现笔记的列表、创建、详情/编辑、搜索以及笔记间关联管理功能。设计遵循项目现有模式：React + TypeScript + CSS Modules + react-router-dom。

---

## 2. TypeScript 类型定义（追加到 `types/api.ts`）

```ts
// ====== Note ======

export type NoteType = 'word' | 'grammar' | 'sentence'

export type LinkRelation = 'related' | 'uses_word' | 'uses_grammar' | 'context'

export interface Note {
  id: number
  type: NoteType
  title: string
  content: string
  source_text: string
  reference_id?: number
  reference_type?: string
  tags: string[]
  created_at: string
  updated_at: string
}

export interface NoteLink {
  id: number
  note_id: number
  target_note_id: number
  relation: LinkRelation
  target_note?: Note
}

export interface NoteDetail extends Note {
  links: NoteLink[]
}

export interface NoteListParams {
  type?: NoteType
  tag?: string
  page?: number
  size?: number
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  size: number
}
```

---

## 3. 路由设计

新增 3 个路由，放在 `ProtectedLayout` 内：

| 路径 | 页面组件 | 说明 |
|------|----------|------|
| `/notes` | `NoteListPage` | 笔记列表（支持类型/标签过滤） |
| `/notes/new` | `NoteEditPage` | 新建笔记 |
| `/notes/:id` | `NoteDetailPage` | 笔记详情（含关联列表、编辑、删除） |

路由注册在 `App.tsx` 中添加：

```tsx
<Route path="/notes" element={<NoteListPage />} />
<Route path="/notes/new" element={<NoteEditPage />} />
<Route path="/notes/:id" element={<NoteDetailPage />} />
```

---

## 4. 底部导航

在 `BottomTabBar.tsx` 的 `TAB_CONFIG` 中添加：

```tsx
{ to: '/notes', key: 'nav.notes', icon: '🗒️', end: false },
```

---

## 5. 页面设计

### 5.1 NoteListPage — 笔记列表页 (`/notes`)

**路径**: `pages/note/NoteListPage.tsx`
**样式**: `pages/note/NoteListPage.module.css`

**功能**:
- 顶部类型过滤 Tab（全部 / 单词 / 语法 / 句子），默认"全部"
- 可选标签过滤（点击已有标签筛选）
- 笔记卡片列表，每张卡片显示：
  - 类型图标 + 标题
  - source_text 预览（截断）
  - 标签 Badge 列表
  - 更新时间
- 空状态提示（`EmptyState`）
- 右下角 FAB 按钮 → 跳转 `/notes/new`
- 分页（滚动加载或页码，先做页码）

**状态**:
```
typeFilter: NoteType | null   → API ?type=
tagFilter: string | null      → API ?tag=
notes: Note[]
total: number
page: number
loading: boolean
```

**API 调用**:
- `apiFetch<PaginatedResponse<Note>>('GET', '/api/v1/notes?type=...&tag=...&page=...&size=20')`

---

### 5.2 NoteDetailPage — 笔记详情页 (`/notes/:id`)

**路径**: `pages/note/NoteDetailPage.tsx`
**样式**: `pages/note/NoteDetailPage.module.css`

**功能**:
- 显示笔记完整信息：
  - 类型 Badge + 标题
  - 用户笔记内容（content）
  - 来源句子（source_text）
  - 标签列表
  - 系统关联（reference_id → 跳转到对应的 word/grammar 详情）
- **关联笔记区域** (`links`):
  - 每个 link 显示 target_note 的标题 + 类型 + 关联类型（relation 中文标签）
  - 可点击跳转到对应笔记详情
  - 可删除关联
- **操作按钮**:
  - 编辑按钮 → 切换为编辑模式（内联编辑或跳转编辑页）
  - 删除按钮（弹窗确认后删除 → 返回列表）
- 底部 "添加关联" 按钮 → 弹出搜索/选择笔记的 Modal

**状态**:
```
note: NoteDetail | null
loading: boolean
editing: boolean
linkModalOpen: boolean
```

**API 调用**:
- `apiFetch<NoteDetail>('GET', '/api/v1/notes/{id}')`
- `apiFetch('PUT', '/api/v1/notes/{id}', body)` — 更新
- `apiFetch('DELETE', '/api/v1/notes/{id}')` — 删除
- `apiFetch<NoteLink>('POST', '/api/v1/notes/{id}/links', body)` — 添加关联
- `apiFetch('DELETE', '/api/v1/notes/{id}/links/{linkId}')` — 删除关联

**内联编辑模式**: 为简化设计，详情页直接支持内联编辑而非跳转单独编辑页：
- 点击"编辑"→ 标题/content/source_text/tags 变为可编辑控件
- 点击"保存"→ PUT 请求 → 刷新详情
- 点击"取消"→ 恢复原值

---

### 5.3 NoteEditPage — 新建笔记页 (`/notes/new`)

**路径**: `pages/note/NoteEditPage.tsx`
**样式**: `pages/note/NoteEditPage.module.css`（可与 NoteDetailPage 共享样式）

**功能**:
- 类型选择（word / grammar / sentence），创建后不可改
- 标题输入框
- 内容输入框（textarea）
- 来源句子输入框
- 标签输入（TagInput 组件：输入回车添加标签）
- 系统关联选择（可选，搜索系统中已有的 word/grammar）
- 保存按钮 → POST → 成功后跳转 `/notes/:newId`
- 取消按钮 → 返回 `/notes`

**状态**:
```
type: NoteType
title: string
content: string
sourceText: string
tags: string[]
referenceId: number | null
referenceType: string | null
submitting: boolean
```

**API 调用**:
- `apiFetch<Note>('POST', '/api/v1/notes', body)`

---

## 6. 组件树

```
pages/note/
├── NoteListPage.tsx          # 列表页
├── NoteListPage.module.css
├── NoteDetailPage.tsx        # 详情页（含内联编辑 + 关联管理）
├── NoteDetailPage.module.css
├── NoteEditPage.tsx          # 新建页
├── NoteEditPage.module.css
├── components/
│   ├── NoteCard.tsx           # 列表中的笔记卡片（可选，复用 Card）
│   ├── NoteCard.module.css
│   ├── TagInput.tsx           # 标签输入组件
│   ├── TagInput.module.css
│   ├── LinkSearchModal.tsx    # 搜索并选择要关联的笔记的弹窗
│   └── LinkSearchModal.module.css
```

> 如果 `NoteCard` 很简单，可以直接内联在 `NoteListPage` 中，不必单独抽取。

---

## 7. i18n 键值设计

追加到各语言文件：

```ts
// 中文 (zh.ts)
nav: {
  notes: '笔记',
},
notes: {
  title: '我的笔记',
  empty: '还没有笔记，点击右下角创建第一条吧',
  filterAll: '全部',
  filterWord: '单词',
  filterGrammar: '语法',
  filterSentence: '句子',
  sourceText: '来源句子',
  content: '笔记内容',
  tags: '标签',
  tagPlaceholder: '输入标签，回车添加',
  type: '类型',
  create: '新建笔记',
  edit: '编辑',
  save: '保存',
  cancel: '取消',
  delete: '删除',
  deleteConfirm: '确定要删除这条笔记吗？关联也会被删除。',
  links: '关联笔记',
  noLinks: '暂无关联',
  addLink: '添加关联',
  searchLink: '搜索要关联的笔记',
  relation: {
    related: '相关',
    uses_word: '使用单词',
    uses_grammar: '使用语法',
    context: '上下文',
  },
  reference: '系统关联',
  noReference: '无',
  createSuccess: '笔记创建成功',
  updateSuccess: '笔记更新成功',
  deleteSuccess: '笔记已删除',
},
```

---

## 8. 数据流

```
NoteListPage
  ├── apiFetch<PaginatedResponse<Note>>('GET', '/api/v1/notes?...')
  │     → 渲染列表
  └── 点击 FAB → navigate('/notes/new')

NoteEditPage
  ├── 表单提交 → apiFetch<Note>('POST', '/api/v1/notes', body)
  └── 成功后 → navigate(`/notes/${id}`)

NoteDetailPage
  ├── useEffect → apiFetch<NoteDetail>('GET', '/api/v1/notes/:id')
  ├── 编辑保存 → apiFetch('PUT', '/api/v1/notes/:id', body) → 刷新
  ├── 删除 → apiFetch('DELETE', '/api/v1/notes/:id') → navigate('/notes')
  ├── 添加关联 → 打开 LinkSearchModal
  │     ├── 搜索 → apiFetch<PaginatedResponse<Note>>('GET', '/api/v1/notes/search?q=...')
  │     └── 确认 → apiFetch('POST', '/api/v1/notes/:id/links', { target_note_id, relation })
  └── 删除关联 → apiFetch('DELETE', '/api/v1/notes/:id/links/:linkId')
```

---

## 9. 设计决策

1. **内联编辑 vs 独立编辑页**：详情页使用内联编辑模式（点击编辑按钮切换表单），避免页面跳转。新建页单独路由 `/notes/new`。
2. **分页方式**：先用传统页码分页（page/size），不做无限滚动，保持简单。
3. **搜索关联笔记**：使用 Modal 弹窗而非新页面，体验更轻量。
4. **删除确认**：用 `window.confirm` 即可，不引入额外依赖。
5. **类型创建后不可变**：`type` 字段创建后不可修改，设计上在新建页选择，编辑时不显示类型选择器。

---

## 10. CSS 方案

使用 CSS Modules。主色调复用项目 CSS 变量（`var(--color-primary)` 等）。笔记类型用不同颜色区分：
- 单词 (word): 蓝色系
- 语法 (grammar): 橙色系
- 句子 (sentence): 绿色系
