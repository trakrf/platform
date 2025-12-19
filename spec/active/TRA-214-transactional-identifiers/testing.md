# TRA-214 API Testing Guide

Manual testing guide for transactional asset/location identifiers feature.

## Prerequisites

1. Backend running on `localhost:8080`
2. Valid user credentials (e.g., `test1@test.com` / `password`)
3. `curl` and `jq` installed

## Authentication

All endpoints require a Bearer token. Get one first:

```bash
# Get auth token
AUTH_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "test1@test.com", "password": "password"}')

TOKEN=$(echo "$AUTH_RESPONSE" | jq -r '.data.token')
echo "Token: ${TOKEN:0:50}..."
```

## Asset Endpoints

### Create Asset with Identifiers

```bash
curl -s -X POST http://localhost:8080/api/v1/assets \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "identifier": "ASSET-001",
    "name": "Forklift A",
    "type": "asset",
    "description": "Main warehouse forklift",
    "valid_from": "2025-01-01T00:00:00Z",
    "is_active": true,
    "identifiers": [
      {"type": "rfid", "value": "E200001234567890"},
      {"type": "barcode", "value": "BC-FORK-001"}
    ]
  }' | jq .
```

**Expected Response:**
```json
{
  "data": {
    "id": 123456789,
    "identifier": "ASSET-001",
    "name": "Forklift A",
    "identifiers": [
      {"id": 111, "type": "rfid", "value": "E200001234567890", "is_active": true},
      {"id": 222, "type": "barcode", "value": "BC-FORK-001", "is_active": true}
    ]
  }
}
```

### Create Asset without Identifiers

```bash
curl -s -X POST http://localhost:8080/api/v1/assets \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "identifier": "ASSET-002",
    "name": "Pallet Jack B",
    "type": "asset",
    "valid_from": "2025-01-01T00:00:00Z",
    "is_active": true
  }' | jq .
```

### Get Asset (includes identifiers)

```bash
curl -s -X GET http://localhost:8080/api/v1/assets/{asset_id} \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### List Assets (includes identifiers)

```bash
curl -s -X GET "http://localhost:8080/api/v1/assets?limit=10&offset=0" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### Add Identifier to Existing Asset

```bash
curl -s -X POST http://localhost:8080/api/v1/assets/{asset_id}/identifiers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "type": "ble",
    "value": "AA:BB:CC:DD:EE:FF"
  }' | jq .
```

### Remove Identifier from Asset

```bash
curl -s -X DELETE http://localhost:8080/api/v1/assets/{asset_id}/identifiers/{identifier_id} \
  -H "Authorization: Bearer $TOKEN" | jq .
```

## Location Endpoints

### Create Location with Identifiers

```bash
curl -s -X POST http://localhost:8080/api/v1/locations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "identifier": "LOC-WH-001",
    "name": "Warehouse Zone A",
    "description": "Main storage area",
    "valid_from": "2025-01-01T00:00:00Z",
    "is_active": true,
    "identifiers": [
      {"type": "rfid", "value": "E200ZONE0001"},
      {"type": "barcode", "value": "BC-ZONE-A"}
    ]
  }' | jq .
```

### Get Location (includes identifiers)

```bash
curl -s -X GET http://localhost:8080/api/v1/locations/{location_id} \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### Get Location with Relations

```bash
curl -s -X GET "http://localhost:8080/api/v1/locations/{location_id}?include=relations" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### List Locations (includes identifiers)

```bash
curl -s -X GET "http://localhost:8080/api/v1/locations?limit=10&offset=0" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### Add Identifier to Existing Location

```bash
curl -s -X POST http://localhost:8080/api/v1/locations/{location_id}/identifiers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "type": "nfc",
    "value": "NFC-ZONE-A-001"
  }' | jq .
```

### Remove Identifier from Location

```bash
curl -s -X DELETE http://localhost:8080/api/v1/locations/{location_id}/identifiers/{identifier_id} \
  -H "Authorization: Bearer $TOKEN" | jq .
```

## Lookup Endpoint

### Lookup by Tag Value

Find an asset or location by its tag identifier:

```bash
# Lookup by RFID
curl -s -X GET "http://localhost:8080/api/v1/lookup/tag?type=rfid&value=E200001234567890" \
  -H "Authorization: Bearer $TOKEN" | jq .

# Lookup by barcode
curl -s -X GET "http://localhost:8080/api/v1/lookup/tag?type=barcode&value=BC-FORK-001" \
  -H "Authorization: Bearer $TOKEN" | jq .

# Lookup by BLE
curl -s -X GET "http://localhost:8080/api/v1/lookup/tag?type=ble&value=AA:BB:CC:DD:EE:FF" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Expected Response (asset found):**
```json
{
  "data": {
    "entity_type": "asset",
    "entity_id": 123456789,
    "asset": {
      "id": 123456789,
      "identifier": "ASSET-001",
      "name": "Forklift A"
    }
  }
}
```

**Expected Response (location found):**
```json
{
  "data": {
    "entity_type": "location",
    "entity_id": 987654321,
    "location": {
      "id": 987654321,
      "identifier": "LOC-WH-001",
      "name": "Warehouse Zone A"
    }
  }
}
```

**Expected Response (not found):**
```json
{
  "error": {
    "type": "not_found",
    "title": "No entity found with this tag",
    "status": 404
  }
}
```

## Tag Identifier Types

Supported tag types:
- `rfid` - RFID tags (e.g., EPC Gen2)
- `barcode` - Barcodes (1D/2D)
- `ble` - Bluetooth Low Energy beacons
- `nfc` - NFC tags
- `qr` - QR codes

## Error Responses

### Duplicate Tag Value

```json
{
  "error": {
    "type": "internal_error",
    "title": "Failed to create asset",
    "status": 500,
    "detail": "one or more tag identifiers already exist"
  }
}
```

### Invalid Tag Type

```json
{
  "error": {
    "type": "validation_error",
    "title": "Validation failed",
    "status": 400,
    "detail": "Key: 'TagIdentifierRequest.Type' Error:Field validation for 'Type' failed"
  }
}
```

## Full Test Script

Save this as `test_tra214.sh` and run with `bash test_tra214.sh`:

```bash
#!/bin/bash
set -e

BASE_URL="http://localhost:8080"

echo "=== TRA-214 Feature Test ==="
echo ""

# 1. Get auth token
echo "1. Getting auth token..."
AUTH=$(curl -s -X POST $BASE_URL/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "test1@test.com", "password": "password"}')
TOKEN=$(echo "$AUTH" | jq -r '.data.token')
echo "   Token obtained: ${TOKEN:0:30}..."

# 2. Create asset with identifiers
echo ""
echo "2. Creating asset with identifiers..."
ASSET=$(curl -s -X POST $BASE_URL/api/v1/assets \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "identifier": "TEST-'$(date +%s)'",
    "name": "Test Asset",
    "type": "asset",
    "valid_from": "2025-01-01T00:00:00Z",
    "is_active": true,
    "identifiers": [
      {"type": "rfid", "value": "RFID-'$(date +%s)'"},
      {"type": "barcode", "value": "BC-'$(date +%s)'"}
    ]
  }')

ASSET_ID=$(echo "$ASSET" | jq -r '.data.id // empty')
if [ -z "$ASSET_ID" ]; then
  echo "   FAILED: $(echo "$ASSET" | jq -r '.error.detail')"
  exit 1
fi
echo "   Created asset ID: $ASSET_ID"
echo "   Identifiers: $(echo "$ASSET" | jq -c '.data.identifiers | length') tags attached"

# 3. Get asset and verify identifiers
echo ""
echo "3. Getting asset by ID..."
GET_ASSET=$(curl -s -X GET "$BASE_URL/api/v1/assets/$ASSET_ID" \
  -H "Authorization: Bearer $TOKEN")
IDENT_COUNT=$(echo "$GET_ASSET" | jq '.data.identifiers | length')
echo "   Asset has $IDENT_COUNT identifiers"

# 4. Lookup by RFID
echo ""
echo "4. Testing lookup by RFID..."
RFID_VALUE=$(echo "$ASSET" | jq -r '.data.identifiers[0].value')
LOOKUP=$(curl -s -X GET "$BASE_URL/api/v1/lookup/tag?type=rfid&value=$RFID_VALUE" \
  -H "Authorization: Bearer $TOKEN")
ENTITY_TYPE=$(echo "$LOOKUP" | jq -r '.data.entity_type // empty')
if [ "$ENTITY_TYPE" = "asset" ]; then
  echo "   SUCCESS: Found asset via RFID lookup"
else
  echo "   FAILED: Lookup returned: $(echo "$LOOKUP" | jq -c '.')"
  exit 1
fi

# 5. Add identifier to existing asset
echo ""
echo "5. Adding new identifier to asset..."
ADD_IDENT=$(curl -s -X POST "$BASE_URL/api/v1/assets/$ASSET_ID/identifiers" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"type": "ble", "value": "BLE-'$(date +%s)'"}')
NEW_IDENT_ID=$(echo "$ADD_IDENT" | jq -r '.data.id // empty')
if [ -n "$NEW_IDENT_ID" ]; then
  echo "   Added identifier ID: $NEW_IDENT_ID"
else
  echo "   FAILED: $(echo "$ADD_IDENT" | jq -r '.error.detail')"
fi

# 6. Remove identifier
echo ""
echo "6. Removing identifier..."
DEL_RESULT=$(curl -s -X DELETE "$BASE_URL/api/v1/assets/$ASSET_ID/identifiers/$NEW_IDENT_ID" \
  -H "Authorization: Bearer $TOKEN")
DELETED=$(echo "$DEL_RESULT" | jq -r '.deleted // false')
echo "   Deleted: $DELETED"

echo ""
echo "=== All tests passed! ==="
```

## Cleanup

To delete test data, use the standard DELETE endpoints:

```bash
curl -s -X DELETE http://localhost:8080/api/v1/assets/{asset_id} \
  -H "Authorization: Bearer $TOKEN" | jq .
```
