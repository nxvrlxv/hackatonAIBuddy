BEGIN;

DROP TABLE agent.conversation_chunks;
DROP TABLE agent.conversation_summaries;
DROP TABLE agent.messages;
DROP TABLE agent.conversations;
DROP SCHEMA agent;

COMMIT;
