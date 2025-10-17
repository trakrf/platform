# CS108 RFID Tag Inventory Parsing Logic Summary

## Tag Inventory Response Formats

The CS108 reader provides tag inventory data in two main formats:

1. **Normal Mode Inventory Response** (pkt_ver = 0x03)
2. **Compact Mode Inventory Response** (pkt_ver = 0x04)

The mode is determined by the `inv_mode` bit (bit 26) in the `INV_CFG` register (0x0901).

## Normal Mode Inventory Packet Structure

```
Byte 0: pkt_ver (0x03)
Byte 1: flags (CRC validity, FastID data, Phase data, etc.)
Bytes 2-3: pkt_type (0x8005 for low level or 0x0005 for high level)
Bytes 4-5: pkt_len (variable)
Bytes 6-7: reserved
Bytes 8-11: ms_ctr (millisecond counter when tag was inventoried)
Byte 12: wb_rssi (Wideband RSSI)
Byte 13: nb_rssi (Narrowband RSSI)
Byte 14: phase (tag phase data)
Byte 15: chidx (current channel index)
Byte 16: data1_count (DATA1 word length)
Byte 17: data2_count (DATA2 word length)
Bytes 18-19: port (current antenna port)
Bytes 20+: inv_data (PC + EPC + CRC16)
```

The tag data (inv_data) length can be calculated with:
`((pkt_len – 3) * 4) – ((flags >> 6) & 3)`

## Compact Mode Inventory Packet Structure

```
Byte 0: pkt_ver (0x04)
Byte 1: flags (CRC validity)
Bytes 2-3: pkt_type (0x8005 for low level or 0x0005 for high level)
Bytes 4-5: pkt_len (payload length in bytes)
Byte 6: Antenna Port# (0-15)
Byte 7: Reserved
Bytes 8+: payload (multiple tag data sets)
```

Each tag data in the payload is formatted as:
`PC(2 bytes) + EPC(EPC length) + NB_RSSI(1 byte)`

The EPC length is calculated from the PC value: `((PC >> 11) * 2)`

## RSSI Value Calculation

For Narrowband RSSI (nb_rssi):
- Mantissa = bits 2:0
- Exponent = bits 7:3
- Value in dB = 20 * log10(2^Exponent * (1 + Mantissa / 2^3))

## Phase Data Calculation (Normal Mode Only)

- Phase data is in bits 0-5 of byte 14
- Phase in Radians = `<bit 0 to bit 5> × 2 × π / 128`
- Phase in Degrees = Phase in Radians × 180 / π

## Key Considerations

1. **Byte Ordering**: RFID firmware uses "reversely populated" (byte-swapped) order for multi-byte fields
2. **Sequence Number**: For tag data uplink, the header reserve byte (byte 4) contains a sequence number incrementing from 0 to 255
3. **Padding Bytes**: The flags field indicates padding bytes added to the end of the packet to force 32-bit boundary
4. **RSSI Filtering**: Optional filtering can be configured using the `HST_INV_RSSI_FILTERING_CONFIG` and `HST_INV_RSSI_FILTERING_THRESHOLD` registers