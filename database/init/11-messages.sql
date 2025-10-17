SET search_path = trakrf,public;

-- Create the messages table optimized for TimescaleDB
CREATE TABLE messages
(
    message_timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    message_topic     TEXT        NOT NULL,
    message_data      JSONB       NOT NULL,
    -- Primary key must include timestamp for hypertable
    PRIMARY KEY (message_timestamp, message_topic)
);

-- Indexes optimized for time-series queries
CREATE INDEX idx_messages_account_time ON messages (message_topic, message_timestamp DESC);

-- Create index on common JSON fields if they exist in message_data
CREATE INDEX idx_messages_message_type ON messages ((message_data ->> 'type'));

-- Row Level Security
ALTER TABLE messages
    ENABLE ROW LEVEL SECURITY;

-- todo: add account_id back in populated by trigger that parses topic
-- -- Create policies for each table
-- CREATE POLICY account_isolation_messages ON messages
--    USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE messages IS 'TimescaleDB hypertable for storing time-series message data';
COMMENT ON COLUMN messages.message_timestamp IS 'Timestamp when message was received by the system';
COMMENT ON COLUMN messages.message_topic IS 'MQTT topic the message was received on';
COMMENT ON COLUMN messages.message_data IS 'JSON message payload';

-- First create the hypertable
SELECT create_hypertable('messages', 'message_timestamp');
SELECT set_chunk_time_interval('messages', INTERVAL '1 day');

-- Add compression policy (optional, depends on your retention needs)
-- SELECT add_compression_policy('messages', INTERVAL '7 days');

-- Add retention policy (optional, depends on your needs)
SELECT add_retention_policy('messages', INTERVAL '10 days');


CREATE OR REPLACE FUNCTION process_messages() returns trigger
    language plpgsql
as
$$
DECLARE
    topic_account_id INT;
    device_name      TEXT;
BEGIN

    SELECT a.id INTO topic_account_id FROM accounts a WHERE a.domain = split_part(NEW.message_topic, '/', 1);

    IF topic_account_id IS NULL THEN
        RAISE NOTICE 'Could not find account for topic: %. Be sure that topic matches account slug', NEW.message_topic;
    END IF;

    INSERT INTO locations (account_id, identifier, name)
    SELECT DISTINCT topic_account_id,
                    t.tag ->> 'capturePointName',
                    t.tag ->> 'capturePointName' || ' auto added from message' as name
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT *
                      FROM antennas a
                      WHERE a.account_id = topic_account_id
                        AND a.identifier = t.tag ->> 'capturePointName');

    IF NOT EXISTS (SELECT *
                   FROM devices d
                   WHERE d.account_id = topic_account_id
                     AND d.identifier = NEW.message_data ->> 'rfidReaderName') THEN
        INSERT INTO devices (account_id, identifier, name, type)
        VALUES (topic_account_id,
                NEW.message_data ->> 'rfidReaderName',
                NEW.message_data ->> 'rfidReaderName' || ' auto added from message',
                'unknown');
    END IF;

    INSERT INTO antennas (account_id, device_id, location_id, identifier, name)
    SELECT DISTINCT topic_account_id,
                    (SELECT id
                     FROM devices
                     WHERE account_id = topic_account_id
                       AND identifier = NEW.message_data ->> 'rfidReaderName') AS device_id,
                    l.id,
                    t.tag ->> 'capturePointName',
                    t.tag ->> 'capturePointName' || ' auto added from message' as name
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN locations l ON l.account_id = topic_account_id AND l.identifier = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (SELECT *
                      FROM antennas a
                      WHERE a.account_id = topic_account_id
                        AND a.identifier = t.tag ->> 'capturePointName');

    INSERT INTO assets (account_id, identifier, name, type)
    SELECT DISTINCT topic_account_id,
                    t.tag ->> 'epc',
                    t.tag ->> 'epc' || ' auto added from message' as name,
                    'unknown'                                     as "type"
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT *
                      FROM assets
                      WHERE account_id = topic_account_id
                        AND identifier = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT *
                      FROM tags
                      WHERE account_id = topic_account_id
                        AND identifier = t.tag ->> 'epc');

    INSERT INTO tags (account_id, asset_id, identifier, type)
    SELECT DISTINCT topic_account_id,
                    e.id,
                    t.tag ->> 'epc',
                    'unknown' as "type"
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN assets e ON e.account_id = topic_account_id AND e.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT *
                      FROM tags e
                      WHERE e.account_id = topic_account_id
                        AND e.identifier = t.tag ->> 'epc');

    -- Insert the processed tags into events
    INSERT INTO events (asset_id, location_id, signal_strength, device_timestamp)
    SELECT e.asset_id                                                   AS asset_id,
           a.location_id                                                 AS location_id,
           (t.tag ->> 'rssi')::real                                      AS signal_strength,
           to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000)    AS device_timestamp
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN antennas a ON a.account_id = topic_account_id AND a.identifier = t.tag ->> 'capturePointName'
             JOIN tags e ON e.account_id = topic_account_id AND e.identifier = t.tag ->> 'epc';

-- Return the NEW record to complete the trigger
    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        -- Log error details if needed
        RAISE WARNING 'Error processing message: %', SQLERRM;
        RETURN NULL;
END;
$$;

-- auto-generated definition
create or replace trigger messages_insert_trigger
    after insert
    on messages
    for each row
execute procedure process_messages();

