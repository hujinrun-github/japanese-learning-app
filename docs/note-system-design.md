# 笔记系统设计方案

## 1. 核心思路

笔记系统是一个**用户私有**的记录工具，用户可以记录不熟悉的单词、语法、句子，并且笔记之间可以相互关联。设计上遵循项目现有的模式：Handler → Service → Adapter → Store → SQLite，纯标准库，依赖注入。

---

## 2. 数据库设计

新增一张 `notes` 表和一张 `note_links` 关联表：

```sql
-- 笔记主表
CREATE TABLE notes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    type            TEXT NOT NULL CHECK(type IN ('word', 'grammar', 'sentence')),
    title           TEXT NOT NULL,              -- 笔记标题（如单词本身、语法名）
    content         TEXT NOT NULL DEFAULT '',    -- 用户自己的笔记内容
    source_text     TEXT NOT NULL DEFAULT '',    -- 遇到时的原始句子/上下文
    reference_id    INTEGER,                    -- 可选：关联系统中已有的 word_id / grammar_point_id
    reference_type  TEXT,                       -- 'word' | 'grammar' | 'lesson'
    tags_json       TEXT NOT NULL DEFAULT '[]',  -- 用户自定义标签
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_notes_user_id ON notes(user_id);
CREATE INDEX idx_notes_type ON notes(user_id, type);

-- 笔记关联表（多对多自引用）
CREATE TABLE note_links (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    note_id         INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    relation        TEXT NOT NULL DEFAULT 'related',
        -- 'uses_word'    : 语法笔记用到了某个单词笔记
        -- 'uses_grammar' : 句子笔记用到了某个语法笔记
        -- 'context'      : 某个单词出现在某个句子中
        -- 'related'      : 一般关联
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(note_id, target_note_id)
);

CREATE INDEX idx_note_links_note_id ON note_links(note_id);
```

**设计要点：**
- `reference_id` + `reference_type` 允许笔记**可选地**关联系统中已有的单词/语法点/课文，但不强制
- `note_links` 实现了笔记之间的多对多关联，`relation` 字段区分关联类型
- 每条笔记和关联都带 `user_id`，天然实现用户隔离

---

## 3. Go Model

```go
// internal/module/note/model.go

type NoteType string

const (
    TypeWord     NoteType = "word"
    TypeGrammar  NoteType = "grammar"
    TypeSentence NoteType = "sentence"
)

type LinkRelation string

const (
    RelationRelated     LinkRelation = "related"
    RelationUsesWord    LinkRelation = "uses_word"
    RelationUsesGrammar LinkRelation = "uses_grammar"
    RelationContext     LinkRelation = "context"
)

type Note struct {
    ID            int64     `json:"id"`
    UserID        int64     `json:"-"`
    Type          NoteType  `json:"type"`
    Title         string    `json:"title"`
    Content       string    `json:"content"`
    SourceText    string    `json:"source_text"`
    ReferenceID   *int64    `json:"reference_id,omitempty"`
    ReferenceType *string   `json:"reference_type,omitempty"`
    Tags          []string  `json:"tags"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type NoteLink struct {
    ID           int64        `json:"id"`
    NoteID       int64        `json:"note_id"`
    TargetNoteID int64        `json:"target_note_id"`
    Relation     LinkRelation `json:"relation"`
    TargetNote   *Note        `json:"target_note,omitempty"` // 查询时填充
}

// NoteDetail 是查询单条笔记时的完整视图，包含所有关联笔记
type NoteDetail struct {
    Note
    Links []NoteLink `json:"links"`
}

// NoteListParams 是列表查询的过滤参数
type NoteListParams struct {
    Type    NoteType // 空表示不过滤
    Tag     string   // 空表示不过滤
    Offset  int
    Limit   int
}
```

---

## 4. Store 接口

```go
// NoteStoreInterface 定义数据访问层
type NoteStoreInterface interface {
    Create(note *Note) error
    GetByID(userID, noteID int64) (*Note, error)
    List(userID int64, params NoteListParams) ([]Note, int, error)
    Update(note *Note) error
    Delete(userID, noteID int64) error
    Search(userID int64, query string, limit int) ([]Note, error)

    // 关联管理
    AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error)
    RemoveLink(userID, linkID int64) error
    GetLinks(userID, noteID int64) ([]NoteLink, error)
}
```

---

## 5. API 端点设计

所有端点都在 `/api/v1/notes` 下，需要认证：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/notes?type=word&tag=n5&page=1&size=20` | 列表查询，支持按类型/标签过滤，分页 |
| `POST` | `/api/v1/notes` | 创建笔记 |
| `GET` | `/api/v1/notes/{id}` | 笔记详情（含关联的笔记列表） |
| `PUT` | `/api/v1/notes/{id}` | 更新笔记 |
| `DELETE` | `/api/v1/notes/{id}` | 删除笔记（级联删除关联） |
| `GET` | `/api/v1/notes/search?q=xxx` | 全文搜索（标题+内容） |
| `POST` | `/api/v1/notes/{id}/links` | 创建关联 `{"target_note_id": 2, "relation": "uses_word"}` |
| `DELETE` | `/api/v1/notes/{id}/links/{linkId}` | 删除关联 |

---

## 6. 关联场景示例

假设用户遇到一个句子「雨が降っている」，其中有不认识的单词「雨」和语法「～ている」：

```
1. 先创建单词笔记: POST /notes {type:"word", title:"雨", content:"あめ、雨", source_text:"雨が降っている"}
   → id=1

2. 创建语法笔记: POST /notes {type:"grammar", title:"～ている", content:"表示动作持续", source_text:"雨が降っている"}
   → id=2

3. 创建句子笔记: POST /notes {type:"sentence", title:"雨が降っている", content:"正在下雨"}
   → id=3

4. 建立关联:
   POST /notes/2/links {target_note_id: 1, relation: "uses_word"}    -- 语法用了单词
   POST /notes/3/links {target_note_id: 1, relation: "context"}      -- 句子中有单词
   POST /notes/3/links {target_note_id: 2, relation: "uses_grammar"} -- 句子用了语法
```

查询句子笔记(id=3)详情时，返回：
```json
{
  "id": 3,
  "type": "sentence",
  "title": "雨が降っている",
  "content": "正在下雨",
  "links": [
    {"id": 1, "note_id": 3, "target_note_id": 1, "relation": "context",
     "target_note": {"id": 1, "title": "雨", "type": "word"}},
    {"id": 2, "note_id": 3, "target_note_id": 2, "relation": "uses_grammar",
     "target_note": {"id": 2, "title": "～ている", "type": "grammar"}}
  ]
}
```

---

## 7. 文件结构

完全遵循项目现有模式：

```
internal/module/note/
├── model.go       -- Note, NoteLink, NoteDetail, 枚举常量
├── service.go     -- NoteService + NoteStoreInterface
├── handler.go     -- HTTP handlers + RegisterRoutes
└── note_test.go   -- 表格驱动测试

internal/data/
├── note_store.go  -- SQLite 实现
├── adapters.go    -- 添加 NoteStoreAdapter（如果需要的话）

internal/data/migrations/
└── 006_notes.sql  -- DDL
```

---

## 8. 与现有系统的关系

- `reference_id` + `reference_type` 可以让笔记关联到 `words.id` / `grammar_points.id` / `lessons.id`，在详情接口中可以选择性地把这部分信息也查出来展示
- 笔记系统是独立模块，不修改现有的 word/grammar/lesson 表结构
- 用户隔离沿用 `user_id` 模式，与 `word_records`、`grammar_records` 等表一致

---

## 9. 核心取舍

笔记是**独立实体**而非对现有 word/grammar 表的扩展。这样用户记录的内容不受预置数据库的限制，同时通过 `reference_id` 保留与系统数据的关联能力。
