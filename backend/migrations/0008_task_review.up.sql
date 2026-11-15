-- Апрув задач наставником: стажёр отправляет задачу на проверку (review),
-- наставник принимает (done) или возвращает с комментарием (in_progress).
-- Контракт: docs/approvals-contract.md
BEGIN;

-- Ограничение статуса в 0001 объявлено без имени, Postgres назвал его intern_tasks_status_check.
ALTER TABLE buddy.intern_tasks DROP CONSTRAINT intern_tasks_status_check;
ALTER TABLE buddy.intern_tasks ADD CONSTRAINT intern_tasks_status_check
    CHECK (status IN ('todo', 'in_progress', 'review', 'done'));

ALTER TABLE buddy.intern_tasks
    ADD COLUMN submitted_at   TIMESTAMPTZ,                          -- когда отправлена на проверку
    ADD COLUMN reviewed_by    BIGINT REFERENCES buddy.users (id),   -- кто принял/отклонил последним
    ADD COLUMN reviewed_at    TIMESTAMPTZ,
    ADD COLUMN review_comment TEXT;                                 -- комментарий последнего отклонения

-- На проверке всегда известна дата отправки (по ней сортируется очередь наставника).
ALTER TABLE buddy.intern_tasks ADD CONSTRAINT intern_tasks_review_submitted_check
    CHECK (status <> 'review' OR submitted_at IS NOT NULL);

-- Очередь наставника: только задачи на проверке, старые сверху.
CREATE INDEX idx_intern_tasks_review ON buddy.intern_tasks (submitted_at) WHERE status = 'review';

-- Демо: у Саши «Доступ к Jira» уже ждёт проверки — у anna_mentor сразу есть запрос на главной.
UPDATE buddy.intern_tasks it
SET status = 'review', submitted_at = now() - interval '20 minutes', updated_at = now()
FROM buddy.users u, buddy.task_templates tt
WHERE u.id = it.user_id AND tt.id = it.template_id
  AND u.login = 'sasha.intern' AND tt.title = 'Доступ к Jira' AND it.status <> 'done';

COMMIT;
