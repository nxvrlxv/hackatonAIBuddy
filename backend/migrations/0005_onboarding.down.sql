BEGIN;
-- Обратный порядок: задачи стажёров → шаблоны → этапы (FK без CASCADE).
DELETE FROM buddy.intern_tasks
WHERE template_id IN (
    SELECT tt.id FROM buddy.task_templates tt
    JOIN buddy.onboarding_stages s ON s.id = tt.stage_id
    WHERE s.code IN ('team', 'tools', 'first_tasks'));
DELETE FROM buddy.task_templates
WHERE stage_id IN (SELECT id FROM buddy.onboarding_stages WHERE code IN ('team', 'tools', 'first_tasks'));
DELETE FROM buddy.onboarding_stages WHERE code IN ('team', 'tools', 'first_tasks');
COMMIT;
