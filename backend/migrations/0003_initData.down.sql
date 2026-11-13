BEGIN;
-- сначала стажёры: они ссылаются на наставников через mentor_id
DELETE FROM buddy.users WHERE login IN ('sasha.intern', 'maria.intern', 'petya.intern', 'olga.intern');
DELETE FROM buddy.users WHERE login IN ('anna_mentor', 'vasya_mentor');
DELETE FROM buddy.teams WHERE code IN ('travel', 'delivery', 'adverts', 'payments');
COMMIT;