# 笔记系统设计方案（最终版）

> 讨论日期：2026-05-08
> 原始草案：docs/note-system-design.md

---

## 1. 核心定位

笔记系统是**用户私有 + 可关联 + 可复习**的记录工具。兼顾两种使用场景：

- **复习辅助**：从单词/语法详情页一键创建笔记，自动关联系统内容
- **独立知识库**：自由记录课外遇到的单词、语法、句子，不绑定系统内容

笔记内容使用 **Markdown** 格式，前端负责渲染。

---

## 2. 数据库设计

### 2.1 notes 主表

```sql
CREATE TABLE notes (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id),
    type                TEXT NOT NULL CHECK(type IN ('word', 'grammar', 'sentence')),
    title               TEXT NOT NULL,
    content             TEXT NOT NULL DEFAULT '',       -- Markdown 格式
    source_text         TEXT NOT NULL DEFAULT '',       -- 原始句子/上下文
    reference_id        INTEGER,                       -- 可选：关联系统已有 word_id/grammar_point_id/lesson_id
    reference_type      TEXT,                          -- 'word' | 'grammar' | 'lesson'
    tags_json           TEXT NOT NULL DEFAULT '[]',
    -- SRS 复习字段
    mastery_level       INTEGER NOT NULL DEFAULT 0,    -- 0~5+，SM-2 重复次数
    next_review_at      DATETIME,                      -- NULL = 未加入复习 或 已毕业
    ease_factor         REAL NOT NULL DEFAULT 2.5,     -- SM-2 EF
    interval            INTEGER NOT NULL DEFAULT 0,    -- 距下次复习天数
    review_history_json TEXT NOT NULL DEFAULT '[]',
    -- 时间戳
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    deleted_at          DATETIME                       -- 软删除
);

CREATE INDEX idx_notes_user_id ON notes(user_id);
CREATE INDEX idx_notes_type ON notes(user_id, type);
CREATE INDEX idx_notes_review ON notes(user_id, next_review_at)
    WHERE next_review_at IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_notes_reference ON notes(reference_type, reference_id)
    WHERE deleted_at IS NULL;
```

**SRS 状态机：**

```
未加入复习 (next_review_at IS NULL, mastery=0)
    │  POST /promote
    ▼
复习中 (next_review_at = now(), mastery=0)
    │  POST /review (easy/normal/hard)
    ▼
复习中 (next_review_at = next, mastery++)
    │  连续正确 → mastery 增长，间隔拉长
    │  mastery >= 5 → 自动毕业
    ▼
已毕业 (next_review_at = NULL, mastery >= 5)
    │  POST /recycle
    ▼
复习中 (回炉，mastery 不变，重新安排复习)
```

评分 hard 时 mastery 重置为 0（同现有 SM-2 逻辑）。

### 2.2 note_links 关联表

```sql
CREATE TABLE note_links (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    note_id         INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    relation        TEXT NOT NULL DEFAULT 'related',
        -- 'uses_word'    : 语法/句子笔记用到了某个单词笔记
        -- 'uses_grammar' : 句子笔记用到了某个语法笔记
        -- 'context'      : 某个单词/语法出现在某个句子中
        -- 'related'      : 一般关联
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(note_id, target_note_id)
);

CREATE INDEX idx_note_links_note_id ON note_links(note_id);
CREATE INDEX idx_note_links_target ON note_links(target_note_id);
```

### 2.3 FTS5 全文搜索

```sql
CREATE VIRTUAL TABLE notes_fts USING fts5(
    title, content, source_text,
    content=notes, content_rowid=id
);

CREATE TRIGGER notes_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;

CREATE TRIGGER notes_ad AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
END;

CREATE TRIGGER notes_au AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;
```

**FTS5 搜索注意事项：** 搜索时需 JOIN `notes` 表过滤软删除和用户隔离：

```sql
SELECT n.* FROM notes n
JOIN notes_fts fts ON n.id = fts.rowid
WHERE notes_fts MATCH ? AND n.user_id = ? AND n.deleted_at IS NULL
ORDER BY rank;
```

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
    ID               int64       `json:"id"`
    UserID           int64       `json:"-"`
    Type             NoteType    `json:"type"`
    Title            string      `json:"title"`
    Content          string      `json:"content"`
    SourceText       string      `json:"source_text"`
    ReferenceID      *int64      `json:"reference_id,omitempty"`
    ReferenceType    *string     `json:"reference_type,omitempty"`
    Tags             []string    `json:"tags"`
    // SRS
    MasteryLevel     int         `json:"mastery_level"`
    NextReviewAt     *time.Time  `json:"next_review_at,omitempty"`
    EaseFactor       float64     `json:"ease_factor"`
    Interval         int         `json:"interval"`
    ReviewHistory    []ReviewEvent `json:"review_history"`
    CreatedAt        time.Time   `json:"created_at"`
    UpdatedAt        time.Time   `json:"updated_at"`
}

type ReviewEvent struct {
    Rating     string    `json:"rating"`      // "easy" | "normal" | "hard"
    ReviewedAt time.Time `json:"reviewed_at"`
}

type NoteLink struct {
    ID           int64        `json:"id"`
    NoteID       int64        `json:"note_id"`
    TargetNoteID int64        `json:"target_note_id"`
    Relation     LinkRelation `json:"relation"`
    TargetNote   *NoteDigest  `json:"target_note,omitempty"`
}

type NoteDetail struct {
    Note
    OutgoingLinks []NoteLink `json:"links"`          // 我关联了谁
    IncomingLinks []NoteLink `json:"backlinks"`      // 谁关联了我
}

type NoteDigest struct {
    ID    int64    `json:"id"`
    Title string   `json:"title"`
    Type  NoteType `json:"type"`
}

type NoteListParams struct {
    Type   NoteType
    Tag    string
    Sort   string   // "created_at" | "updated_at" | "next_review_at"
    Order  string   // "asc" | "desc"
    Offset int
    Limit  int
}

// ReviewCard 统一复习队列卡片
type ReviewCard struct {
    CardType string     `json:"card_type"`           // "word" | "note"
    WordCard *WordCard  `json:"word_card,omitempty"`
    NoteCard *NoteCard  `json:"note_card,omitempty"`
    IsNew    bool       `json:"is_new"`
}

type NoteCard struct {
    Note       Note   `json:"note"`
    NextReview *time.Time `json:"next_review_at,omitempty"`
    IsNew      bool   `json:"is_new"`
}
```

---

## 4. Store 接口

```go
// NoteStoreInterface 定义于 service.go

type NoteStoreInterface interface {
    // CRUD
    Create(note *Note) error
    GetByID(userID, noteID int64) (*Note, error)
    List(userID int64, params NoteListParams) ([]Note, int, error)
    Update(note *Note) error
    SoftDelete(userID, noteID int64) error

    // 搜索
    Search(userID int64, query string, limit int) ([]Note, error)

    // 关联
    AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error)
    RemoveLink(userID, linkID int64) error
    GetOutgoingLinks(userID, noteID int64) ([]NoteLink, error)
    GetIncomingLinks(userID, noteID int64) ([]NoteLink, error)

    // SRS
    Promote(userID, noteID int64) error
    Demote(userID, noteID int64) error   // 退出复习
    SaveReview(userID, noteID int64, note Note) error  // 保存 SM-2 结果

    // 复习队列
    ListDueNotes(userID int64) ([]Note, error)
    ListArchived(userID int64, params NoteListParams) ([]Note, int, error)  // 已毕业

    // 跨模块
    ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)

    // 标签
    ListTags(userID int64) ([]string, error)
}
```

---

## 5. API 端点

所有端点在 `/api/v1` 下，需认证。

### 5.1 笔记 CRUD

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/notes?type=word&tag=N5&sort=updated_at&order=desc&page=1&size=20` | 列表，支持类型/标签过滤、排序、分页 |
| `POST` | `/notes` | 创建笔记 |
| `GET` | `/notes/{id}` | 笔记详情（含出向+入向链接） |
| `PUT` | `/notes/{id}` | 更新笔记 |
| `DELETE` | `/notes/{id}` | 软删除 |

### 5.2 搜索

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/notes/search?q=keyword` | FTS5 全文搜索（标题+内容+上下文） |

### 5.3 关联管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/notes/{id}/links` | 创建关联 `{"target_note_id": 2, "relation": "uses_word"}` |
| `DELETE` | `/notes/{id}/links/{linkId}` | 删除关联 |

### 5.4 SRS 复习

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/notes/{id}/promote` | 加入复习队列（设置 next_review_at=now） |
| `DELETE` | `/notes/{id}/promote` | 退出复习队列（next_review_at=NULL，笔记保留） |
| `POST` | `/notes/{id}/review` | 评分 `{"rating": "easy\|normal\|hard"}` |
| `POST` | `/notes/{id}/recycle` | 已毕业卡片回炉重练（等同 promote，语义更明确） |
| `GET` | `/notes/archive?sort=updated_at&page=1&size=20` | 已毕业卡片列表 |

### 5.5 统一复习队列

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/review/queue?page=1&size=20` | 合并单词+笔记到期卡片，按 next_review_at 升序 |

返回 `ReviewCard` 数组，`card_type` 区分 `word` / `note`。新卡片（无记录）排在最后。

### 5.6 标签

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/notes/tags` | 返回当前用户用过的所有标签列表 |

### 5.7 跨模块注入（修改现有接口）

word/grammar 详情接口返回中追加 `related_notes` 字段（`NoteDigest[]`，最多 5 条）。

---

## 6. 统一复习队列设计

`/review/queue` 需要合并两类数据源：

- **单词到期卡片**：现有 `word_records` 中 `next_review_at <= now()` 的记录
- **笔记到期卡片**：`notes` 中 `next_review_at IS NOT NULL AND next_review_at <= now()` 的记录

实现方式：在 Handler 层并行调用 WordService 和 NoteService，合并卡片数组后按 `next_review_at` 排序返回。

```go
// 伪代码
cards := []ReviewCard{}
wordCards, _ := wordSvc.GetReviewQueue(userID, level)
noteCards, _ := noteSvc.GetReviewQueue(userID)
for _, wc := range wordCards {
    cards = append(cards, ReviewCard{CardType: "word", WordCard: &wc})
}
for _, nc := range noteCards {
    cards = append(cards, ReviewCard{CardType: "note", NoteCard: &nc})
}
sort.Slice(cards, ...)  // 按 next_review_at 升序
```

---

## 7. SM-2 算法复用

笔记复习直接复用 `internal/module/word/sm2.go` 中的 `CalcNextReview`。实现时需先将其抽取到独立包（`internal/sm2/`），使 word 和 note 模块共用的 reviewing 逻辑保持单一来源。

毕业条件：`mastery_level >= 5` 时自动设置 `next_review_at = NULL`。

注意：`recycle` 和 `promote` 底层是同一操作（设置 `next_review_at = now()`），分开两个端点是为了语义清晰。

---

## 8. 文件结构

```
internal/module/note/
├── model.go       -- Note, NoteLink, NoteDetail, NoteDigest, 枚举常量
├── service.go     -- NoteService + NoteStoreInterface
├── handler.go     -- HTTP handlers + RegisterRoutes
└── note_test.go   -- 表格驱动测试

internal/module/review/
├── handler.go     -- 统一复习队列 handler（或放在 word handler 中）

internal/data/
├── note_store.go  -- SQLite 实现（含 FTS5 搜索、SRS 操作）
├── adapters.go    -- 添加 NoteStoreAdapter

internal/data/migrations/
└── 006_notes.sql  -- DDL（notes + note_links + FTS5 + triggers）

internal/module/word/sm2.go  → internal/sm2/sm2.go  -- 若需抽取 SM-2 供复用
```

---

## 9. 核心取舍

1. **笔记是独立实体**，不修改现有 word/grammar/lesson 表结构。通过 `reference_id` 保留可选的系统关联。

2. **SRS 字段直加在 notes 表**。笔记是用户私有的，无需像 `words`/`word_records` 那样分两张表。`NULL` 表示"不在复习中"，避免额外的 promote 表。

3. **软删除**。查询时过滤 `deleted_at IS NOT NULL`。

4. **Markdown 内容**。后端不解析，前端渲染。

5. **FTS5 搜索**。比 LIKE 性能好且支持日文，代价是多一个虚拟表和三个触发器。

6. **统一复习队列为独立端点**。原有单词复习端点保留，后续可废弃。

7. **SM-2 算法需抽取复用**。笔记和单词共用同一套算法逻辑。
