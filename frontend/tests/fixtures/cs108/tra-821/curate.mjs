#!/usr/bin/env node
// One-off curation: read the raw bridge log, segment into scan sessions
// bounded by ESC_START/ESC_STOP, classify each session's 0x9100 shape,
// and emit a structured fixture file.
//
// Run: node curate.mjs raw-bridge-log-100-cycles.json > curated.json

import { readFileSync } from 'node:fs';
import { argv } from 'node:process';

const path = argv[2];
const { logs } = JSON.parse(readFileSync(path, 'utf8'));

// Parse a CS108 packet header off the front of a hex frame.
// Returns { length, module, event, payload } or null if it's a continuation fragment.
function parseHeader(hex) {
  const bytes = hex.split(' ').map(b => parseInt(b, 16));
  if (bytes.length < 10) return null;
  if (bytes[0] !== 0xA7) return null;
  return {
    length: bytes[2],            // CS108 length byte (payload after 8-B header)
    module: bytes[3],
    direction: bytes[5],
    event: (bytes[8] << 8) | bytes[9],
    headerHex: bytes.slice(0, 10).map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' '),
    payloadStart: bytes.slice(10).map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' '),
    totalBytes: bytes.length,
    expectedTotal: 8 + bytes[2],
    fullBytes: bytes,
  };
}

// Stitch BLE-fragmented bytes back into CS108 packets.
// Each entry that starts with A7 B3 begins a packet; subsequent entries are
// continuation bytes belonging to the most recently started packet UNTIL we
// have collected `expectedTotal` bytes.
function stitchPackets(txEntries) {
  const packets = [];
  let current = null;
  let collected = 0;

  for (const e of txEntries) {
    const bytes = e.hex.split(' ').map(b => parseInt(b, 16));
    if (bytes[0] === 0xA7 && bytes.length >= 8) {
      if (current && collected < current.expectedTotal) {
        // Previous packet was incomplete — bail and discard
        current = null;
      }
      const len = bytes[2];
      current = {
        startTs: e.timestamp,
        endTs: e.timestamp,
        expectedTotal: 8 + len,
        event: (bytes[8] << 8) | bytes[9],
        module: bytes[3],
        bytes: [...bytes],
        bleFrames: [e],
      };
      collected = bytes.length;
      if (collected >= current.expectedTotal) {
        packets.push(current);
        current = null;
        collected = 0;
      }
    } else if (current) {
      current.bytes.push(...bytes);
      current.bleFrames.push(e);
      current.endTs = e.timestamp;
      collected += bytes.length;
      if (collected >= current.expectedTotal) {
        // Truncate to expected length
        current.bytes = current.bytes.slice(0, current.expectedTotal);
        packets.push(current);
        current = null;
        collected = 0;
      }
    }
  }
  return packets;
}

// Group packets into scan sessions bounded by RX `1B 33` (ESC start) and
// RX `1B 30` (ESC stop). For each session, list its TX packets in order.
function segmentSessions(logs) {
  const sessions = [];
  let current = null;

  for (const e of logs) {
    if (e.direction === 'RX' && e.hex.includes('90 03 1B 33')) {
      if (current) sessions.push(current);
      current = { startTs: e.timestamp, endTs: null, startRX: e.hex, txEntries: [], stopRX: null };
    } else if (e.direction === 'RX' && e.hex.includes('90 03 1B 30')) {
      if (current) {
        current.endTs = e.timestamp;
        current.stopRX = e.hex;
        sessions.push(current);
        current = null;
      }
    } else if (e.direction === 'TX') {
      if (current) current.txEntries.push(e);
    }
  }
  if (current) sessions.push(current);
  return sessions;
}

// Classify a scan session by shape, based on its stitched CS108 packets.
function classifySession(session) {
  const packets = stitchPackets(session.txEntries);
  const dataPackets = packets.filter(p => p.event === 0x9100);
  const goodReads = packets.filter(p => p.event === 0x9101);
  const ackPackets = packets.filter(p => p.event === 0x9003);

  // Skip pure status pings (1 byte payload of 0x06)
  const realDataPackets = dataPackets.filter(p => !(p.expectedTotal === 11 && p.bytes[10] === 0x06));
  const statusPings = dataPackets.length - realDataPackets.length;

  // Count records by scanning each data packet's payload for 0x0D
  const totalRecords = realDataPackets.reduce((sum, p) => {
    const payload = p.bytes.slice(10, p.expectedTotal);
    return sum + payload.filter(b => b === 0x0D).length;
  }, 0);

  // Is any data packet incomplete (no 0x0D in its payload)?
  const hasIncomplete = realDataPackets.some(p => {
    const payload = p.bytes.slice(10, p.expectedTotal);
    return !payload.includes(0x0D);
  });

  let shape;
  if (realDataPackets.length === 0) shape = 'NO_DATA';
  else if (realDataPackets.length === 1 && totalRecords === 1) shape = 'CLEAN_SINGLE';
  else if (realDataPackets.length === 2 && totalRecords === 1) shape = 'DATA_SPLIT_2PKT';
  else if (realDataPackets.length === 2 && totalRecords === 2) shape = 'BUNDLED_SECOND_PKT';
  else if (realDataPackets.length === 1 && totalRecords > 1) shape = 'BUNDLED_SINGLE_PKT';
  else if (realDataPackets.length > 2 && totalRecords === 1) shape = 'DATA_SPLIT_3PKT_PLUS';
  else if (totalRecords > 1) shape = 'BUNDLED_MIXED';
  else if (hasIncomplete) shape = 'INCOMPLETE_UNTERMINATED';
  else shape = 'OTHER';

  return {
    startTs: session.startTs,
    endTs: session.endTs,
    shape,
    counts: {
      dataPackets: dataPackets.length,
      realDataPackets: realDataPackets.length,
      statusPings,
      goodReads: goodReads.length,
      ackPackets: ackPackets.length,
      totalRecords,
    },
    dataPacketHex: realDataPackets.map(p => ({
      ts: p.startTs,
      lenByte: p.bytes[2],
      expectedTotal: p.expectedTotal,
      hex: p.bytes.map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' '),
      payloadHex: p.bytes.slice(10, p.expectedTotal).map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' '),
    })),
    goodReadTimes: goodReads.map(p => p.startTs),
    ackPacketTimes: ackPackets.map(p => p.startTs),
  };
}

const sessions = segmentSessions(logs);
const classified = sessions.map(classifySession);

// Aggregate
const shapeCounts = {};
for (const s of classified) {
  shapeCounts[s.shape] = (shapeCounts[s.shape] || 0) + 1;
}

// Pick one canonical example per shape (the first occurrence)
const canonicals = {};
for (const s of classified) {
  if (!canonicals[s.shape]) {
    canonicals[s.shape] = s;
  }
}

const output = {
  source: path,
  totalSessions: classified.length,
  shapeCounts,
  canonicals,
  allSessions: classified,
};

process.stdout.write(JSON.stringify(output, null, 2));
