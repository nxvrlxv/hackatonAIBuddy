package onboarding

import "context"

// Store — то, что сервису нужно от базы. В тестах сервиса подставляется фейк.
type Store interface {
	EnsureTasks(ctx context.Context, userID int64) error
	ListTasks(ctx context.Context, userID int64) ([]Row, error)
	UpdateStatus(ctx context.Context, userID, taskID int64, status Status) (Task, error)
}
