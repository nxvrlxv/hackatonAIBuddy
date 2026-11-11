insert into buddy.teams (code, name) values ('delivery', 'Команда Авито Доставки');
insert into buddy.teams (code, name) values ('travel', 'Команда Авито Путешествий');
insert into buddy.teams (code, name) values ('advert', 'Команда Авито Реклама');

insert into buddy.users (login, password_hash, full_name, role, team_id, mentor_id)
values ('anna_mentor', 'password', 'Анна Смирнова', 'mentor', (
    select id from buddy.teams t where t.code = 'travel'
), NULL);

insert into buddy.users (login, password_hash, full_name, role, team_id, mentor_id)
values ('vasya_mentor', 'password', 'Василий Пупкин', 'mentor', (
    select id from buddy.teams t where t.code = 'delivery'
), NULL);

INSERT INTO buddy.users (login, password_hash, full_name, role, team_id, mentor_id) VALUES
    ('sasha.intern', '!', 'Саша Иванов', 'intern',
        (SELECT id FROM buddy.teams WHERE code = 'travel'),
        (SELECT id FROM buddy.users WHERE login = 'anna_mentor')),
    ('maria.intern', '!', 'Мария Козлова', 'intern',
        (SELECT id FROM buddy.teams WHERE code = 'delivery'),
        (SELECT id FROM buddy.users WHERE login = 'vasya_mentor'));
