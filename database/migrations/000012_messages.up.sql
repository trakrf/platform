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
CREATE INDEX idx_messages_org_time ON messages (message_topic, message_timestamp DESC);

-- Create index on common JSON fields if they exist in message_data
CREATE INDEX idx_messages_message_type ON messages ((message_data ->> 'type'));

-- Row Level Security
ALTER TABLE messages
    ENABLE ROW LEVEL SECURITY;

-- todo: add org_id back in populated by trigger that parses topic
-- -- Create policies for each table
-- CREATE POLICY org_isolation_messages ON messages
--    USING (org_id = current_setting('app.current_org_id')::INT);

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
    topic_org_id INT;
    device_name      TEXT;
BEGIN

    SELECT o.id INTO topic_org_id FROM organizations o WHERE o.domain = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %. Be sure that topic matches organization domain', NEW.message_topic;
    END IF;

    INSERT INTO locations (org_id, identifier, name)
    SELECT DISTINCT topic_org_id,
                    t.tag ->> 'capturePointName',
                    t.tag ->> 'capturePointName' || ' auto added from message' as name
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT *
                      FROM scan_points sp
                      WHERE sp.org_id = topic_org_id
                        AND sp.identifier = t.tag ->> 'capturePointName');

    IF NOT EXISTS (SELECT *
                   FROM scan_devices sd
                   WHERE sd.org_id = topic_org_id
                     AND sd.identifier = NEW.message_data ->> 'rfidReaderName') THEN
        INSERT INTO scan_devices (org_id, identifier, name, type)
        VALUES (topic_org_id,
                NEW.message_data ->> 'rfidReaderName',
                NEW.message_data ->> 'rfidReaderName' || ' auto added from message',
                'unknown');
    END IF;

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name)
    SELECT DISTINCT topic_org_id,
                    (SELECT id
                     FROM scan_devices
                     WHERE org_id = topic_org_id
                       AND identifier = NEW.message_data ->> 'rfidReaderName') AS scan_device_id,
                    l.id,
                    t.tag ->> 'capturePointName',
                    t.tag ->> 'capturePointName' || ' auto added from message' as name
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN locations l ON l.org_id = topic_org_id AND l.identifier = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (SELECT *
                      FROM scan_points sp
                      WHERE sp.org_id = topic_org_id
                        AND sp.identifier = t.tag ->> 'capturePointName');

    INSERT INTO assets (org_id, identifier, name, type)
    SELECT DISTINCT topic_org_id,
                    t.tag ->> 'epc',
                    t.tag ->> 'epc' || ' auto added from message' as name,
                    'unknown'                                     as "type"
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT *
                      FROM assets
                      WHERE org_id = topic_org_id
                        AND identifier = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT *
                      FROM identifiers
                      WHERE org_id = topic_org_id
                        AND value = t.tag ->> 'epc');

    INSERT INTO identifiers (org_id, asset_id, value, type)
    SELECT DISTINCT topic_org_id,
                    a.id,
                    t.tag ->> 'epc',
                    'rfid' as "type"
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN assets a ON a.org_id = topic_org_id AND a.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT *
                      FROM identifiers i
                      WHERE i.org_id = topic_org_id
                        AND i.value = t.tag ->> 'epc');

    -- Insert the processed scans into asset_scans
    INSERT INTO asset_scans (org_id, asset_id, location_id, scan_point_id, timestamp)
    SELECT topic_org_id,
           i.asset_id                                                    AS asset_id,
           sp.location_id                                                AS location_id,
           sp.id                                                         AS scan_point_id,
           to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000) AS timestamp
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
             JOIN scan_points sp ON sp.org_id = topic_org_id AND sp.identifier = t.tag ->> 'capturePointName'
             JOIN identifiers i ON i.org_id = topic_org_id AND i.value = t.tag ->> 'epc';

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

