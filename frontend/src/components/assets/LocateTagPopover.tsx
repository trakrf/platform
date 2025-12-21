import { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Target, Radio } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';

interface LocateTagPopoverProps {
  identifiers: TagIdentifier[];
  assetIdentifier: string;
  isActive: boolean;
  triggerClassName?: string;
  variant?: 'icon' | 'button';
}

export function LocateTagPopover({
  identifiers,
  assetIdentifier,
  isActive,
  triggerClassName = '',
  variant = 'icon',
}: LocateTagPopoverProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

  const activeIdentifiers = identifiers.filter((i) => i.is_active);
  const hasActiveIdentifiers = activeIdentifiers.length > 0;
  const canLocate = isActive && hasActiveIdentifiers;

  useEffect(() => {
    if (isOpen && triggerRef.current) {
      const rect = triggerRef.current.getBoundingClientRect();
      const popoverWidth = 280;
      const popoverHeight = Math.min(activeIdentifiers.length * 48 + 60, 300);

      let left = rect.left + rect.width / 2 - popoverWidth / 2;
      let top = rect.bottom + 8;

      if (left < 8) left = 8;
      if (left + popoverWidth > window.innerWidth - 8) {
        left = window.innerWidth - popoverWidth - 8;
      }
      if (top + popoverHeight > window.innerHeight - 8) {
        top = rect.top - popoverHeight - 8;
      }

      setPosition({ top, left });
    }
  }, [isOpen, activeIdentifiers.length]);

  useEffect(() => {
    if (!isOpen) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (
        popoverRef.current &&
        !popoverRef.current.contains(e.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setIsOpen(false);
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen]);

  const handleTriggerClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (canLocate) {
      setIsOpen(!isOpen);
    }
  };

  const handleLocateTag = (tagValue: string) => {
    window.location.hash = `#locate?epc=${encodeURIComponent(tagValue)}`;
    setIsOpen(false);
  };

  const baseButtonClass = variant === 'icon'
    ? `p-1 sm:p-1.5 rounded transition-colors ${
        canLocate
          ? 'text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20'
          : 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
      }`
    : `flex-1 flex items-center justify-center gap-1 sm:gap-2 px-2 sm:px-3 py-1.5 sm:py-2 text-xs sm:text-sm font-medium rounded-lg transition-colors border ${
        canLocate
          ? 'text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border-blue-200 dark:border-blue-800'
          : 'text-gray-400 bg-gray-100 dark:text-gray-500 dark:bg-gray-800 border-gray-200 dark:border-gray-700 cursor-not-allowed'
      }`;

  return (
    <>
      <button
        ref={triggerRef}
        onClick={handleTriggerClick}
        disabled={!canLocate}
        data-testid="locate-button"
        className={`${baseButtonClass} ${triggerClassName}`}
        aria-label={`Locate ${assetIdentifier}`}
        aria-expanded={isOpen}
        aria-haspopup="true"
      >
        <Target className={variant === 'icon' ? 'h-4 w-4' : 'h-3.5 w-3.5 sm:h-4 sm:w-4'} />
        {variant === 'button' && <span className="hidden xs:inline">Locate</span>}
      </button>

      {isOpen &&
        createPortal(
          <>
            <div
              className="fixed inset-0 z-50 sm:hidden"
              onClick={() => setIsOpen(false)}
            />
            <div
              ref={popoverRef}
              className="fixed z-50 bg-white dark:bg-gray-800 rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 animate-fadeIn"
              style={{
                top: position.top,
                left: position.left,
                width: 280,
                maxHeight: 300,
              }}
            >
              <div className="px-3 py-2 border-b border-gray-200 dark:border-gray-700">
                <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                  Select tag to locate
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                  {activeIdentifiers.length} active tag{activeIdentifiers.length !== 1 ? 's' : ''}
                </p>
              </div>
              <div className="max-h-[200px] overflow-y-auto">
                {activeIdentifiers.map((identifier) => (
                  <button
                    key={identifier.id}
                    onClick={() => handleLocateTag(identifier.value)}
                    className="w-full flex items-center gap-2 px-3 py-2.5 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors text-left border-b border-gray-100 dark:border-gray-700 last:border-b-0"
                  >
                    <Radio className="h-4 w-4 text-blue-600 dark:text-blue-400 flex-shrink-0" />
                    <span className="flex-1 text-sm font-mono text-gray-900 dark:text-gray-100 truncate">
                      {identifier.value}
                    </span>
                    <Target className="h-4 w-4 text-blue-600 dark:text-blue-400 flex-shrink-0" />
                  </button>
                ))}
              </div>
            </div>
          </>,
          document.body
        )}
    </>
  );
}
