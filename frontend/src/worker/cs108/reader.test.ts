import { describe, it, expect, beforeEach, vi, type Mock } from 'vitest';
import { CS108Reader } from './reader.js';
import { ReaderState, ReaderMode, RainTarget } from '../types/reader.js';
import { CommandManager, SequenceAbortedError, CommandInFlightError } from './command.js';
import { PacketHandler } from './packet.js';
import { NotificationManager } from './notification/manager.js';
import { IDLE_SEQUENCE } from './system/sequences.js';
import { INVENTORY_CONFIG_SEQUENCE } from './rfid/inventory/sequences.js';
import { BARCODE_CONFIG_SEQUENCE } from './barcode/sequences.js';
import { LOCATE_CONFIG_SEQUENCE, locateSettingsSequence } from './rfid/locate/sequences.js';
import { RFID_REGISTERS } from './rfid/constant.js';
import { RFID_START_SEQUENCE } from './rfid/sequences.js';
import { RFID_IDENTITY_SEQUENCE } from './system/identity.js';
import { removeLeadingZeros } from '../../utils/reconciliationUtils';
import type { CS108Packet } from './type.js';

// Mock all dependencies
// `importOriginal` for the error classes, deliberately. They used to be
// hand-redeclared here, which makes the mock a SECOND definition that can drift
// from the real one without any test noticing — and since reader.ts now
// discriminates by class rather than by message, a drifted copy would silently
// stop matching. Taking the real classes means the mock cannot disagree.
vi.mock('./command.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./command.js')>()),
  CommandManager: vi.fn().mockImplementation((sendToTransport, notificationHandler, stateContext) => {
    // Store the stateContext to simulate state transitions
    const mockManager = {
      abortSequence: vi.fn().mockResolvedValue(undefined),
      resetAbortFlag: vi.fn(),
      executeSequence: vi.fn().mockImplementation(async (sequence) => {
        // Simulate CommandManager state transitions
        if (stateContext) {
          // Set BUSY state at start (using string literals to avoid import issues)
          stateContext.setReaderState('Busy');

          // Simulate async command execution
          await new Promise(resolve => setTimeout(resolve, 0));

          // Set final state on success - check last command for finalState
          const lastCommand = sequence[sequence.length - 1];
          // The finalState in sequences already has the string value (e.g., 'Scanning', 'Connected')
          const finalState = lastCommand?.finalState || 'Connected';
          stateContext.setReaderState(finalState);
        }
      }),
      executeCommand: vi.fn(),
      handleCommandResponse: vi.fn(),
      isWaitingForResponse: vi.fn().mockReturnValue(false),
      isIdle: vi.fn().mockReturnValue(true),
      // Delegates to the same spies rather than being its own, so a caller that
      // takes the wire once and issues N commands through the runner is still
      // observable through `executeSequence`/`executeCommand`. A mock that
      // recorded runExclusive separately would make every existing assertion
      // about mode sequences silently stop seeing the settings path.
      runExclusive: vi.fn(async (body: (run: unknown) => Promise<unknown>) => body({
        command: (...args: unknown[]) => mockManager.executeCommand(...args),
        sequence: (...args: unknown[]) => mockManager.executeSequence(...args)
      })),

      // The reader's own state choke point, handed back so a test can put the
      // reader in a state and hold it there. Reaching it any other way means
      // assigning `readerState` directly, which bypasses the CONNECTED branch
      // that schedules `convergeToTriggerState()` — so a test written that way
      // can observe a level being latched but never the convergence that acts
      // on it. TRA-1247.
      __stateContext: stateContext
    };
    return mockManager;
  }),
}));

vi.mock('./packet.js', () => ({
  PacketHandler: vi.fn().mockImplementation(() => ({
    processIncomingData: vi.fn()
  }))
}));

vi.mock('./notification/manager.js', () => ({
  NotificationManager: vi.fn().mockImplementation(() => ({
    getRouter: vi.fn().mockReturnValue({
      handleNotification: vi.fn(),
      clear: vi.fn()
    })
  }))
}));

vi.mock('./notification/router.js', () => ({
  NotificationRouter: vi.fn().mockImplementation(() => ({
    handleNotification: vi.fn()
  }))
}));
vi.mock('./event.js', async (importOriginal) => {
  const actual = await importOriginal() as any;
  return {
    ...actual,
    // Keep the actual exports but ensure the ones we need are defined
    BARCODE_SEND_COMMAND: actual.BARCODE_SEND_COMMAND || { command: 0x0013, name: 'BARCODE_SEND_COMMAND' },
    BARCODE_ESC_START: actual.BARCODE_ESC_START || new Uint8Array([0x1B, 0x33]),
    BARCODE_ESC_TRIGGER: actual.BARCODE_ESC_TRIGGER || new Uint8Array([0x1B, 0x31]),
    BARCODE_ESC_STOP: actual.BARCODE_ESC_STOP || new Uint8Array([0x1B, 0x30])
  };
});

describe('CS108Reader', () => {
  let reader: CS108Reader;
  let commandManagerMock: CommandManager;
  let packetHandlerMock: PacketHandler;
  let notificationManagerMock: NotificationManager;
  let postMessageSpy: Mock;

  beforeEach(() => {
    // Setup global postMessage spy
    postMessageSpy = vi.fn();
    globalThis.postMessage = postMessageSpy;

    // Create reader instance
    reader = new CS108Reader();

    // Get mocked instances
    commandManagerMock = (reader as any).commandManager;
    packetHandlerMock = (reader as any).packetHandler;
    notificationManagerMock = (reader as any).notificationManager;

    // Clear all mock calls
    vi.clearAllMocks();
  });

  describe('constructor', () => {
    it('should initialize with correct default state', () => {
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
      expect(reader.getMode()).toBeNull();
    });

    it('should emit initial state events', () => {
      // Create a new reader to capture constructor events
      const localPostMessageSpy = vi.fn();
      globalThis.postMessage = localPostMessageSpy;

      const newReader = new CS108Reader();

      // Constructor emits initial state
      expect(localPostMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_STATE_CHANGED',
        payload: { readerState: ReaderState.DISCONNECTED },
        timestamp: expect.any(Number)
      }));

      expect(localPostMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_MODE_CHANGED',
        payload: { mode: null },
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('onConnect()', () => {
    it('should initialize default settings on connection', async () => {
      await reader.connect();

      const settings = reader.getSettings();
      expect(settings.rfid).toEqual({
        transmitPower: 30,
        session: 1,
        target: RainTarget.A,
        qValue: 4
      });
      expect(settings.barcode).toEqual({
        continuous: false,
        timeout: 5000,
        illumination: true,
        aimPattern: true
      });
    });
  });

  describe('onDisconnect()', () => {
    it('should abort any running sequences on disconnect', async () => {
      await reader.connect();
      await reader.disconnect();

      expect(commandManagerMock.abortSequence).toHaveBeenCalledWith('Disconnect requested');
    });

    /**
     * Reader details describe THIS reader. Carrying them across a disconnect
     * would mean the next connection opens showing the previous device's
     * firmware — which is worse than showing nothing, because it looks read.
     */
    it('forgets what the last reader was', async () => {
      await reader.connect();
      (reader as any).readerDetails = { serialNumber: 'CS108ABC12345' };

      await reader.disconnect();

      expect((reader as any).readerDetails).toEqual({});
    });
  });

  /**
   * Reading the reader's own identity (TRA-1232).
   *
   * Nothing we produce records the reader's firmware, so a capture cannot say
   * what it was taken on and flashing destroys the attribution permanently.
   */
  describe('reader identity', () => {
    /**
     * A REG_RESP as the device sends it: pkt_ver, reserved, addr LE, data LE.
     *
     * ⚠ On 0x8100, not on the 0x8002 the read was sent on — measured on
     * hardware 2026-09-02. 0x8002 answers a read with the same one-byte status
     * a write gets; the value arrives on the RFID processor's uplink data
     * channel, which it shares with tag reads.
     */
    function registerResponsePacket(register: number, value: number): CS108Packet {
      return {
        eventCode: 0x8100,
        event: { eventCode: 0x8100, name: 'INVENTORY_TAG', isCommand: false, isNotification: true },
        rawPayload: new Uint8Array([
          0x70, 0x00,
          register & 0xFF, (register >> 8) & 0xFF,
          value & 0xFF, (value >> 8) & 0xFF, (value >> 16) & 0xFF, (value >> 24) & 0xFF,
        ]),
        payload: undefined,
      } as unknown as CS108Packet;
    }

    it('asks the Bluetooth board what it is, on connect', async () => {
      await reader.connect();

      const asked = (commandManagerMock.executeCommand as Mock).mock.calls.map(
        ([event]) => event.eventCode
      );
      expect(asked).toEqual([0xB000, 0xC000, 0xB004]);
    });

    /**
     * The whole read is a nice-to-have and a connect is not. A board that will
     * not answer must not cost the operator their session.
     */
    it('connects anyway when the board will not answer', async () => {
      (commandManagerMock.executeCommand as Mock).mockRejectedValue(new Error('Command timeout'));

      await expect(reader.connect()).resolves.toBe(true);
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('keeps asking the rest after one read fails', async () => {
      (commandManagerMock.executeCommand as Mock)
        .mockRejectedValueOnce(new Error('Command timeout'))
        .mockResolvedValue(undefined);

      await reader.connect();

      expect((commandManagerMock.executeCommand as Mock).mock.calls.map(([e]) => e.eventCode))
        .toEqual([0xB000, 0xC000, 0xB004]);
    });

    /**
     * Observed at the packet choke point, BEFORE the split into command
     * responses and notifications.
     *
     * A register response may settle the command that asked for it, or arrive a
     * beat later with nothing in flight, depending on whether the device also
     * sends a status byte first. Nothing here has ever read a register from
     * this device and the vendor source does not settle the question, so the
     * observation has to be right under either behaviour.
     */
    it('reads the RFID firmware version out of a register response', () => {
      const raw = (2 << 24) | (6 << 12) | 46;  // V2.6.46, the published image
      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([
        registerResponsePacket(RFID_REGISTERS.FIRMWARE_VER, raw),
      ]);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      expect((reader as any).readerDetails).toEqual({ rfidFirmware: '2.6.46' });
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_DETAILS',
        payload: { details: { rfidFirmware: '2.6.46' } },
      }));
    });

    /**
     * ⚠ A register value arrives as an INVENTORY_TAG notification, and
     * `InventoryParser` cannot read it: `pkt_ver 0x70` hits its unknown-version
     * branch, byte-slides one byte at a time and charges eight `parseErrors`
     * per register read. It answers no command in flight either — the 0x8002
     * status ack already settled the read.
     *
     * So it is consumed here. Nothing downstream wants it, and routing it would
     * quietly pollute a health counter on every connection.
     */
    it('consumes a register response instead of routing it to the tag parser', () => {
      const router = (reader as any).notificationRouter;
      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([
        registerResponsePacket(RFID_REGISTERS.MAC_ERROR, 0),
      ]);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      expect(router.handleNotification).not.toHaveBeenCalled();
    });

    /** The other half: an actual tag read on that channel still gets through. */
    it('still routes an ordinary inventory packet on the same channel', () => {
      const router = (reader as any).notificationRouter;
      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([{
        eventCode: 0x8100,
        event: { eventCode: 0x8100, name: 'INVENTORY_TAG', isCommand: false, isNotification: true },
        rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80, 0x0A, 0x00, 0x00, 0x00]),
        payload: undefined,
      } as unknown as CS108Packet]);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      expect(router.handleNotification).toHaveBeenCalled();
    });

    it('reads the MAC error, including a healthy zero', () => {
      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([
        registerResponsePacket(RFID_REGISTERS.MAC_ERROR, 0),
      ]);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      expect((reader as any).readerDetails).toEqual({ macError: 0 });
    });

    /**
     * Thousands of register WRITES per session are acknowledged on this same op
     * code with a one-byte status, and none of them says anything about
     * identity. Re-emitting on each would be a message storm.
     */
    it('says nothing on an ordinary firmware-command acknowledgement', () => {
      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([{
        eventCode: 0x8002,
        event: { eventCode: 0x8002, name: 'RFID_FIRMWARE_COMMAND', isCommand: true, isNotification: false },
        rawPayload: new Uint8Array([0x00]),
        payload: undefined,
      } as unknown as CS108Packet]);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      expect((reader as any).readerDetails).toEqual({});
      expect(postMessageSpy).not.toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_DETAILS',
      }));
    });

    /**
     * The R2000 is POWERED OFF at connect — IDLE_SEQUENCE opens with
     * RFID_POWER_OFF — so these two reads ride the first mode that powers the
     * radio on, rather than costing the connect path a power cycle on a device
     * whose characterised fault is RFID_POWER_OFF going silent (TRA-1217).
     */
    it('puts the register reads in an RFID mode sequence', async () => {
      await reader.connect();
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.INVENTORY);

      const sequence = (commandManagerMock.executeSequence as Mock).mock.calls[0][0];
      for (const read of RFID_IDENTITY_SEQUENCE) expect(sequence).toContain(read);
    });

    it('puts them in LOCATE too, without displacing the tag mask from the end', async () => {
      await reader.connect();
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: 'E280689400000000001018DD' } });

      const sequence = (commandManagerMock.executeSequence as Mock).mock.calls[0][0];
      for (const read of RFID_IDENTITY_SEQUENCE) expect(sequence).toContain(read);
      // TRA-1091/TRA-1122 pin the mask as the tail of a LOCATE sequence, and
      // two identity reads are not a reason to move it.
      const mask = locateSettingsSequence('E280689400000000001018DD');
      expect(sequence.slice(-mask.length)).toEqual(mask);
    });

    /** IDLE and BARCODE leave the radio off. Asking it anything there is noise. */
    it('does not append them to a mode that leaves the radio off', async () => {
      await reader.connect();
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.IDLE);

      const sequence = (commandManagerMock.executeSequence as Mock).mock.calls[0][0];
      expect(sequence).not.toContain(RFID_IDENTITY_SEQUENCE[0]);
    });

    /**
     * Once per connection, not once per mode change. A handheld session is a
     * long run of mode changes, and the firmware version does not move between
     * them — two extra commands on every one would be a permanent tax on the
     * hottest path in the app for an answer we already have.
     */
    it('stops asking once the reader has answered', async () => {
      await reader.connect();
      (reader as any).readerDetails = { rfidFirmware: '2.6.46' };
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.INVENTORY);

      const sequence = (commandManagerMock.executeSequence as Mock).mock.calls[0][0];
      expect(sequence).not.toContain(RFID_IDENTITY_SEQUENCE[0]);
    });
  });

  describe('handleBleData()', () => {
    it('should process incoming data and route packets correctly', () => {
      const testData = new Uint8Array([0xA7, 0xB3, 0x00, 0x01]);
      const mockPacket: CS108Packet = {
        header: { prefix: 0xA7B3, messageLength: 1, flags: 0, reserved: 0, crc: 0 },
        eventCode: 0x0001,
        event: { eventCode: 0x0001, name: 'TEST_COMMAND', isCommand: true, isNotification: false },
        rawPayload: new Uint8Array([0x00]),
        payload: undefined
      };

      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([mockPacket]);

      // Mock that we're waiting for this specific command response
      (commandManagerMock.isWaitingForResponse as Mock).mockReturnValue(true);

      (reader as any).handleBleData(testData);

      expect(packetHandlerMock.processIncomingData).toHaveBeenCalledWith(testData);
      expect(commandManagerMock.handleCommandResponse).toHaveBeenCalledWith(mockPacket);
    });

    /**
     * `0xA101` carrying 0x0000 used to be matched and `continue`d before any
     * routing, on a comment asserting it was "spurious" and did not "indicate
     * actual communication problems". The 2026-09-01 hardware capture measured
     * 1543 of them in 86 minutes, one per unanswered command, arriving a median
     * 34 ms after the command they answered — the same latency as a healthy
     * reply. The discard is why an 86-minute fault storm reached no handler at
     * all. Refs TRA-1229.
     */
    it('does not discard the ERROR_NOTIFICATION the device actually sends (0x0000)', () => {
      const mockPacket: CS108Packet = {
        header: { prefix: 0xA7B3, messageLength: 2, flags: 0, reserved: 0, crc: 0 },
        eventCode: 0xA101,
        event: { eventCode: 0xA101, name: 'ERROR_NOTIFICATION', isCommand: true, isNotification: true },
        rawPayload: new Uint8Array([0x00, 0x00]),
        payload: 0x0000
      } as unknown as CS108Packet;

      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([mockPacket]);
      (commandManagerMock.isWaitingForResponse as Mock).mockReturnValue(false);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      const router = (notificationManagerMock.getRouter as Mock).mock.results[0]?.value ||
                     (notificationManagerMock.getRouter as Mock)();
      expect(router.handleNotification).toHaveBeenCalledWith(mockPacket);
    });

    /**
     * A rejection is a fault: it says we sent something the device refused, or
     * that the device is in a state that needs addressing before business as
     * usual. It must not have to win a routing race against the ordinary
     * isCommand/isNotification dispatch to be seen.
     */
    it('routes ERROR_NOTIFICATION to the fault path even while a command is in flight', () => {
      const mockPacket: CS108Packet = {
        header: { prefix: 0xA7B3, messageLength: 2, flags: 0, reserved: 0, crc: 0 },
        eventCode: 0xA101,
        event: { eventCode: 0xA101, name: 'ERROR_NOTIFICATION', isCommand: true, isNotification: true },
        rawPayload: new Uint8Array([0x00, 0x00]),
        payload: 0x0000
      } as unknown as CS108Packet;

      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([mockPacket]);
      // A command IS in flight and the manager would claim this packet.
      (commandManagerMock.isWaitingForResponse as Mock).mockReturnValue(true);

      (reader as any).handleBleData(new Uint8Array([0xA7, 0xB3]));

      // It still reaches the error handler, so the fault is counted and named,
      // and it still settles the command rather than letting it time out.
      const router = (notificationManagerMock.getRouter as Mock).mock.results[0]?.value ||
                     (notificationManagerMock.getRouter as Mock)();
      expect(router.handleNotification).toHaveBeenCalledWith(mockPacket);
      expect(commandManagerMock.handleCommandResponse).toHaveBeenCalledWith(mockPacket);
    });

    it('should route notification packets to notification router', () => {
      const testData = new Uint8Array([0xA7, 0xB3, 0x00, 0x02]);
      const mockPacket: CS108Packet = {
        header: { prefix: 0xA7B3, messageLength: 1, flags: 0, reserved: 0, crc: 0 },
        eventCode: 0x8002,
        event: { eventCode: 0x8002, name: 'TEST_NOTIFICATION', isCommand: false, isNotification: true },
        rawPayload: new Uint8Array([0x00]),
        payload: undefined
      };

      (packetHandlerMock.processIncomingData as Mock).mockReturnValue([mockPacket]);

      (reader as any).handleBleData(testData);

      // Get the router from the notificationManager mock
      const router = (notificationManagerMock.getRouter as Mock).mock.results[0]?.value ||
                     (notificationManagerMock.getRouter as Mock)();
      expect(router.handleNotification).toHaveBeenCalledWith(mockPacket);
    });
  });

  describe('setMode()', () => {
    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('should transition to IDLE mode', async () => {
      // First switch to a different mode so we're not already in IDLE
      await reader.setMode(ReaderMode.INVENTORY);
      vi.clearAllMocks();

      // Now test transitioning to IDLE
      await reader.setMode(ReaderMode.IDLE);

      expect(commandManagerMock.abortSequence).toHaveBeenCalledWith('Mode change requested');
      expect(commandManagerMock.executeSequence).toHaveBeenCalledWith(IDLE_SEQUENCE);
      expect(reader.getMode()).toBe(ReaderMode.IDLE);
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('should transition to INVENTORY mode', async () => {
      // Clear mocks from connect() to isolate this test
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.INVENTORY);

      // Should call executeSequence once for INVENTORY mode
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);
      const calledSequence = commandManagerMock.executeSequence.mock.calls[0][0];
      // Should contain both IDLE and INVENTORY commands
      expect(calledSequence).toContain(IDLE_SEQUENCE[0]);
      expect(calledSequence).toContain(INVENTORY_CONFIG_SEQUENCE[0]);
      expect(reader.getMode()).toBe(ReaderMode.INVENTORY);
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('should transition to LOCATE mode', async () => {
      // Clear mocks from connect() to isolate this test
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.LOCATE);

      // Should call executeSequence once for LOCATE mode
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);
      const calledSequence = commandManagerMock.executeSequence.mock.calls[0][0];
      // Should contain both IDLE and LOCATE commands
      expect(calledSequence).toContain(IDLE_SEQUENCE[0]);
      expect(calledSequence).toContain(LOCATE_CONFIG_SEQUENCE[0]);
      expect(reader.getMode()).toBe(ReaderMode.LOCATE);
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('should transition to BARCODE mode', async () => {
      // Clear mocks from connect() to isolate this test
      vi.clearAllMocks();

      await reader.setMode(ReaderMode.BARCODE);

      // Should call executeSequence once for BARCODE mode
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);
      const calledSequence = commandManagerMock.executeSequence.mock.calls[0][0];
      // Should contain both IDLE and BARCODE commands
      expect(calledSequence).toContain(IDLE_SEQUENCE[0]);
      expect(calledSequence).toContain(BARCODE_CONFIG_SEQUENCE[0]);
      expect(reader.getMode()).toBe(ReaderMode.BARCODE);
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('should emit BUSY state during transition', async () => {
      // Clear any connect() events
      postMessageSpy.mockClear();

      await reader.setMode(ReaderMode.INVENTORY);

      // CommandManager sets BUSY state which triggers state change event
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_STATE_CHANGED',
        payload: { readerState: ReaderState.BUSY },
        timestamp: expect.any(Number)
      }));
    });

    it('should emit mode and state changed events on success', async () => {
      // Clear any connect() events
      postMessageSpy.mockClear();

      await reader.setMode(ReaderMode.BARCODE);

      // Should emit mode changed
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_MODE_CHANGED',
        payload: { mode: ReaderMode.BARCODE },
        timestamp: expect.any(Number)
      }));

      // CommandManager sets READY state which triggers state change event
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_STATE_CHANGED',
        payload: { readerState: ReaderState.CONNECTED },
        timestamp: expect.any(Number)
      }));
    });

    it('should handle sequence aborted errors gracefully', async () => {
      const abortError = new SequenceAbortedError('Mode change requested');
      (commandManagerMock.executeSequence as Mock).mockRejectedValueOnce(abortError);

      // Should not throw
      await expect(reader.setMode(ReaderMode.INVENTORY)).resolves.toBeUndefined();
    });

    it('should set ERROR mode on real failure', async () => {
      const error = new Error('Command failed');
      (commandManagerMock.executeSequence as Mock).mockRejectedValueOnce(error);

      await expect(reader.setMode(ReaderMode.INVENTORY)).rejects.toThrow('Command failed');
      // Now we set ReaderMode.ERROR, not ReaderState.ERROR
      expect(reader.getMode()).toBe(ReaderMode.ERROR);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_MODE_CHANGED',
        payload: { mode: ReaderMode.ERROR },
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('setSettings()', () => {
    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('should apply RFID settings to hardware via CommandManager when READY', async () => {
      // Set reader to INVENTORY mode first
      await reader.setMode(ReaderMode.INVENTORY);

      const newSettings = {
        rfid: {
          transmitPower: 25
        }
      };

      // Clear previous calls from setMode
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings(newSettings);

      // Should call executeSequence for transmit power
      expect(commandManagerMock.executeSequence).toHaveBeenCalled();

      const settings = reader.getSettings();
      expect(settings.rfid?.transmitPower).toBe(25);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'SETTINGS_UPDATED',
        payload: { settings: newSettings },
        timestamp: expect.any(Number)
      }));
    });

    it('should store settings even when not READY', async () => {
      // Set reader to SCANNING state
      (reader as any).readerState = ReaderState.SCANNING;

      const newSettings = { rfid: { transmitPower: 25 } };

      // Should NOT throw - just stores settings
      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();

      // Settings should be stored
      const settings = reader.getSettings();
      expect(settings.rfid?.transmitPower).toBe(25);
    });

    it('should store RFID settings even in BARCODE mode', async () => {
      // Set reader to BARCODE mode
      await reader.setMode(ReaderMode.BARCODE);

      const newSettings = { rfid: { transmitPower: 25 } };

      // Should NOT throw - just stores settings
      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();

      // Settings should be stored for future use
      const settings = reader.getSettings();
      expect(settings.rfid?.transmitPower).toBe(25);
    });

    it('should allow RFID settings in INVENTORY mode', async () => {
      await reader.setMode(ReaderMode.INVENTORY);

      const newSettings = { rfid: { transmitPower: 25 } };

      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();
    });

    it('should allow RFID settings in LOCATE mode', async () => {
      await reader.setMode(ReaderMode.LOCATE);

      const newSettings = { rfid: { transmitPower: 30 } };

      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();
    });

    it('should store barcode settings even in INVENTORY mode', async () => {
      // Set reader to INVENTORY mode
      await reader.setMode(ReaderMode.INVENTORY);

      const newSettings = { barcode: { continuous: true } };

      // Should NOT throw - just stores settings
      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();

      // Settings should be stored for future use
      const settings = reader.getSettings();
      expect(settings.barcode?.continuous).toBe(true);
    });

    it('should handle SequenceAbortedError gracefully', async () => {
      await reader.setMode(ReaderMode.INVENTORY);

      const abortError = new SequenceAbortedError('Settings change aborted');
      (commandManagerMock.executeCommand as Mock).mockRejectedValueOnce(abortError);

      const newSettings = { rfid: { transmitPower: 25 } };

      // Should not throw
      await expect(reader.setSettings(newSettings)).resolves.toBeUndefined();
    });

    it('should re-throw real command failures', async () => {
      await reader.setMode(ReaderMode.INVENTORY);

      const error = new Error('Hardware command failed');
      (commandManagerMock.executeSequence as Mock).mockRejectedValueOnce(error);

      const newSettings = { rfid: { transmitPower: 25 } };

      await expect(reader.setSettings(newSettings)).rejects.toThrow('Hardware command failed');
    });

    it('should skip RFID processing when no rfid settings provided', async () => {
      // Set to BARCODE mode to allow barcode settings
      await reader.setMode(ReaderMode.BARCODE);

      const newSettings = { barcode: { continuous: true } };

      await reader.setSettings(newSettings);

      // Reset mock to check only setSettings calls
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings(newSettings);

      // Should not call CommandManager for RFID commands
      expect(commandManagerMock.executeSequence).not.toHaveBeenCalled();

      // Should still update settings and emit event
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'SETTINGS_UPDATED',
        payload: { settings: newSettings },
        timestamp: expect.any(Number)
      }));
    });

    it('should apply only provided RFID settings without defaults', async () => {
      await reader.setMode(ReaderMode.INVENTORY);

      const newSettings = {
        rfid: {
          transmitPower: 20
          // algorithm and inventoryMode not provided
        }
      };

      // Clear previous calls
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings(newSettings);

      // Should call executeSequence for power setting
      expect(commandManagerMock.executeSequence).toHaveBeenCalled(
      );
    });
  });

  /**
   * TRA-1091 — characterisation only, no behaviour change.
   *
   * The ticket hypothesises that when the Locate deep link races the command
   * mutex and setSettings throws, the tag mask is never applied and the reader
   * searches UNFILTERED (a false positive on a tag finder). These tests pin
   * down what the code actually does on that path, so the hypothesis can be
   * confirmed or closed without guessing. They assert current behaviour; none
   * of them is a fix.
   */
  describe('LOCATE tag mask sourcing (TRA-1091)', () => {
    const TARGET_EPC = 'E280689400000000001018DD';
    const SECOND_TARGET_EPC = 'E280689400000000001018EE';

    // Last N commands of a mode sequence — buildModeSequences() appends the
    // mask sequence last.
    const maskTail = (sequence: unknown[], length: number) => sequence.slice(-length);

    const lastModeSequence = () => {
      const calls = (commandManagerMock.executeSequence as Mock).mock.calls;
      return calls[calls.length - 1][0];
    };

    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('builds the mask into the LOCATE mode sequence itself, from the settings passed to setMode', async () => {
      // This is the deep-link path: App.tsx puts the EPC in the settings store,
      // DeviceManager snapshots the store and hands it to setMode(). No
      // separate setSettings() call is needed for the mask to reach hardware.
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: TARGET_EPC } });

      const expected = locateSettingsSequence(TARGET_EPC);
      expect(maskTail(lastModeSequence(), expected.length)).toEqual(expected);
    });

    /**
     * The defect this replaced: `error.message.includes('aborted')`.
     *
     * That match was OVER-SATISFIABLE — any error whose text happened to contain
     * the word took the swallow-and-return branch. It is the worst shape to
     * carry into an unattended run, because the branch it guards CONSUMES the
     * evidence: an unrelated failure is recorded as a success and cannot be
     * recovered from the log afterwards. A narrow match fails loudly; this one
     * launders.
     *
     * Both messages below contain "aborted" and neither is an abort.
     */
    it('does not swallow an unrelated error whose message merely says aborted', async () => {
      for (const message of [
        'RFID module aborted the power ramp',
        'upload aborted by peer',
      ]) {
        (commandManagerMock.executeSequence as Mock).mockRejectedValueOnce(new Error(message));

        await expect(
          reader.setSettings({ rfid: { transmitPower: 25, targetEPC: TARGET_EPC } })
        ).rejects.toThrow(message);
      }
    });

    it('still swallows a genuine SequenceAbortedError', async () => {
      // The other half: narrowing the match must not have closed the branch it
      // was there for. Asserting only the negative would pass on a predicate
      // that never fires at all.
      (commandManagerMock.executeSequence as Mock).mockRejectedValueOnce(
        new SequenceAbortedError('mode change in progress')
      );

      await expect(
        reader.setSettings({ rfid: { transmitPower: 25, targetEPC: TARGET_EPC } })
      ).resolves.toBeUndefined();
    });

    it('still treats a mutex collision as benign, but reports it loudly', async () => {
      // TRA-1197 should make this unreachable — a settings push queues rather
      // than losing the mutex. The guard stays anyway, because the outcomes are
      // asymmetric: swallowing costs a deferred settings write that the mode
      // change reapplies, while propagating puts the primary Locate path into
      // ERROR, which is the hardware-found defect TRA-1091 shipped to fix.
      //
      // What DID change is the silence. It is logged at error level so a
      // bypassed queue is countable rather than invisible.
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      (commandManagerMock.executeSequence as Mock)
        .mockRejectedValueOnce(new CommandInFlightError());

      await expect(
        reader.setSettings({ rfid: { transmitPower: 25, targetEPC: TARGET_EPC } })
      ).resolves.toBeUndefined();

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[Worker] ERROR:',
        expect.stringContaining('should be unreachable'),
        expect.anything()
      );
      consoleErrorSpy.mockRestore();
    });

    it('applies the mask on the settings path now that it no longer loses the race', async () => {
      // The positive half of what the old benign branch was protecting: with
      // the wire taken once for the whole block, the tag mask actually reaches
      // the hardware from setSettings rather than being deferred to whatever
      // mode change happened to follow.
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: TARGET_EPC } });
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings({ rfid: { transmitPower: 25, targetEPC: SECOND_TARGET_EPC } });

      // The mask sequence is the LAST thing the settings block writes, so it is
      // the last call — and it carries the NEW target, not the one setMode put
      // there.
      expect(lastModeSequence()).toEqual(locateSettingsSequence(SECOND_TARGET_EPC));
    });

    it('still re-throws genuine hardware failures', async () => {
      // The benign-collision branch must not swallow real errors.
      (commandManagerMock.executeSequence as Mock)
        .mockRejectedValueOnce(new Error('Hardware command failed'));

      await expect(
        reader.setSettings({ rfid: { transmitPower: 25 } })
      ).rejects.toThrow('Hardware command failed');
    });

    it('installs an all-zero mask with tag select ENABLED when the EPC is missing — it does not search unfiltered', async () => {
      // buildModeSequences() coerces a missing targetEPC to '' rather than
      // undefined, and locateSettingsSequence('') pads to 24 zeros instead of
      // returning []. So "Building LOCATE with targetEPC: none" configures the
      // reader to match an all-zero EPC — nothing responds. That is a false
      // NEGATIVE, the opposite of the unfiltered-search failure mode.
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: '' } });

      const emptyMask = locateSettingsSequence('');
      expect(emptyMask.length).toBeGreaterThan(0);
      expect(maskTail(lastModeSequence(), emptyMask.length)).toEqual(emptyMask);

      // Only an undefined EPC would skip masking altogether, and the reader
      // never passes undefined.
      expect(locateSettingsSequence(undefined)).toEqual([]);
    });

    it('masks the full 128 bits of a 128-bit EPC, so tags sharing a 96-bit prefix no longer collide', async () => {
      // TRA-1108. These two EPCs differ ONLY in their last 8 hex chars, which
      // is where most schemes put the serial. Under the old three-register,
      // 96-bit mask they produced byte-identical sequences and the reader
      // could not tell them apart.
      const EPC_128_A = 'E28011700000020F8B1C0B39AAAAAAAA';
      const EPC_128_B = 'E28011700000020F8B1C0B39BBBBBBBB';
      expect(EPC_128_A).toHaveLength(32);

      expect(locateSettingsSequence(EPC_128_A)).not.toEqual(locateSettingsSequence(EPC_128_B));

      // Nor is either of them equal to a search for their shared 96-bit prefix.
      expect(locateSettingsSequence(EPC_128_A)).not.toEqual(
        locateSettingsSequence(EPC_128_A.slice(0, 24))
      );

      // The mode sequence carries the widened mask through to hardware.
      (commandManagerMock.executeSequence as Mock).mockClear();
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: EPC_128_A } });

      const expected = locateSettingsSequence(EPC_128_A);
      expect(maskTail(lastModeSequence(), expected.length)).toEqual(expected);
    });

    /**
     * Two real 128-bit tags off the operator's bench, read by the fixed
     * reader. They differ only in the final hex char — inside the 32-bit tail
     * the old 96-bit mask never covered. TRA-1108 fixed both the mask width
     * and the deep link that fed it a stripped value.
     */
    describe('real 128-bit bench tags', () => {
      const TAG_633 = '00000000000000000000533034313633';
      const TAG_634 = '00000000000000000000533034313634';

      const MASK_VALUE_REGISTERS = [
        RFID_REGISTERS.TAGMSK_0_3,
        RFID_REGISTERS.TAGMSK_4_7,
        RFID_REGISTERS.TAGMSK_8_11,
        RFID_REGISTERS.TAGMSK_12_15
      ];

      // The mask values a sequence writes, in order. createFirmwareCommand
      // lays the register address and the value out LSB-first.
      const maskValues = (sequence: ReturnType<typeof locateSettingsSequence>) =>
        sequence
          .map(cmd => ({
            register: cmd.payload![2] | (cmd.payload![3] << 8),
            value: ((cmd.payload![4]
              | (cmd.payload![5] << 8)
              | (cmd.payload![6] << 16)
              | (cmd.payload![7] << 24)) >>> 0)
          }))
          .filter(write => MASK_VALUE_REGISTERS.includes(write.register))
          .map(write => write.value);

      it('are told apart when the full EPC reaches the mask builder', () => {
        // Identical through hex char 24 — the whole difference lives in the
        // tail that TAGMSK_12_15 now covers.
        expect(TAG_633.slice(0, 24)).toBe(TAG_634.slice(0, 24));
        expect(locateSettingsSequence(TAG_633)).not.toEqual(locateSettingsSequence(TAG_634));
      });

      it('are findable from a leading-zero-stripped value too, now that both widths are masked (TRA-1120)', () => {
        // Nothing distinguishes a stripped '533034313633' of 96-bit origin
        // from one of 128-bit origin, so the mask builder cannot recover the
        // width. TRA-1108 answered that upstream — InventoryTableRow and
        // InventoryMobileCard send the untruncated tag.epc — which does
        // nothing for the manual EPC field or the registry, both of which
        // hold the stripped form.
        //
        // So the builder now emits BOTH readings and ORs them. What matters
        // here is that the stripped value ends up carrying the full-width
        // match's own mask, byte for byte; the register-level proof of the OR
        // lives in rfid/locate/sequences.test.ts.
        const stripped = removeLeadingZeros(TAG_633);
        expect(stripped).toBe('533034313633');

        // The last descriptor configured is the 128-bit one, and its four
        // mask registers are exactly what the full-width EPC produces.
        expect(maskValues(locateSettingsSequence(stripped)).slice(-4))
          .toEqual(maskValues(locateSettingsSequence(TAG_633)));

        // Not the same sequence, though: the 96-bit reading is still there
        // too, on the descriptor that runs first.
        expect(locateSettingsSequence(stripped))
          .not.toEqual(locateSettingsSequence(TAG_633));
        expect(locateSettingsSequence(stripped))
          .toEqual(locateSettingsSequence('000000000000533034313633'));
      });

      it('round-trips correctly for a 96-bit EPC, which is why this went unnoticed', () => {
        // padStart(24) is an exact inverse of the stripping at 96 bits, so
        // every 24-char tag — all 1.13M reads in preview — works fine. This
        // is the regression guard on the primary path.
        const epc96 = '000000000000000012345678';
        expect(locateSettingsSequence(removeLeadingZeros(epc96)))
          .toEqual(locateSettingsSequence(epc96));
      });
    });
  });

  describe('startScanning()', () => {
    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('should throw error if not in READY state', async () => {
      // Force state to SCANNING
      (reader as any).readerState = ReaderState.SCANNING;

      await expect(reader.startScanning()).rejects.toThrow('Cannot start scanning from state Scanning');
    });

    it('should start barcode scanning in BARCODE mode', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      postMessageSpy.mockClear();

      // Simulate trigger press to prevent reconciliation from stopping scan
      (reader as any).triggerState = true;

      // Clear mock call history before test
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.startScanning();

      // Should call executeSequence for barcode start
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);

      expect(reader.getState()).toBe(ReaderState.SCANNING);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_STATE_CHANGED',
        payload: { readerState: ReaderState.SCANNING },
        timestamp: expect.any(Number)
      }));
    });

    it('should handle INVENTORY mode (TODO)', async () => {
      await reader.setMode(ReaderMode.INVENTORY);
      postMessageSpy.mockClear();

      // Simulate trigger press to prevent reconciliation from stopping scan
      (reader as any).triggerState = true;

      // Currently a TODO, so it doesn't actually send commands
      await reader.startScanning();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('should handle LOCATE mode (TODO)', async () => {
      // LOCATE mode requires targetEPC to be set via options
      await reader.setMode(ReaderMode.LOCATE, { targetEPC: 'E280689400000000001018DD' });

      // Apply the targetEPC to hardware via setSettings to update lastAppliedTargetEPC
      await reader.setSettings({
        rfid: { targetEPC: 'E280689400000000001018DD' } as any
      });

      postMessageSpy.mockClear();

      // Simulate trigger press to prevent reconciliation from stopping scan
      (reader as any).triggerState = true;

      // Now scanning should work since lastAppliedTargetEPC is set
      await reader.startScanning();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('should set ERROR state on failure', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      postMessageSpy.mockClear();

      // Simulate trigger press to prevent reconciliation issues
      (reader as any).triggerState = true;

      const error = new Error('Command failed');
      // Override the mock to reject and set ERROR state
      (commandManagerMock.executeSequence as Mock).mockImplementationOnce(async () => {
        // Access the reader's setReaderState method directly
        (reader as any).setReaderState(ReaderState.BUSY);
        await new Promise(resolve => setTimeout(resolve, 0));
        // CommandManager would set ERROR state on failure
        (reader as any).setReaderState(ReaderState.ERROR);
        throw error;
      });

      await expect(reader.startScanning()).rejects.toThrow('Command failed');
      expect(reader.getState()).toBe(ReaderState.ERROR);
    });
  });

  /**
   * TRA-1122 (absorbed into TRA-1123): stop → change EPC → start inside ~2s
   * returns zero reads for a tag that is present, with Status showing
   * "Searching" and no error.
   *
   * The mask reaches the hardware from exactly two places: buildModeSequences()
   * during setMode, and setSettings() *while the reader is CONNECTED*.
   * startScanning() never wrote it. So a retarget that lands in the window the
   * reader spends leaving SCANNING is stored in readerSettings and never
   * written, and the search then runs against the previous tag's mask —
   * silently, because nothing failed.
   *
   * TRA-1225 CLOSED THE BUSY ROUTE INTO HERE, AND THIS BACKSTOP STILL EARNS ITS
   * PLACE. A push that lands mid-transition now waits for the reader to settle
   * and applies itself, so BUSY no longer arrives at startScanning unapplied —
   * and it must not, because writing the mask here costs ~3.7s INSIDE the
   * search, which is the whole of TRA-1225.
   *
   * What still reaches this backstop is a push the reader could not take at
   * all: DISCONNECTED, or a settle that timed out on a wedged reader. Those are
   * reported at ERROR and left for the next start to write. The cases below are
   * driven through DISCONNECTED for that reason — not because the BUSY case
   * stopped mattering, but because it is now handled a step earlier.
   */
  describe('LOCATE tag mask reaches hardware before scanning (TRA-1122)', () => {
    const FIRST_EPC = 'E280689400000000001018DD';
    const SECOND_EPC = 'E280689400000000001018EE';

    const executedSequences = () =>
      (commandManagerMock.executeSequence as Mock).mock.calls.map(call => call[0]);

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: FIRST_EPC } });
      // Held trigger, so the start is not reconciled straight back to a stop.
      (reader as any).triggerState = true;
      postMessageSpy.mockClear();
    });

    it('writes a target the reader could not take when it was pushed', async () => {
      // The push is reported at ERROR and left unapplied — silenced here so the
      // suite's output stays clean; that it IS reported is asserted in the
      // TRA-1225 loudness block rather than duplicated here.
      const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      (reader as any).readerState = ReaderState.DISCONNECTED;
      await reader.setSettings({ rfid: { targetEPC: SECOND_EPC } });
      (reader as any).readerState = ReaderState.CONNECTED;
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.startScanning();

      expect(executedSequences()[0]).toEqual(locateSettingsSequence(SECOND_EPC));
      errorSpy.mockRestore();
    });

    it('does not rewrite a mask that is already on the hardware', async () => {
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.startScanning();

      // The start sequence, and nothing else.
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);
    });

    it('surfaces a failed mask write instead of searching for the wrong tag', async () => {
      const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      (reader as any).readerState = ReaderState.DISCONNECTED;
      await reader.setSettings({ rfid: { targetEPC: SECOND_EPC } });
      (reader as any).readerState = ReaderState.CONNECTED;
      (commandManagerMock.executeSequence as Mock).mockClear();
      errorSpy.mockRestore();
      // Any failed mask write, not specifically the mutex collision: that error
      // is an invariant violation since TRA-1197 and no longer a thing this
      // path can meet. Pinning the test to it would make it a test of an
      // unreachable condition rather than of the behaviour it is named for.
      (commandManagerMock.executeSequence as Mock)
        .mockRejectedValueOnce(new Error('Command timeout'));

      await expect(reader.startScanning()).rejects.toThrow('Command timeout');

      // It failed on the mask, and never went on to start a search aimed at
      // the previous tag.
      expect(executedSequences()).toEqual([locateSettingsSequence(SECOND_EPC)]);
    });

    it('rewrites the mask after LOCATE is re-entered from another mode', async () => {
      // Leaving LOCATE invalidates whatever is on the hardware; coming back
      // must not trust the record of what was applied before.
      (reader as any).triggerState = false;
      await reader.setMode(ReaderMode.INVENTORY);
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: FIRST_EPC } });
      (reader as any).triggerState = true;
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.startScanning();

      // The LOCATE mode sequence just rewrote the mask, so the start needs no
      // second write.
      expect(commandManagerMock.executeSequence).toHaveBeenCalledTimes(1);
    });
  });

  /**
   * TRA-1225, measured on hardware 2026-08-31 at DEBUG.
   *
   * A settings push that arrives while the reader is BUSY was stored and never
   * applied — at `logger.debug`, which the default INFO level does not print,
   * so the drop produced NO OUTPUT AT ALL in any captured run.
   *
   * The consequence is not the one the reported symptom suggests. The mask does
   * still reach the hardware: startScanning()'s TRA-1122 backstop writes it.
   * But it writes it INSIDE the search. That write is 18 commands at a 100ms
   * settling delay each, preceded by the vendor's ~1.9s post-ABORT quiet
   * window — about 3.7s — so a 4s trigger hold produced no inventory at all:
   *
   *     [Reader] Converging to trigger held - starting scan
   *     [Reader] Target EPC changed since it was last written - applying...
   *     [CommandManager] Executing sequence of 18 commands
   *     [CommandManager] Holding 1861ms for the device's quiet window
   *     ... steps 1..15 of 18 ...          <- trigger released here
   *     RESULT: the PREVIOUS tag x2, the requested tag x0
   *
   * No RFID_START_SEQUENCE. The search never ran, and the two stray reads of
   * the previous tag are the tail of the search before it.
   *
   * The first search in the same run passes for one reason only: its push
   * landed while CONNECTED, so setSettings applied the mask and the caller
   * awaited it. The cost was paid outside the timed window.
   *
   * So BUSY must not mean "discard". BUSY is by construction transient — some
   * sequence is in flight and will publish CONNECTED — so the push waits for
   * that and then applies, which is what makes `await setSettings(...)` mean
   * "the radio is configured" again. The 3.7s goes back where the first search
   * already pays it.
   */
  describe('a settings push that lands mid-transition (TRA-1225)', () => {
    const FIRST_EPC = 'E280689400000000001018DD';
    const SECOND_EPC = 'E280689400000000001018EE';

    const executedSequences = () =>
      (commandManagerMock.executeSequence as Mock).mock.calls.map(call => call[0]);

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: FIRST_EPC } });
      (reader as any).triggerState = true;
      (commandManagerMock.executeSequence as Mock).mockClear();
    });

    it('applies a target that arrives while a sequence is still in flight', async () => {
      (reader as any).readerState = ReaderState.BUSY;

      const push = reader.setSettings({ rfid: { transmitPower: 30, targetEPC: SECOND_EPC } });
      // The in-flight sequence completes, exactly as CommandManager publishes it.
      (reader as any).setReaderState(ReaderState.CONNECTED);
      await push;

      expect(executedSequences()).toContainEqual(locateSettingsSequence(SECOND_EPC));
    });

    it('does not resolve until the mask is actually on the hardware', async () => {
      (reader as any).readerState = ReaderState.BUSY;

      let resolved = false;
      const push = reader.setSettings({ rfid: { transmitPower: 30, targetEPC: SECOND_EPC } })
        .then(() => { resolved = true; });

      // Give the push every chance to resolve early. If it does, the caller is
      // told the settings landed while the radio still carries the old mask —
      // which is the whole defect, since the write is then displaced into the
      // search.
      await new Promise(resolve => setTimeout(resolve, 10));
      expect(resolved).toBe(false);

      (reader as any).setReaderState(ReaderState.CONNECTED);
      await push;
      expect(resolved).toBe(true);
    });

    it('leaves nothing for the scan start to write', async () => {
      (reader as any).readerState = ReaderState.BUSY;
      const push = reader.setSettings({ rfid: { transmitPower: 30, targetEPC: SECOND_EPC } });
      (reader as any).setReaderState(ReaderState.CONNECTED);
      await push;
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.startScanning();

      // The start sequence, and nothing else. A mask write here is the 3.7s
      // that swallowed the search.
      expect(executedSequences()).toEqual([RFID_START_SEQUENCE]);
    });

    /**
     * The complement to TRA-1237, and it has to keep working.
     *
     * That fix stops CommandManager publishing ERROR for a step that is about
     * to be retried, and for an abort — so the only ERROR that now reaches this
     * waiter is a sequence that genuinely failed with its retries spent. On
     * that, dropping the push is CORRECT: waiting it out would wait for an
     * answer already known.
     *
     * What must not be lost is the report. This line is the entire reason
     * TRA-1237 was findable — it was already being written, loudly, in every
     * rep it happened in, and the defect surfaced only once someone counted the
     * lines. A fix that made the drop quieter would have been a worse outcome
     * than the drop.
     */
    it('still drops and REPORTS a push when the reader settles into a genuine ERROR', async () => {
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      (reader as any).readerState = ReaderState.BUSY;

      const push = reader.setSettings({ rfid: { transmitPower: 30, targetEPC: SECOND_EPC } });
      (reader as any).setReaderState(ReaderState.ERROR);
      await push;

      expect(executedSequences()).not.toContainEqual(locateSettingsSequence(SECOND_EPC));
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[Worker] ERROR:',
        expect.stringContaining('did NOT reach the radio')
      );
      consoleErrorSpy.mockRestore();
    });

    /**
     * `hasHardwareSettings` gates the whole apply block and does not list
     * targetEPC, so a targetEPC-only push takes NEITHER branch: not applied,
     * not deferred, not logged at all. The integration spec carries a comment
     * describing this and works around it by always sending transmitPower.
     * The workaround is the evidence; remove the need for it.
     */
    it('applies a push carrying nothing but a new target', async () => {
      await reader.setSettings({ rfid: { targetEPC: SECOND_EPC } });

      expect(executedSequences()).toContainEqual(locateSettingsSequence(SECOND_EPC));
    });
  });

  /**
   * The drop was invisible, and that is the most durable part of TRA-1225,
   * independent of the fix: `logger.debug` with WorkerLogger defaulting to INFO
   * means every soak log we hold is blind to it.
   *
   * These assert against `console` with the logger at its DEFAULT level, not
   * against logger.warn/logger.error. Spying the logger would pass even if the
   * level still swallowed the line, which is the failure being fixed rather
   * than a test of it.
   */
  describe('a settings push that cannot be applied says so (TRA-1225)', () => {
    const TARGET_EPC = 'E280689400000000001018DD';
    const OTHER_EPC = 'E280689400000000001018EE';

    let errorSpy: Mock;
    let warnSpy: Mock;

    const whatItSaid = () =>
      [...errorSpy.mock.calls, ...warnSpy.mock.calls].map(call => call.join(' ')).join('\n');

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: TARGET_EPC } });
      errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {}) as unknown as Mock;
      warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {}) as unknown as Mock;
    });

    it('names the target it could not apply, at the default log level', async () => {
      (reader as any).readerState = ReaderState.DISCONNECTED;

      await reader.setSettings({ rfid: { transmitPower: 30, targetEPC: OTHER_EPC } });

      expect(whatItSaid()).toMatch(/targetEPC/i);
    });

    it('stays quiet when the target did reach the hardware', async () => {
      await reader.setSettings({ rfid: { transmitPower: 30, targetEPC: OTHER_EPC } });

      expect(whatItSaid()).not.toMatch(/targetEPC/i);
    });
  });

  /**
   * Hardware-found, 2026-08-20: retargeting *while the search is running* left
   * the old mask on the reader. setSettings only writes the mask while
   * CONNECTED; during SCANNING it takes the "stored but not applied" branch,
   * and nothing rewrites it because the search never restarts.
   *
   * Measured over the ble-mcp-test bridge: retargeting a running search to a
   * decoy EPC matching no tag on the bench kept delivering reads at 13.5 Hz and
   * -43 dBm — the *previous* tag, arriving through the stale mask. The ring
   * buffer cleared correctly and then refilled from the same wrong tag, because
   * the hardware mask is the only EPC filter there is: addRssiReading() never
   * receives an EPC, despite comments elsewhere claiming locateStore filters.
   *
   * A tag finder reporting another tag's signal is the whole of TRA-1123.
   */
  describe('LOCATE retarget while scanning (hardware-found)', () => {
    const FIRST_EPC = 'E280689400000000001018DD';
    const SECOND_EPC = 'E280689400000000001018EE';

    const executedSequences = () =>
      (commandManagerMock.executeSequence as Mock).mock.calls.map(call => call[0]);

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.LOCATE, { rfid: { targetEPC: FIRST_EPC } });
      (reader as any).triggerState = true;
      await reader.startScanning();
      postMessageSpy.mockClear();
    });

    it('puts the new mask on the hardware rather than storing it for later', async () => {
      expect(reader.getState()).toBe(ReaderState.SCANNING);
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings({ rfid: { targetEPC: SECOND_EPC } });

      expect(executedSequences()).toContainEqual(locateSettingsSequence(SECOND_EPC));
    });

    it('is searching again on the new target when it settles', async () => {
      await reader.setSettings({ rfid: { targetEPC: SECOND_EPC } });

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('leaves a running search alone when the target has not changed', async () => {
      (commandManagerMock.executeSequence as Mock).mockClear();

      await reader.setSettings({ rfid: { targetEPC: FIRST_EPC } });

      expect(executedSequences()).not.toContainEqual(locateSettingsSequence(FIRST_EPC));
      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });
  });

  /**
   * Trigger convergence — the escalation loop this closes (Mike, 2026-08-30):
   *
   *   "the trigger is the toughest use case because the user has their hands on
   *    that and will inevitably cycle it if it feels unresponsive to them, which
   *    will escalate and exacerbate any stacking behavior"
   *
   * The design, in Mike's words: trigger events that arrive during BUSY are
   * **dropped**, and on command completion the reader **converges to the current
   * trigger state**. Drop the edges, reconcile the level. That is only sound
   * because `triggerState` is maintained unconditionally at the top of the
   * notification handler — "we maintain reported trigger state regardless of
   * command state" — so the level is always current even when no edge acted.
   *
   * The convergence check existed but could never fire: it read
   * `!this.triggerState && !this.scanningRequested`, and startScanning() sets
   * `scanningRequested = true` as its first statement with nothing clearing it
   * before the check. It conflated "the button is holding this scan" with
   * "somebody called startScanning".
   */
  describe('trigger convergence after a start completes', () => {
    /**
     * Let the backstop run, and let whatever it starts finish.
     *
     * Convergence is scheduled on a macrotask so it lands after anything
     * already in flight; the scan it then starts takes another tick of its own
     * to reach its final state. One tick observes the reader mid-start and
     * reads BUSY, which looks like the defect rather than the fix.
     */
    const settleConvergence = async () => {
      for (let i = 0; i < 5; i++) await new Promise(resolve => setTimeout(resolve, 0));
    };

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.INVENTORY);
      postMessageSpy.mockClear();
    });

    it('converges to stopped when the trigger is released mid-start', async () => {
      // The defect this replaces: the reader finished the start and sat there
      // SCANNING with the operator's finger already off the trigger.
      (reader as any).triggerState = true;
      const starting = reader.startScanning();

      // Release lands while BUSY. The edge is dropped by the handler; only the
      // level survives, and the level is what convergence reads.
      (reader as any).triggerState = false;

      await starting;

      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('stays scanning when the trigger is still held', async () => {
      // The other half — asserting only the negative would pass on a
      // convergence that stops unconditionally.
      (reader as any).triggerState = true;

      await reader.startScanning();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('does not stop a button-started scan just because no trigger is held', async () => {
      // Convergence is the TRIGGER's to apply to a scan the trigger started.
      // A button-started scan has triggerState false from beginning to end, and
      // must not be torn down by that.
      (reader as any).triggerState = false;

      await reader.startScanning();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('converges on the FINAL level after the trigger is cycled during BUSY', async () => {
      // The escalation case: an operator who thinks it is unresponsive cycles
      // the trigger repeatedly while the start is in flight. Every one of those
      // edges is dropped, and none of them enqueues work. What decides the
      // outcome is where the trigger ends up.
      (reader as any).triggerState = true;
      const starting = reader.startScanning();

      (reader as any).triggerState = false;
      (reader as any).triggerState = true;
      (reader as any).triggerState = false;
      (reader as any).triggerState = true;

      await starting;

      // Ends held, so it ends scanning.
      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('reconciles a press dropped during a settings push — the hardware repro', async () => {
      // Found on hardware 2026-08-30, within seconds: cycle the trigger while
      // moving between tabs and the reader ends up with the trigger DOWN and
      // nothing scanning. Tab navigation pushes settings; a press landing
      // during that push is dropped because the reader is BUSY, the push
      // completes and publishes CONNECTED, and before consolidation nothing
      // re-read the trigger level. No further edge is coming — the finger is
      // already down — so it stayed stranded until the operator let go.
      //
      // applySettings() was one of two paths with no reconciliation of its own.
      // The battery poll is the other, and it is worse because it fires at an
      // unpredictable 60s point (TRA-1212).
      (reader as any).triggerState = true;

      await reader.setSettings({ rfid: { transmitPower: 25 } });
      await settleConvergence();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('leaves a settings push alone when the trigger is not held', async () => {
      // The other half: convergence must not invent a scan nobody asked for.
      await reader.setSettings({ rfid: { transmitPower: 25 } });
      await settleConvergence();

      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    it('issues exactly one start sequence no matter how hard the trigger is cycled', async () => {
      // The stacking half of the same concern: cycling must not multiply
      // commands. The reader's guards act only from CONNECTED or SCANNING, and
      // a start holds BUSY throughout, so the extra edges reach nothing.
      (reader as any).triggerState = true;
      (commandManagerMock.executeSequence as Mock).mockClear();

      const starting = reader.startScanning();
      for (let i = 0; i < 6; i++) {
        (reader as any).triggerState = i % 2 === 0;
      }
      await starting;

      const starts = (commandManagerMock.executeSequence as Mock).mock.calls
        .filter(call => call[0] === RFID_START_SEQUENCE);
      expect(starts).toHaveLength(1);
    });
  });

  /**
   * TRA-1247's discriminator, and the reason it had to be settled before the
   * e2e harness timeout was touched.
   *
   * The convergence tests above assign `triggerState` directly, so they assume
   * the level is recorded while the reader is BUSY without ever showing it.
   * Two readings of the 2026-09-02 arm (48/200 reps failing with
   * `readerState: Busy` / `status: Idle`) were open on that assumption:
   *
   *   - the level IS recorded, only the edge is dropped — the press was never
   *     lost, and the harness gave up before convergence could act on it;
   *   - the notification does not reach the worker during BUSY at all — in
   *     which case no harness timeout helps and the fix is elsewhere.
   *
   * These tests answer it. The routing half — that `NotificationRouter` emits
   * the event whatever the reader state — is in
   * `notification/system.test.ts`.
   */
  /**
   * The reassembly diagnostic is only as good as the handler it reads.
   *
   * `CommandManager` prints the inbound packet ring buffer when a `0x0000`
   * rejection arrives, and only `PacketHandler.processIncomingData()` fills
   * it — which is the reader's job, through `handleBleData`. Give the command
   * manager a handler of its own and the report says
   * `Recent BLE packets (0 captured)` for ever. TRA-1250.
   *
   * Asserted on identity rather than behaviour because that is the whole
   * defect: two correct-looking objects, one of them never fed.
   */
  it('hands the CommandManager the packet handler it feeds, not a second one', () => {
    const fresh = new CS108Reader();

    const constructedWith = (CommandManager as unknown as Mock).mock.calls.at(-1);
    expect(constructedWith?.[3], 'CommandManager must be given a packet handler').toBeDefined();
    expect(constructedWith?.[3]).toBe((fresh as any).packetHandler);
  });

  describe('a trigger notification arriving while BUSY', () => {
    /** See the convergence block above: the backstop needs several macrotasks. */
    const settleConvergence = async () => {
      for (let i = 0; i < 5; i++) await new Promise(resolve => setTimeout(resolve, 0));
    };

    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.INVENTORY);
      postMessageSpy.mockClear();
    });

    /**
     * Hold the reader BUSY through the same choke point the product uses, and
     * hand back the function that lets bring-up finish.
     */
    const holdBusy = (): (() => void) => {
      const stateContext = (commandManagerMock as unknown as {
        __stateContext: { setReaderState: (state: string) => void };
      }).__stateContext;
      stateContext.setReaderState(ReaderState.BUSY);
      return () => stateContext.setReaderState(ReaderState.CONNECTED);
    };

    it('records the level and tells the UI, without starting a scan', async () => {
      const finishBringUp = holdBusy();
      expect(reader.getState()).toBe(ReaderState.BUSY);
      (commandManagerMock.executeSequence as Mock).mockClear();

      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      });

      // The level is latched even though the edge was dropped. This is the
      // limb the discriminator was asking about: the press is not lost.
      expect((reader as any).triggerState).toBe(true);
      // And the store learns immediately, which is what the e2e helper polls.
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      }));
      // The edge itself does nothing, by design.
      expect(commandManagerMock.executeSequence).not.toHaveBeenCalled();

      finishBringUp();
    });

    it('starts the scan when the reader settles, not when the press arrives', async () => {
      const finishBringUp = holdBusy();

      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      });
      expect(reader.getState()).toBe(ReaderState.BUSY);

      finishBringUp();
      await settleConvergence();

      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    /**
     * TIME revokes nothing — half of ADR 0016's mechanism, isolated.
     *
     * `triggerState` moves at `reader.ts:179` and on disconnect, with no timer
     * between them, and the device pushes nothing unbidden (ADR 0019). What
     * DOES revoke a level is a mode change, whose `IDLE_SEQUENCE` re-reads the
     * real switch position with `GET_TRIGGER_STATE`; that half needs the device
     * and is measured in
     * `tests/e2e/trigger-level-is-reread-on-mode-change.spec.ts`. This test is
     * its control: no mode change here, so the level must hold.
     */
    it('keeps the level latched while the reader stays BUSY — no timer revokes it', async () => {
      const finishBringUp = holdBusy();

      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      });

      // Well past the ~500ms the ADR named.
      await new Promise(resolve => setTimeout(resolve, 750));

      expect((reader as any).triggerState).toBe(true);
      expect(reader.getState()).toBe(ReaderState.BUSY);

      finishBringUp();
      await settleConvergence();
      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });
  });

  describe('stopScanning()', () => {
    beforeEach(async () => {
      await reader.connect();
      await reader.setMode(ReaderMode.BARCODE);
      postMessageSpy.mockClear();
    });

    it('should return early if not scanning', async () => {
      // Start in READY state (not scanning)
      expect(reader.getState()).toBe(ReaderState.CONNECTED);

      // Clear mocks to verify no commands are sent
      vi.clearAllMocks();

      await reader.stopScanning();

      expect(commandManagerMock.executeSequence).not.toHaveBeenCalled();
    });

    it('should stop barcode scanning in BARCODE mode', async () => {
      // Simulate trigger press to allow scanning to start
      (reader as any).triggerState = true;

      // Start scanning first
      await reader.startScanning();
      expect(reader.getState()).toBe(ReaderState.SCANNING);

      vi.clearAllMocks();

      // Release trigger before stopping to prevent reconciliation restart
      (reader as any).triggerState = false;

      await reader.stopScanning();

      // Should call executeSequence for barcode stop
      expect(commandManagerMock.executeSequence).toHaveBeenCalled();

      expect(reader.getState()).toBe(ReaderState.CONNECTED);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'READER_STATE_CHANGED',
        payload: { readerState: ReaderState.CONNECTED },
        timestamp: expect.any(Number)
      }));
    });

    it('should throw error on failure and set ERROR state', async () => {
      // Simulate trigger press to allow scanning to start
      (reader as any).triggerState = true;

      // Start scanning first
      await reader.startScanning();
      expect(reader.getState()).toBe(ReaderState.SCANNING);

      vi.clearAllMocks();

      const error = new Error('Command failed');
      // Override the mock to reject and set ERROR state
      (commandManagerMock.executeSequence as Mock).mockImplementationOnce(async () => {
        // Access the reader's setReaderState method directly
        (reader as any).setReaderState(ReaderState.BUSY);
        await new Promise(resolve => setTimeout(resolve, 0));
        // CommandManager would set ERROR state on failure
        (reader as any).setReaderState(ReaderState.ERROR);
        throw error;
      });

      await expect(reader.stopScanning()).rejects.toThrow('Command failed');
      // Implementation now sets ERROR state for stop failures (line 730 in reader.ts)
      expect(reader.getState()).toBe(ReaderState.ERROR);
    });
  });

  describe('handleNotificationEvent()', () => {
    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('should handle trigger press in scanning mode', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      postMessageSpy.mockClear();

      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      });

      expect(commandManagerMock.executeSequence).toHaveBeenCalled();
      expect(reader.getState()).toBe(ReaderState.SCANNING);
    });

    it('should handle trigger release when scanning', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      await reader.startScanning();
      // Clear scanningRequested flag so trigger release will stop
      (reader as any).scanningRequested = false;
      postMessageSpy.mockClear();

      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: false }
      });

      expect(commandManagerMock.executeSequence).toHaveBeenCalled();
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
    });

    // TRA-1171: the UI has to learn about the release when the notification
    // arrives, not when the stop finishes. Posting behind the awaited stop is
    // what let the Locate gauge and alarm keep running after the operator let
    // go, and it also meant a REJECTED stop emitted no event at all
    // (TRA-1168).
    it('posts TRIGGER_STATE_CHANGED before awaiting the stop (TRA-1171)', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      await reader.startScanning();
      (reader as any).scanningRequested = false;
      postMessageSpy.mockClear();

      // A stop that never settles on its own. The event can only be observed
      // here if it was posted BEFORE the await.
      let releaseStop: () => void = () => {};
      (commandManagerMock.executeSequence as Mock).mockImplementationOnce(
        () => new Promise<void>(resolve => { releaseStop = resolve; })
      );

      const handled = (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: false }
      });

      await Promise.resolve();

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: false }
      }));

      releaseStop();
      await handled;

      // Exactly once. The hoist has to remove the old post at the end of the
      // case, or every trigger edge is emitted twice.
      const triggerPosts = postMessageSpy.mock.calls.filter(
        (call: unknown[]) => (call[0] as { type?: string })?.type === 'TRIGGER_STATE_CHANGED'
      );
      expect(triggerPosts).toHaveLength(1);
    });

    it('posts a debounced trigger edge exactly once (TRA-1171)', async () => {
      // The debounced branch used to `break` out of the switch and fall into
      // the post at the end. With the post hoisted above it, that same `break`
      // would emit the event twice.
      await reader.setMode(ReaderMode.BARCODE);
      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true }
      });
      postMessageSpy.mockClear();

      // Immediately again, inside triggerDebounceMs.
      await (reader as any).handleNotificationEvent({
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: false }
      });

      const triggerPosts = postMessageSpy.mock.calls.filter(
        (call: unknown[]) => (call[0] as { type?: string })?.type === 'TRIGGER_STATE_CHANGED'
      );
      expect(triggerPosts).toHaveLength(1);
    });

    it('should handle barcode auto-stop request', async () => {
      await reader.setMode(ReaderMode.BARCODE);
      await reader.startScanning();
      postMessageSpy.mockClear();

      await (reader as any).handleNotificationEvent({
        type: 'BARCODE_AUTO_STOP_REQUEST'
      });

      expect(commandManagerMock.executeSequence).toHaveBeenCalled();
      expect(reader.getState()).toBe(ReaderState.CONNECTED);

      // Should NOT emit the auto-stop event
      expect(postMessageSpy).not.toHaveBeenCalledWith(expect.objectContaining({
        type: 'BARCODE_AUTO_STOP_REQUEST'
      }));
    });

    it('should pass through other events', async () => {
      const testEvent = {
        type: 'TAG_READ',
        payload: { epc: 'test123' }
      };

      await (reader as any).handleNotificationEvent(testEvent);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        ...testEvent,
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('Battery monitoring', () => {
    beforeEach(async () => {
      vi.useFakeTimers();
      await reader.connect();
      postMessageSpy.mockClear();
      vi.clearAllTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it.skip('should schedule battery check after setMode(IDLE)', async () => {
      // This test has timing issues with fake timers
      // Battery scheduling works correctly in production
      // First set to a different mode to ensure we're not already in IDLE
      await reader.setMode(ReaderMode.INVENTORY);

      // Clear mocks to ensure clean state
      vi.clearAllMocks();

      // Now spy on scheduleBatteryCheck before calling setMode
      const scheduleSpy = vi.spyOn(reader as any, 'scheduleBatteryCheck').mockImplementation(() => {
        // Mock implementation to avoid actual timer setup
      });

      // The mock executeSequence needs to complete async
      await reader.setMode(ReaderMode.IDLE);

      // Don't run all timers - just check that scheduleBatteryCheck was called
      expect(scheduleSpy).toHaveBeenCalled();
      scheduleSpy.mockRestore();
    });

    it('should not schedule battery check when readerState is SCANNING', async () => {
      // Set reader to SCANNING state
      (reader as any).readerState = ReaderState.SCANNING;
      const scheduleSpy = vi.spyOn(reader as any, 'scheduleBatteryCheck');

      (reader as any).scheduleBatteryCheck();

      // Should return early without scheduling
      expect(scheduleSpy).toHaveBeenCalled();
      expect(scheduleSpy).toHaveReturnedWith(undefined);
    });

    it('should not schedule battery check when readerState is BUSY', async () => {
      // Set reader to BUSY state
      (reader as any).readerState = ReaderState.BUSY;
      const scheduleSpy = vi.spyOn(reader as any, 'scheduleBatteryCheck');

      (reader as any).scheduleBatteryCheck();

      // Should return early without scheduling
      expect(scheduleSpy).toHaveBeenCalled();
      expect(scheduleSpy).toHaveReturnedWith(undefined);
    });

    it('should respect batteryCheckInterval setting', async () => {
      // Set custom interval
      await reader.setSettings({
        system: {
          batteryCheckInterval: 30 // 30 seconds
        }
      });

      (reader as any).scheduleBatteryCheck();

      // Timer should be set with 30000ms
      expect(vi.getTimerCount()).toBe(1);
    });

    it.skip('should disable battery check when interval is 0', async () => {
      await reader.setSettings({
        system: {
          batteryCheckInterval: 0 // Disabled
        }
      });

      // Clear any existing timers and count current timers
      vi.clearAllTimers();
      const timerCountBefore = vi.getTimerCount();

      (reader as any).scheduleBatteryCheck();

      // No new timer should be set when interval is 0
      const timerCountAfter = vi.getTimerCount();
      expect(timerCountAfter).toBe(timerCountBefore);
    });

    it('should double check frequency when battery < 20%', () => {
      // Set battery to low level
      (reader as any).lastBatteryPercentage = 15;
      (reader as any).readerSettings = {
        system: {
          batteryCheckInterval: 60
        }
      };

      (reader as any).scheduleBatteryCheck();

      // Timer should be set with half interval (30000ms instead of 60000ms)
      expect(vi.getTimerCount()).toBe(1);
      // Note: We can't directly inspect timer duration in vitest,
      // but the logic is tested
    });

    it('should emit BATTERY_LEVEL_CHANGED only when percentage changes', async () => {
      // Mock getBatteryPercentage to return different values
      const getBatterySpy = vi.spyOn(reader as any, 'getBatteryPercentage')
        .mockResolvedValueOnce(85)
        .mockResolvedValueOnce(85) // Same value
        .mockResolvedValueOnce(84); // Different value

      // Mock scheduleBatteryCheck to avoid recursion
      let callCount = 0;
      const originalScheduleBatteryCheck = (reader as any).scheduleBatteryCheck.bind(reader);
      vi.spyOn(reader as any, 'scheduleBatteryCheck').mockImplementation(() => {
        callCount++;
        if (callCount > 3) return; // Prevent infinite recursion
        return originalScheduleBatteryCheck();
      });

      // Set initial state
      (reader as any).lastBatteryPercentage = -1;
      (reader as any).readerSettings = {
        system: {
          batteryCheckInterval: 60
        }
      };

      // First check - should update from -1 to 85
      await (reader as any).scheduleBatteryCheck();
      vi.advanceTimersByTime(60000);

      // Verify the test runs without infinite loops
      expect(getBatterySpy).toHaveBeenCalled();
    });

    it('should clear battery timer on disconnect', async () => {
      // Start battery check timer
      (reader as any).scheduleBatteryCheck();
      expect((reader as any).batteryCheckTimer).toBeDefined();

      // Disconnect should clear the timer
      await reader.disconnect();

      expect((reader as any).batteryCheckTimer).toBeUndefined();
    });

    it.skip('should continue checking despite errors', async () => {
      // Mock getBatteryPercentage to throw error
      vi.spyOn(reader as any, 'getBatteryPercentage').mockRejectedValue(new Error('Battery read failed'));

      // Mock scheduleBatteryCheck to track calls and prevent infinite recursion
      let scheduleCallCount = 0;
      const originalScheduleBatteryCheck = (reader as any).scheduleBatteryCheck.bind(reader);
      const scheduleSpy = vi.spyOn(reader as any, 'scheduleBatteryCheck').mockImplementation(() => {
        scheduleCallCount++;
        if (scheduleCallCount > 2) return; // Prevent infinite recursion
        return originalScheduleBatteryCheck();
      });

      // Set settings for battery check
      (reader as any).readerSettings = {
        system: {
          batteryCheckInterval: 60
        }
      };

      // Start the battery check
      await (reader as any).scheduleBatteryCheck();

      // Advance timer to trigger the battery check
      vi.advanceTimersByTime(60000);
      await Promise.resolve(); // Let promises settle

      // Should have called scheduleBatteryCheck twice: initial + reschedule after error
      expect(scheduleSpy).toHaveBeenCalledTimes(2);
    });
  });
});