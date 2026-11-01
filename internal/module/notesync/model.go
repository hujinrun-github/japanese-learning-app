package notesync

import (
	"errors"
	"time"

	"japanese-learning-app/internal/integration/notion"
	"japanese-learning-app/internal/module/note"
)

const TargetTypeNotion = "notion"

var (
	ErrTargetNotFound  = errors.New("note sync target not found")
	ErrTargetInUse     = errors.New("note sync target is in use")
	ErrIdentityLocked  = errors.New("note sync target identity is locked")
	ErrBindingNotFound = errors.New("note sync binding not found")
	ErrStateNotFound   = errors.New("note sync state not found")
)

type SyncTarget struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"-"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	DataSourceID   string    `json:"data_source_id"`
	TokenEnv       string    `json:"token_env"`
	TitleProperty  string    `json:"title_property"`
	TagsProperty   string    `json:"tags_property"`
	FlowIDProperty string    `json:"flow_id_property"`
	SyncTags       []string  `json:"sync_tags"`
	Enabled        bool      `json:"enabled"`
	AutoPush       bool      `json:"auto_push"`
	HasToken       bool      `json:"has_token"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SyncBinding struct {
	NoteID    int64     `json:"note_id"`
	TargetID  int64     `json:"target_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SyncState struct {
	NoteID         int64      `json:"note_id"`
	TargetID       int64      `json:"target_id"`
	ExternalPageID string     `json:"external_page_id"`
	ExternalURL    string     `json:"external_url"`
	LocalHash      string     `json:"local_hash"`
	ExternalHash   string     `json:"external_hash"`
	LastDirection  string     `json:"last_direction,omitempty"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	PendingDelete  bool       `json:"pending_delete"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ExternalPage struct {
	ID             string
	URL            string
	LastEditedTime string
	Archived       bool
	InTrash        bool
	Title          string
	Tags           []string
	FlowID         string
	Content        string
	SourceText     string
}

type SyncResult struct {
	Created       int             `json:"created"`
	Updated       int             `json:"updated"`
	Imported      int             `json:"imported"`
	Skipped       int             `json:"skipped"`
	PendingDelete int             `json:"pending_delete"`
	Errors        []SyncItemError `json:"errors,omitempty"`
}

type SyncItemError struct {
	NoteID  int64  `json:"note_id,omitempty"`
	PageID  string `json:"page_id,omitempty"`
	Message string `json:"message"`
}

type Store interface {
	CreateTarget(target *SyncTarget) error
	GetTarget(userID, targetID int64) (*SyncTarget, error)
	ListTargets(userID int64) ([]SyncTarget, error)
	UpdateTarget(target *SyncTarget) error
	DeleteTarget(userID, targetID int64) error
	CountBindings(userID, targetID int64) (int, error)
	GetBinding(userID, noteID int64) (*SyncBinding, error)
	SetBinding(userID, noteID, targetID int64) error
	ClearBinding(userID, noteID int64) error
	GetState(userID, noteID, targetID int64) (*SyncState, error)
	GetStateByPage(userID, targetID int64, pageID string) (*SyncState, error)
	SaveState(state *SyncState) error
	ListBoundNoteIDs(userID, targetID int64) ([]int64, error)
	ListStates(userID, targetID int64) ([]SyncState, error)
}

type NoteStore interface {
	Create(note *note.Note) error
	GetByID(userID, noteID int64) (*note.Note, error)
	Update(note *note.Note) error
	SoftDelete(userID, noteID int64) error
}

// NoteRestorer is implemented by stores that support restoring a soft-deleted
// note. Keeping it optional preserves compatibility with existing note store
// adapters while enabling the deletion-recovery flow where available.
type NoteRestorer interface {
	Restore(userID, noteID int64) error
}

type NotionClient interface {
	TestDataSource(dataSourceID string) error
	QueryDataSource(dataSourceID string) ([]notion.Page, error)
	RetrievePageBlocks(pageID string) ([]notion.Block, error)
	CreatePage(input notion.PageInput) (notion.Page, error)
	UpdatePage(pageID string, input notion.PageInput) (notion.Page, error)
	ReplacePageBlocks(pageID string, blocks []notion.Block) error
	RestorePage(pageID string) error
}

type ClientFactory func(target SyncTarget) (NotionClient, error)

type BindingRequest struct {
	TargetID *int64 `json:"target_id"`
}

type TargetRequest struct {
	Name           string   `json:"name"`
	DataSourceID   string   `json:"data_source_id"`
	TokenEnv       string   `json:"token_env"`
	TitleProperty  string   `json:"title_property"`
	TagsProperty   string   `json:"tags_property"`
	FlowIDProperty string   `json:"flow_id_property"`
	SyncTags       []string `json:"sync_tags"`
	Enabled        *bool    `json:"enabled"`
	AutoPush       *bool    `json:"auto_push"`
}

type TestTargetRequest struct {
	TargetRequest
}

type Service struct {
	store  Store
	notes  NoteStore
	client ClientFactory
	now    func() time.Time
}

// NewService creates the sync service. ClientFactory is injected to keep all
// Notion HTTP calls replaceable in tests.
func NewService(store Store, notes NoteStore, client ClientFactory) *Service {
	return &Service{store: store, notes: notes, client: client, now: time.Now}
}
