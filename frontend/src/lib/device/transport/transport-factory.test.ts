import { describe, it, expect, afterEach } from 'vitest';
import { TransportFactory } from './transport-factory';
import { CS108BLETransport } from './cs108-ble-transport';

/**
 * TRA-1177 §4/§5.
 *
 * The factory used to present four modes ('auto' | 'ble' | 'bridge' | 'mock')
 * of which exactly one was reachable, and its fallback returned a MockTransport
 * that fabricates an 85% battery level and three EPCs at 100ms intervals.
 *
 * That was not test-only scaffolding. ScanControls.tsx calls connect() with no
 * browser-support gate, and DeviceManager.create performs no Bluetooth
 * precheck, so a user on Safari, Firefox or any iOS browser reached it and got
 * a resolved connection to fabricated hardware. Absence of the API is now an
 * error, never a quiet substitution.
 *
 * Safe for e2e: the ble-mcp-test bundle assigns navigator.bluetooth, so the
 * mocked path takes the same branch real hardware does.
 */

const originalBluetooth = Object.getOwnPropertyDescriptor(navigator, 'bluetooth');

function setWebBluetooth(present: boolean) {
  if (present) {
    Object.defineProperty(navigator, 'bluetooth', {
      value: {},
      configurable: true,
      writable: true,
    });
  } else {
    delete (navigator as unknown as Record<string, unknown>).bluetooth;
  }
}

afterEach(() => {
  delete (navigator as unknown as Record<string, unknown>).bluetooth;
  if (originalBluetooth) {
    Object.defineProperty(navigator, 'bluetooth', originalBluetooth);
  }
});

describe('TransportFactory.create', () => {
  it('returns a CS108BLETransport when Web Bluetooth is present', () => {
    setWebBluetooth(true);

    expect(TransportFactory.create({})).toBeInstanceOf(CS108BLETransport);
  });

  it('throws rather than substituting a mock when Web Bluetooth is absent', () => {
    setWebBluetooth(false);

    expect(() => TransportFactory.create({})).toThrow(/Web Bluetooth/i);
  });

  it('never hands back a transport that fabricates device data', () => {
    setWebBluetooth(false);

    let result: unknown;
    try {
      result = TransportFactory.create({});
    } catch {
      result = undefined;
    }

    // The old behaviour returned a MockTransport here, which reported a
    // battery level and streamed invented EPCs to a real user.
    expect(result).toBeUndefined();
  });
});
