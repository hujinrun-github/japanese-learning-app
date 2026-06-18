package shadowing

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

type fakeLessonReader struct {
	lesson *lesson.Lesson
	err    error
	gotID  int64
}

func (f *fakeLessonReader) GetDetail(id int64) (*lesson.Lesson, error) {
	f.gotID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.lesson, nil
}

type fakeStore struct {
	progress          *Progress
	summary           []SentenceAttemptSummary
	completed         []int
	progressVersion   int
	summaryVersion    int
	completedVersion  int
	insertedAttempts  []Attempt
	upsertedProgress  []Progress
	getProgressErr    error
	upsertProgressErr error
	insertAttemptErr  error
	listSummaryErr    error
	listCompletedErr  error
}

func (f *fakeStore) GetProgress(userID, lessonID int64, version int) (*Progress, error) {
	f.progressVersion = version
	if f.getProgressErr != nil {
		return nil, f.getProgressErr
	}
	return f.progress, nil
}

func (f *fakeStore) UpsertProgress(p Progress) error {
	if f.upsertProgressErr != nil {
		return f.upsertProgressErr
	}
	f.upsertedProgress = append(f.upsertedProgress, p)
	f.progress = &p
	return nil
}

func (f *fakeStore) InsertAttempt(a Attempt) error {
	if f.insertAttemptErr != nil {
		return f.insertAttemptErr
	}
	f.insertedAttempts = append(f.insertedAttempts, a)
	return nil
}

func (f *fakeStore) ListAttemptSummary(userID, lessonID int64, version int) ([]SentenceAttemptSummary, error) {
	f.summaryVersion = version
	if f.listSummaryErr != nil {
		return nil, f.listSummaryErr
	}
	return f.summary, nil
}

func (f *fakeStore) ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error) {
	f.completedVersion = version
	if f.listCompletedErr != nil {
		return nil, f.listCompletedErr
	}
	return f.completed, nil
}

func validLesson() *lesson.Lesson {
	return &lesson.Lesson{
		LessonSummary: lesson.LessonSummary{
			ID:               42,
			AudioURL:         "https://example.com/audio.mp3",
			ShadowingEnabled: true,
			ShadowingVersion: 7,
		},
		Sentences: []lesson.Sentence{
			{Index: 0, StartMS: 0, EndMS: 1000},
			{Index: 1, StartMS: 1500, EndMS: 2500},
		},
	}
}

func requireShadowingCode(t *testing.T, err error, wantCode string) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %s", wantCode)
	}
	var shadowingErr *Error
	if !errors.As(err, &shadowingErr) {
		t.Fatalf("error %T = %v, want *Error", err, err)
	}
	if shadowingErr.Code != wantCode {
		t.Fatalf("error code = %s, want %s", shadowingErr.Code, wantCode)
	}
	return shadowingErr
}

func TestServiceGetSessionValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		lesson *lesson.Lesson
		code   string
	}{
		{
			name: "disabled lesson",
			lesson: func() *lesson.Lesson {
				l := validLesson()
				l.ShadowingEnabled = false
				return l
			}(),
			code: ErrShadowingDisabled,
		},
		{
			name: "missing audio",
			lesson: func() *lesson.Lesson {
				l := validLesson()
				l.AudioURL = ""
				return l
			}(),
			code: ErrShadowingMediaMissing,
		},
		{
			name: "empty sentences",
			lesson: func() *lesson.Lesson {
				l := validLesson()
				l.Sentences = nil
				return l
			}(),
			code: ErrShadowingContentInvalid,
		},
		{
			name: "invalid sentence timing",
			lesson: func() *lesson.Lesson {
				l := validLesson()
				l.Sentences = []lesson.Sentence{{Index: 0, StartMS: 1000, EndMS: 1000}}
				return l
			}(),
			code: ErrShadowingContentInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeLessonReader{lesson: tt.lesson}, &fakeStore{})

			session, err := svc.GetSession(100, tt.lesson.ID)

			if session != nil {
				t.Fatalf("session = %+v, want nil", session)
			}
			requireShadowingCode(t, err, tt.code)
		})
	}
}

func TestServiceSaveProgressStaleVersionReturnsCurrentVersion(t *testing.T) {
	l := validLesson()
	svc := NewService(&fakeLessonReader{lesson: l}, &fakeStore{})

	progress, err := svc.SaveProgress(100, l.ID, ProgressRequest{
		Version:           l.ShadowingVersion - 1,
		LastSentenceIndex: 0,
		LastPositionMS:    500,
		PracticeMode:      PracticeModeNormal,
	})

	if progress != nil {
		t.Fatalf("progress = %+v, want nil", progress)
	}
	shadowingErr := requireShadowingCode(t, err, ErrShadowingVersionStale)
	if shadowingErr.CurrentShadowingVersion != l.ShadowingVersion {
		t.Fatalf("CurrentShadowingVersion = %d, want %d", shadowingErr.CurrentShadowingVersion, l.ShadowingVersion)
	}
}

func TestServiceSaveAttemptRejectsPassiveModes(t *testing.T) {
	tests := []struct {
		name string
		mode PracticeMode
	}{
		{name: "normal", mode: PracticeModeNormal},
		{name: "slow", mode: PracticeModeSlow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := validLesson()
			store := &fakeStore{}
			svc := NewService(&fakeLessonReader{lesson: l}, store)

			err := svc.SaveAttempt(100, l.ID, AttemptRequest{
				Version:       l.ShadowingVersion,
				SentenceIndex: 0,
				PracticeMode:  tt.mode,
				PlaybackRate:  1,
				LoopCount:     0,
			})

			requireShadowingCode(t, err, ErrShadowingAttemptModeInvalid)
			if len(store.insertedAttempts) != 0 {
				t.Fatalf("insertedAttempts len = %d, want 0", len(store.insertedAttempts))
			}
		})
	}
}

func TestServiceGetSessionUsesCurrentShadowingVersion(t *testing.T) {
	l := validLesson()
	score := 90
	store := &fakeStore{
		progress: &Progress{
			UserID:            100,
			LessonID:          l.ID,
			ShadowingVersion:  l.ShadowingVersion,
			LastSentenceIndex: 1,
			LastPositionMS:    1600,
			LastPracticeMode:  PracticeModeLoop,
		},
		summary: []SentenceAttemptSummary{
			{SentenceIndex: 1, AttemptCount: 2, BestScore: &score, LastScore: &score},
		},
		completed: []int{1},
	}
	svc := NewService(&fakeLessonReader{lesson: l}, store)

	session, err := svc.GetSession(100, l.ID)

	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if session == nil {
		t.Fatal("GetSession() returned nil session")
	}
	if store.progressVersion != l.ShadowingVersion {
		t.Fatalf("progressVersion = %d, want %d", store.progressVersion, l.ShadowingVersion)
	}
	if store.summaryVersion != l.ShadowingVersion {
		t.Fatalf("summaryVersion = %d, want %d", store.summaryVersion, l.ShadowingVersion)
	}
	if store.completedVersion != l.ShadowingVersion {
		t.Fatalf("completedVersion = %d, want %d", store.completedVersion, l.ShadowingVersion)
	}
	if len(session.CompletedSentenceIndexes) != 1 || session.CompletedSentenceIndexes[0] != 1 {
		t.Fatalf("CompletedSentenceIndexes = %+v, want [1]", session.CompletedSentenceIndexes)
	}
}
