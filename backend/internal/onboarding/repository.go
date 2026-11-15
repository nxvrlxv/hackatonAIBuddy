package onboarding

import "context"

// Store — то, что сервису нужно от базы. В тестах сервиса подставляется фейк.
type Store interface {
	EnsureTasks(ctx context.Context, userID int64) error
	ListTasks(ctx context.Context, userID int64) ([]Row, error)
	UpdateStatus(ctx context.Context, userID, taskID int64, status Status) (Task, error)
	ListReviews(ctx context.Context, mentorID int64) ([]Review, error)
	Approve(ctx context.Context, mentorID, taskID int64) error
	Reject(ctx context.Context, mentorID, taskID int64, comment string) error
	ListInterns(ctx context.Context, mentorID int64) ([]InternProgress, error)
}
