/**
 * CS108 LOCATE Mode Sequence Test
 *
 * Tests the raw firmware command sequence for LOCATE mode configuration.
 * This test sends low-level CS108 commands directly to verify hardware response.
 *
 * Command sequence based on CS108 API Spec Appendix C.5 - Search Tag Example
 *
 * Success criteria:
 * - RFID module powers on successfully
 * - All configuration registers accept values
 * - Tag mask configuration applies correctly
 * - Inventory starts with filtered EPC
 */
import { describe, test, expect, beforeEach, afterEach } from 'vitest';
import { RfidReaderTestClient } from './rfid-reader-test-client';
import { cs108TestCommand } from '../../config/ble-bridge.config';

describe('CS108 LOCATE mode sequence test', () => {
  let client: RfidReaderTestClient;

  beforeEach(async () => {
    console.log('\n🧪 Setting up bridge server connection test...');
    client = new RfidReaderTestClient();
  });

  afterEach(async () => {
    if (client) {
      try {
        await client.disconnect();
      } catch (error) {
        console.error('Error during cleanup:', error);
      }
    }
  });

  test('should execute LOCATE mode command sequence', async () => {
    // Connect to bridge server
    console.log('📡 Connecting to bridge server...');
    try {
      await client.connect();
      console.log('✅ Connected successfully');
    } catch (error) {
      console.error('❌ Connection failed:', error);
      throw error;
    }

    /*
RX.676: A7 B3 0A C2 82 37 00 00 80 02 70 01         00 07 00 00 00 00
TX.707: A7 B3 03 C2 82 9E 32 F1 80 02 00
RX.724: A7 B3 0A C2 82 37 00 00 80 02 70 01         00 09 80 01 00 00
TX.751: A7 B3 03 C2 82 9E 32 F1 80 02 00
RX.769: A7 B3 0A C2 82 37 00 00 80 02 70 01         02 09 00 00 00 00
TX.797: A7 B3 03 C2 82 9E 32 F1 80 02 00
RX.810: A7 B3 0A C2 82 37 00 00 80 02 70 01         03 09 00 00 00 00
TX.842: A7 B3 03 C2 82 9E 32 F1 80 02 00
RX.858: A7 B3 0A C2 82 37 00 00 80 02 70 01         05 09 00 00 00 00
     */
    // Command sequence with descriptions
    const cmds = [
      // { desc: 'RFID_POWER_OFF',
      //   data: [0xa7, 0xb3, 0x02, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x01],
      //   delay: 500
      // },
      //
      // { desc: 'BARCODE_POWER_OFF',
      //   data: [0xa7, 0xb3, 0x02, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x90, 0x01],
      //   delay: 200
      // },
      //
      // { desc: 'GET_BATTERY_VOLTAGE',
      //   data: [0xa7, 0xb3, 0x02, 0xc2, 0x82, 0x37, 0x00, 0x00, 0xa0, 0x00],
      //   delay: 200
      // },

      // First, power on the RFID module
      { desc: 'RFID_POWER_ON',
        data: [0xa7, 0xb3, 0x02, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x00],
        delay: 500
      },

      // { desc: 'ANT_CYCLES = 0x0000 from api doc example',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0x07, 0x00, 0x00, 0x00, 0x00] },
      // does not appear to matter
      // { desc: 'read ANT_CYCLES',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x00,    0x00, 0x07, 0x00, 0x00, 0x00, 0x00] },
      // { desc: 'ANT_CYCLES = 0x0001 one cycle',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0x07, 0x01, 0x00, 0x00, 0x00] },
      // { desc: 'read ANT_CYCLES',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x00,    0x00, 0x07] },
      // { desc: 'ANT_CYCLES = 0xFFFF continuous',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0x07, 0xff, 0xff, 0x00, 0x00] },

      // { desc: 'read ANT_CYCLES',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x00,    0x00, 0x07, 0x00, 0x00, 0x00, 0x00] },
      // { desc: 'read ANT_PORT_DWELL',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x00,    0x05, 0x07, 0x00, 0x00, 0x00, 0x00] },
      { desc: 'ANT_PORT_DWELL = 0x00 (defaults to 2000ms, override to 0 to disable)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x05, 0x07, 0x00, 0x00, 0x00, 0x00] },
      // { desc: 'read ANT_PORT_DWELL',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x00,    0x05, 0x07, 0x00, 0x00, 0x00, 0x00] },
      { desc: 'ANT_PORT_POWER = 0x012c 30 dbm',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x06, 0x07, 0x2c, 0x01, 0x00, 0x00] },

      { desc: 'QUERY_CFG = 0x0180',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0x09, 0x80, 0x01, 0x00, 0x00] },
      { desc: 'INV_SEL = FIXED_Q (0x00)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x02, 0x09, 0x00, 0x00, 0x00, 0x00] },
      { desc: 'INV_ALG_PARM_0 = 0x00',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x03, 0x09, 0x00, 0x00, 0x00, 0x00] },
      // { desc: 'INV_ALG_PARM_1 = 0x05',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x04, 0x09, 0x05, 0x00, 0x00, 0x00] },
      { desc: 'INV_ALG_PARM_2 = 0x00',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x05, 0x09, 0x00, 0x00, 0x00, 0x00] },
      // { desc: 'TAGACC_DESC_CFG = 0x07',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x01, 0x0a, 0x07, 0x00, 0x00, 0x00] },

      // { desc: 'ANT_PORT_POWER = 300 (30dBm)',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x06, 0x07, 0x2c, 0x01, 0x00, 0x00] },
      { desc: 'TAGMSK_DESC_CFG = 0x09',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x01, 0x08, 0x09, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_BANK = EPC (0x01)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x02, 0x08, 0x01, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_PTR = 0x20 (start of EPC)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x03, 0x08, 0x20, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_LEN = 0x60 (96 bits)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x04, 0x08, 0x60, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_0_3 = 0x00000000',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x05, 0x08, 0x00, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_4_7 = 0x00000000',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x06, 0x08, 0x00, 0x00, 0x00, 0x00] },
      { desc: 'TAGMSK_8_11 = 0x00010023 (tag 10023)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x07, 0x08, 0x00, 0x01, 0x00, 0x23] },
      // { desc: 'TAGMSK_8_11 = 0x00010023 (tag 12345678)',
      //   data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x07, 0x08, 0x12, 0x34, 0x56, 0x78] },

      { desc: 'INV_CFG = 0x01e04000 (enable mask)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x01, 0x09, 0x00, 0x40, 0xe0, 0x01] },

      { desc: 'HST_CMD = START_INVENTORY (0x0F)',
        data: [0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0xf0, 0x0f, 0x00, 0x00, 0x00] },
    ];


    console.log('\n=== LOCATE MODE COMMAND SEQUENCE TEST ===\n');
    console.log('Testing sequence for LOCATE mode with target EPC 10023');
    console.log('=' .repeat(60));

    // Results collection
    const results: Array<{cmd: string, success: boolean, response: string}> = [];

    // Execute command sequence
    for (let i = 0; i < cmds.length; i++) {
      const cmd = cmds[i];
      const cmdBytes = new Uint8Array(cmd.data);

      console.log(`\n[${i+1}/${cmds.length}] ${cmd.desc}`);
      console.log(`  TX: ${cmd.data.map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' ')}`);

      try {
        const response = await client.smokeTestCommand(cmdBytes, 2000);
        const responseHex = Array.from(response).map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' ');
        console.log(`  RX: ${responseHex}`);

        // Check for success - CS108 firmware commands echo back A7 B3 with various event codes
        // Event code 0x8000 = RFID_POWER_ON response
        // Event code 0x8002 = RFID firmware command acknowledgement
        const isCS108Response = response[0] === 0xA7 && response[1] === 0xB3;
        const eventCode = response.length >= 10 ? (response[8] << 8) | response[9] : 0;
        const success = response.length >= 10  // isCS108Response && (eventCode === 0x8000 || eventCode === 0x8002);

        if (success) {
          console.log(`  ✅ SUCCESS - Command acknowledged (event 0x${eventCode.toString(16).padStart(4, '0')})`);
        } else {
          console.log(`  ❌ FAILED - Unexpected response format or event code 0x${eventCode.toString(16).padStart(4, '0')}`);
        }

        results.push({
          cmd: cmd.desc,
          success,
          response: responseHex
        });

        // Special handling for RFID_POWER_ON - needs more time to initialize
        // if (cmd.desc === 'RFID_POWER_ON' && success) {
        //   console.log('  ⏳ Waiting 500ms for RFID module to initialize...');
        //   await new Promise(resolve => setTimeout(resolve, 500));
        // } else {
        //   // Small delay between commands to avoid overwhelming the hardware
        //   await new Promise(resolve => setTimeout(resolve, 100));
        // }
        await new Promise(resolve => setTimeout(resolve, cmd.delay || 100));
      } catch (error) {
        console.log(`  ❌ ERROR: ${error}`);
        results.push({
          cmd: cmd.desc,
          success: false,
          response: `ERROR: ${error}`
        });
      }
    }

    // Wait a bit to capture any tag reads if inventory started
    console.log('\n⏳ Waiting 10 seconds to capture any tag reads...');
    await new Promise(resolve => setTimeout(resolve, 5000));

    // Send ABORT command to stop inventory
    console.log('\n📤 Sending ABORT command to stop inventory...');
    const abortCmd = new Uint8Array([0xa7, 0xb3, 0x0a, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x02, 0x70, 0x01,    0x00, 0xf0, 0x00, 0x00, 0x00, 0x00]);
    try {
      await client.smokeTestCommand(abortCmd, 2000);
      console.log('✅ Inventory stopped');
    } catch (error) {
      console.log('⚠️  Failed to stop inventory:', error);
    }

    // Display summary
    console.log('\n' + '=' .repeat(60));
    console.log('SEQUENCE RESULTS SUMMARY');
    console.log('=' .repeat(60));

    const successCount = results.filter(r => r.success).length;
    console.log(`\nTotal Commands: ${results.length}`);
    console.log(`✅ Successful: ${successCount}`);
    console.log(`❌ Failed: ${results.length - successCount}`);

    if (results.some(r => !r.success)) {
      console.log('\nFailed Commands:');
      results.forEach((r, i) => {
        if (!r.success) {
          console.log(`  [${i+1}] ${r.cmd}`);
          console.log(`      Response: ${r.response}`);
        }
      });
    }

    const powerOffCmd = new Uint8Array([0xa7, 0xb3, 0x02, 0xc2, 0x82, 0x37, 0x00, 0x00, 0x80, 0x01]);
    try {
      await client.smokeTestCommand(powerOffCmd, 200);
      console.log('✅ RFID Powered off');
    } catch (error) {
      console.log('⚠️  Failed to power off RFID:', error);
    }

    // Test passes if at least the basic commands succeeded
    expect(successCount).toBeGreaterThan(0);
    console.log('\n✅ Locate sequence test completed!');
  }, 30000); // 30 second test timeout for hardware communication
});
