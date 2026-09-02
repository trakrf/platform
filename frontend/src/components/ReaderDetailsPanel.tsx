/**
 * What the connected reader actually is.
 *
 * The values come off the device itself — the Bluetooth board's two firmware
 * versions and its serial number, plus the RFID processor's firmware and its
 * own error register. Until this shipped, nothing we produced recorded any of
 * them: a bug report, a soak capture and a support conversation all had to say
 * "a CS108" and stop there.
 *
 * ⚠ **`Unknown` is a real answer here, and it is not the same as blank.** A
 * value the reader did not give up must not render as an empty cell, because an
 * empty cell reads as "it does not have one" when the truth is "it did not
 * answer". Refs TRA-1232.
 */

import { Cpu } from 'lucide-react';
import type { ReaderDetails } from '@/worker/types/reader';

interface ReaderDetailsPanelProps {
  details: ReaderDetails | null;
}

/** The value, or the fact that we never got one. Never a gap. */
function Field({ label, value }: { label: string; value?: string }) {
  return (
    <div>
      <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{label}</h4>
      <p className={`text-sm font-mono ${
        value ? 'text-gray-600 dark:text-gray-400' : 'text-gray-400 dark:text-gray-500 italic'
      }`}>
        {value ?? 'Unknown'}
      </p>
    </div>
  );
}

export function ReaderDetailsPanel({ details }: ReaderDetailsPanelProps) {
  // Nothing read yet means nothing to be wrong about. Five Unknowns beside a
  // disconnected reader is noise, not information.
  if (!details) return null;

  return (
    <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
      <div className="flex items-center mb-3">
        <Cpu className="w-4 h-4 text-gray-500 dark:text-gray-400 mr-2" />
        <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">Reader Details</h3>
      </div>

      <div className="grid grid-cols-2 gap-x-6 gap-y-3">
        <Field label="Serial Number" value={details.serialNumber} />
        {/*
          The most useful of the three versions: this image is shared across the
          CS108, the CS463 and the CS203X, so it is the number that says whether
          a fixed-reader customer is exposed to the same device behaviour.
        */}
        <Field label="RFID Firmware" value={details.rfidFirmware} />
        <Field label="Bluetooth Firmware" value={details.bluetoothFirmware} />
        <Field label="Silicon Labs Firmware" value={details.siliconLabsFirmware} />
        {/*
          Hex, because Appendix B's table of these codes is in hex — and zero
          is a VALUE (the radio is healthy), so it has to be distinguishable
          from never having been read.
        */}
        <Field
          label="RFID Error Code"
          value={details.macError === undefined
            ? undefined
            : `0x${details.macError.toString(16).padStart(4, '0')}`}
        />
      </div>
    </div>
  );
}
