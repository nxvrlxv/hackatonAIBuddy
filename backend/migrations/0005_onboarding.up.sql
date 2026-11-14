BEGIN;

INSERT INTO buddy.onboarding_stages (code, title, position) VALUES
    ('team',        'Знакомство с командой', 1),
    ('tools',       'Доступы и инструменты', 2),
    ('first_tasks', 'Первые задачи',         3);

-- team_code = NULL: задача для всех команд. Командные задачи идут после общих (position 10+).
INSERT INTO buddy.task_templates (stage_id, team_id, title, description, position)
SELECT s.id, t.id, v.title, v.description, v.position
FROM (VALUES
    ('team', NULL, 'Встреча с наставником',
        'Договорись о первой встрече: цели стажировки, формат созвонов, к кому идти с вопросами.', 1),
    ('team', NULL, 'Знакомство с командой',
        'Приди на стендап и коротко расскажи о себе.', 2),
    ('team', NULL, 'Миссия и ценности Авито',
        'Прочитай документ о миссии и ценностях компании.', 3),

    ('tools', NULL, 'Доступ к GitLab',
        'Оформи заявку на доступ к GitLab и репозиториям команды.', 1),
    ('tools', NULL, 'Доступ к Jira',
        'Получи доступ к доске команды в Jira.', 2),
    ('tools', NULL, 'Настроить VPN',
        'Установи и проверь корпоративный VPN.', 3),
    ('tools', NULL, 'Почта и календарь',
        'Настрой рабочую почту и добавь встречи команды в календарь.', 4),

    ('first_tasks', NULL, 'Собрать проект локально',
        'Склонируй репозиторий команды и запусти проект по README.', 1),
    ('first_tasks', NULL, 'Взять первую задачу',
        'Вместе с наставником выбери стартовую задачу в Jira.', 2),
    ('first_tasks', NULL, 'Пройти первое ревью',
        'Открой merge request и пройди код-ревью.', 3),

    ('team',        'travel',   'Обзор продукта Путешествий',
        'Пройди путь пользователя: поиск и бронирование жилья.', 10),
    ('team',        'delivery', 'Обзор продукта Доставки',
        'Оформи тестовую доставку и посмотри статусы заказа.', 10),
    ('team',        'adverts',  'Документ для студентов',
        'Прочитай материалы команды Объявлений для студентов и молодых специалистов.', 10),
    ('team',        'payments', 'Структура команды Платежей',
        'Изучи документ о структуре команды и зонах ответственности.', 10),
    ('first_tasks', 'payments', 'Безопасность платёжных данных',
        'Пройди вводный инструктаж по работе с платёжными данными.', 10)
) AS v(stage_code, team_code, title, description, position)
JOIN buddy.onboarding_stages s ON s.code = v.stage_code
LEFT JOIN buddy.teams t ON t.code = v.team_code;

-- Демо-прогресс: у Саши пройден первый этап, GitLab в работе.
-- Остальным стажёрам задачи создаёт сервис при первом запросе.
INSERT INTO buddy.intern_tasks (user_id, template_id, status, completed_at)
SELECT u.id, tt.id,
       CASE WHEN s.code = 'team' THEN 'done'
            WHEN tt.title = 'Доступ к GitLab' THEN 'in_progress'
            ELSE 'todo' END,
       CASE WHEN s.code = 'team' THEN now() END
FROM buddy.users u
JOIN buddy.task_templates tt ON tt.team_id IS NULL OR tt.team_id = u.team_id
JOIN buddy.onboarding_stages s ON s.id = tt.stage_id
WHERE u.login = 'sasha.intern';

COMMIT;
