#!/bin/sh

# Docker Compose handles the wait via healthcheck dependency
# (depends_on: timescaledb: condition: service_healthy)
echo "Starting Redpanda Connect ingester..."
echo "MQTT → Database: ${MQTT_TOPIC} → identifier_scans table"

/redpanda-connect run connect.yaml
