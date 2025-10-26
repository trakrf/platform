SET search_path=trakrf,public;

-- Drop trigger, then function, then table
DROP TRIGGER IF EXISTS messages_insert_trigger ON messages;
DROP FUNCTION IF EXISTS process_messages() CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
