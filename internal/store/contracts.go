package store

import (
	"context"
	"time"

	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

type StoreRuntime struct {
	App   AppStores
	Admin AdminStores

	transact func(context.Context, func(*StoreRuntime) error) error
}

func NewStoreRuntime(app AppStores, admin AdminStores, transact func(context.Context, func(*StoreRuntime) error) error) *StoreRuntime {
	return &StoreRuntime{
		App:      app,
		Admin:    admin,
		transact: transact,
	}
}

func (r *StoreRuntime) Transact(ctx context.Context, fn func(*StoreRuntime) error) error {
	if r == nil || r.transact == nil {
		return ErrNestedTransaction
	}
	return r.transact(ctx, fn)
}

type AppStores struct {
	Users        UserStoreInterface
	Words        WordStoreInterface
	Grammar      GrammarStoreInterface
	Lessons      LessonStoreInterface
	Notes        NoteStoreInterface
	Speaking     SpeakingStoreInterface
	Writing      WritingStoreInterface
	Translation  TranslationStoreInterface
	Sessions     SessionStoreInterface
	AudioObjects AudioObjectStoreInterface
}

type AdminStores struct {
	Users       AdminUserStoreInterface
	Words       AdminWordStoreInterface
	Grammar     AdminGrammarStoreInterface
	Lessons     AdminLessonStoreInterface
	Speaking    AdminSpeakingStoreInterface
	Writing     AdminWritingStoreInterface
	Translation AdminTranslationStoreInterface
	Records     AdminRecordStoreInterface
	Imports     AdminImportStoreInterface
}

type UserStoreInterface interface {
	CreateUser(u user.User, passwordHash string) (*user.User, error)
	GetUserByEmail(email string) (*user.User, string, error)
	GetUserByID(id int64) (*user.User, error)
	GetStats(userID int64) (*user.UserStats, error)
	UpdateDailyGoals(userID int64, goals map[string]int) error
	GetDailyGoals(userID int64) (map[string]int, error)
	GetUserIDByEmail(email string) (int64, error)
	CreateResetToken(token string, userID int64, expiresAt time.Time) error
	GetResetToken(token string) (*user.ResetToken, error)
	MarkTokenUsed(token string) error
	UpdatePassword(userID int64, newPasswordHash string) error
	UpdateUser(id int64, name, email string, jlptLevels []string) error
}
type WordStoreInterface interface {
	GetByID(id int64) (*word.Word, error)
	ListByLevel(level word.JLPTLevel) ([]word.Word, error)
	GetRecord(userID, wordID int64) (*word.WordRecord, error)
	ListDueRecords(userID int64) ([]word.WordRecord, error)
	UpsertRecord(r word.WordRecord) error
	BookmarkWord(userID, wordID int64) error
}
type GrammarStoreInterface interface {
	GetByID(id int64) (*grammar.GrammarPoint, error)
	ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error)
	ListByLevelWithStatus(userID int64, level grammar.JLPTLevel) ([]grammar.GrammarPointWithStatus, error)
	GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error)
	UpsertRecord(r grammar.GrammarRecord) error
	ListDueRecords(userID int64) ([]grammar.GrammarRecord, error)
}
type LessonStoreInterface interface {
	ListSummaries(level lesson.JLPTLevel) ([]lesson.LessonSummary, error)
	GetDetail(id int64) (*lesson.Lesson, error)
	GetSentences(lessonID int64) ([]lesson.Sentence, error)
}
type NoteStoreInterface interface{}
type SpeakingStoreInterface interface{}
type WritingStoreInterface interface{}
type TranslationStoreInterface interface{}
type SessionStoreInterface interface{}
type AudioObjectStoreInterface interface{}

type AdminUserStoreInterface interface {
	ListAllUsers(offset, limit int) ([]user.User, int, error)
	GetStats(userID int64) (*user.UserStats, error)
	DeleteUser(id int64) error
}
type AdminWordStoreInterface interface {
	ListAll(level word.JLPTLevel, search string, offset, limit int) ([]word.Word, int, error)
	InsertWord(w word.Word) (int64, error)
	UpdateWord(w word.Word) error
	DeleteWord(id int64) error
	ListAllRecords(userID int64, offset, limit int) ([]word.WordRecord, int, error)
}
type AdminGrammarStoreInterface interface {
	ListAll(level, search string, offset, limit int) ([]grammar.GrammarPoint, int, error)
	InsertPoint(gp grammar.GrammarPoint) (int64, error)
	UpdatePoint(gp grammar.GrammarPoint) error
	DeletePoint(id int64) error
	ListAllRecords(userID int64, offset, limit int) ([]grammar.GrammarRecord, int, error)
}
type AdminLessonStoreInterface interface{}
type AdminSpeakingStoreInterface interface{}
type AdminWritingStoreInterface interface{}
type AdminTranslationStoreInterface interface{}
type AdminRecordStoreInterface interface{}
type AdminImportStoreInterface interface{}
