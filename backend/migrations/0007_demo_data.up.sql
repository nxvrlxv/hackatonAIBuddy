BEGIN;
-- Демо-состав команд: руководитель и наставник в каждой команде, у всех стажёров есть наставник.
INSERT INTO buddy.users (login, password_hash, full_name, role, team_id, mentor_id)
SELECT v.login, '!', v.full_name, v.role, t.id, NULL
FROM (VALUES
    ('igor.lead',     'Игорь Лебедев',    'lead',   'travel'),
    ('ksenia.lead',   'Ксения Белова',    'lead',   'delivery'),
    ('oleg.lead',     'Олег Кузнецов',    'lead',   'adverts'),
    ('elena_mentor',  'Елена Васильева',  'mentor', 'adverts'),
    ('natalia.lead',  'Наталья Фёдорова', 'lead',   'payments'),
    ('roman_mentor',  'Роман Егоров',     'mentor', 'payments')
) AS v(login, full_name, role, team_code)
JOIN buddy.teams t ON t.code = v.team_code;

INSERT INTO buddy.users (login, password_hash, full_name, role, team_id, mentor_id) VALUES
    ('dima.intern', '!', 'Дмитрий Орлов', 'intern',
        (SELECT id FROM buddy.teams WHERE code = 'travel'),
        (SELECT id FROM buddy.users WHERE login = 'anna_mentor'));

UPDATE buddy.users SET mentor_id = (SELECT id FROM buddy.users WHERE login = 'elena_mentor') WHERE login = 'petya.intern';
UPDATE buddy.users SET mentor_id = (SELECT id FROM buddy.users WHERE login = 'roman_mentor') WHERE login = 'olga.intern';

-- Отгул на один день: дата окончания необязательна, по умолчанию равна дате начала.
UPDATE buddy.request_types
SET fields = jsonb_set(fields, '{end_date}', '{"type":"date","required":false,"default":"start_date"}')
WHERE code = 'time_off';
COMMIT;
