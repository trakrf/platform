import { useEffect, useRef, useState, type ReactNode, type KeyboardEvent } from 'react';

type Variant = 'text' | 'number' | 'select' | 'toggle';

interface Option {
  value: string;
  label: string;
}

interface InlineEditCellProps<T extends string | number | boolean> {
  /** The persisted value. */
  value: T;
  /**
   * Persist a change. Receives the raw editor value: a string for
   * text/number/select, a boolean for toggle. Should reject on failure so the
   * cell can revert and surface the error inline.
   */
  onSave: (next: T extends boolean ? boolean : string) => Promise<void>;
  variant: Variant;
  /** Options for the select variant. */
  options?: Option[];
  /** Return an error message for an invalid raw string, or null when valid. */
  validate?: (raw: string) => string | null;
  /** Read-mode rendering of the value. Defaults to the stringified value. */
  display?: (value: T) => ReactNode;
  /** Accessible label for the read-mode trigger / control. */
  ariaLabel: string;
  placeholder?: string;
}

/**
 * Click-to-edit table cell (TRA-940). Read mode shows the value as a button;
 * activating it swaps in an inline editor that commits on blur/Enter and cancels
 * on Escape. Saves are optimistic — the new value shows immediately and reverts
 * if `onSave` rejects, with the error rendered inline (no toast). The cascade
 * cluster (type/transport/target) stays in the row expander; only independent
 * fields use this cell.
 */
export function InlineEditCell<T extends string | number | boolean>({
  value,
  onSave,
  variant,
  options,
  validate,
  display,
  ariaLabel,
  placeholder,
}: InlineEditCellProps<T>) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState<string | null>(null);
  // Value shown optimistically while a save is in flight; null = use prop.
  const [optimistic, setOptimistic] = useState<T | null>(null);
  const inputRef = useRef<HTMLInputElement | HTMLSelectElement | null>(null);

  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  const shown = optimistic !== null ? optimistic : value;

  const renderRead = () => (display ? display(shown) : String(shown));

  const errorMessage = (e: unknown) =>
    e instanceof Error && e.message ? e.message : 'Save failed';

  // commitString handles text/number/select: validate, optimistic-apply, save,
  // revert on failure.
  const commitString = async (raw: string) => {
    if (raw === String(value)) {
      setEditing(false);
      setError(null);
      return;
    }
    const validationError = validate?.(raw) ?? null;
    if (validationError) {
      setError(validationError);
      return;
    }
    setEditing(false);
    setError(null);
    setOptimistic(raw as T);
    try {
      await onSave(raw as T extends boolean ? boolean : string);
      setOptimistic(null);
    } catch (e) {
      setOptimistic(null);
      setError(errorMessage(e));
    }
  };

  const toggle = async (next: boolean) => {
    setError(null);
    setOptimistic(next as T);
    try {
      await onSave(next as T extends boolean ? boolean : string);
      setOptimistic(null);
    } catch (e) {
      setOptimistic(null);
      setError(errorMessage(e));
    }
  };

  if (variant === 'toggle') {
    const checked = Boolean(shown);
    return (
      <span className="inline-flex flex-col">
        <input
          type="checkbox"
          aria-label={ariaLabel}
          checked={checked}
          onChange={(e) => toggle(e.target.checked)}
          className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500"
        />
        {error && <span className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</span>}
      </span>
    );
  }

  if (!editing) {
    return (
      <span className="inline-flex flex-col">
        <button
          type="button"
          aria-label={ariaLabel}
          onClick={() => {
            setDraft(String(value));
            setError(null);
            setEditing(true);
          }}
          className="text-left rounded px-1 -mx-1 hover:bg-gray-100 dark:hover:bg-gray-700/60 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          {renderRead()}
        </button>
        {error && <span className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</span>}
      </span>
    );
  }

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void commitString(draft);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setEditing(false);
      setError(null);
    }
  };

  const fieldClass = `block w-full px-1 py-0.5 text-sm border rounded ${
    error
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2`;

  return (
    <span className="inline-flex flex-col min-w-[6rem]">
      {variant === 'select' ? (
        <select
          ref={(el) => (inputRef.current = el)}
          aria-label={ariaLabel}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            void commitString(e.target.value);
          }}
          onBlur={() => commitString(draft)}
          className={fieldClass}
        >
          {options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : (
        <input
          ref={(el) => (inputRef.current = el)}
          type={variant === 'number' ? 'number' : 'text'}
          aria-label={ariaLabel}
          value={draft}
          placeholder={placeholder}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKeyDown}
          onBlur={() => commitString(draft)}
          className={fieldClass}
        />
      )}
      {error && <span className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</span>}
    </span>
  );
}
