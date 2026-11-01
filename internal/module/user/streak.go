package user

import "log/slog"

// StreakRecorder records that a user studied on the current day and returns
// the resulting consecutive-day streak.
//
// Practice modules depend on this narrow interface rather than *UserService so
// their tests can stub the check-in and so the streak bookkeeping stays a
// separate concern from module logic. *UserService satisfies it.
type StreakRecorder interface {
	RecordStudyDay(userID int64) (int, error)
}

// RecordStudyDayBestEffort records a study day, logging but otherwise
// swallowing failures.
//
// A streak is bookkeeping: the learner has already finished the exercise by the
// time this runs, so a failure here must never turn a successful submission
// into an error response. A nil recorder is a no-op, which keeps modules
// usable when streak tracking is not wired up.
func RecordStudyDayBestEffort(recorder StreakRecorder, userID int64) {
	if recorder == nil {
		return
	}
	if _, err := recorder.RecordStudyDay(userID); err != nil {
		slog.Warn("streak check-in failed", "err", err, "user_id", userID)
	}
}
