BEGIN;
UPDATE buddy.request_types
SET fields = jsonb_set(fields, '{end_date}', '{"type":"date","required":true}')
WHERE code = 'time_off';
UPDATE buddy.users SET mentor_id = NULL WHERE login IN ('petya.intern', 'olga.intern');
-- Задачи чек-листа могли создаться для нового стажёра при первом запросе.
DELETE FROM buddy.intern_tasks WHERE user_id = (SELECT id FROM buddy.users WHERE login = 'dima.intern');
DELETE FROM buddy.users WHERE login = 'dima.intern';
DELETE FROM buddy.users WHERE login IN ('igor.lead', 'ksenia.lead', 'oleg.lead', 'elena_mentor', 'natalia.lead', 'roman_mentor');
COMMIT;
