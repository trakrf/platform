import { useEffect, useRef } from 'react';
import { useBarcodeStore } from '@/stores';
import useSound from 'use-sound';
import beepSound from '@/assets/sounds/beep.wav';

export function useBarcodeAudio() {
  const [playBeep] = useSound(beepSound, {
    volume: 0.7,
    interrupt: true
  });

  const barcodes = useBarcodeStore((state) => state.barcodes);
  const previousCountRef = useRef(barcodes.length);

  useEffect(() => {
    // Play beep when a new barcode is added
    if (barcodes.length > previousCountRef.current) {
      playBeep();
    }

    previousCountRef.current = barcodes.length;
  }, [barcodes.length, playBeep]);
}