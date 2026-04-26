import { useEffect } from 'react';

export function useEscapeToClose(
  isOpen: boolean,
  onClose: () => void,
  disabled = false,
) {
  useEffect(() => {
    if (!isOpen || disabled) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [isOpen, onClose, disabled]);
}
