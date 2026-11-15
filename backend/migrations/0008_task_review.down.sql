BEGIN;

-- Статуса review после отката нет: незавершённые проверки возвращаем в работу.
UPDATE buddy.intern_tasks SET status = 'in_progress' WHERE status = 'review';

DROP INDEX IF EXISTS buddy.idx_intern_tasks_review;
ALTER TABLE buddy.intern_tasks DROP CONSTRAINT IF EXISTS intern_tasks_review_submitted_check;
ALTER TABLE buddy.intern_tasks
    DROP COLUMN IF EXISTS review_comment,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS submitted_at;

ALTER TABLE buddy.intern_tasks DROP CONSTRAINT intern_tasks_status_check;
ALTER TABLE buddy.intern_tasks ADD CONSTRAINT intern_tasks_status_check
    CHECK (status IN ('todo', 'in_progress', 'done'));

COMMIT;
