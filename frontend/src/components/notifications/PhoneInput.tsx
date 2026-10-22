import type { PhoneErrors, PhoneParts } from '@/lib/notifications/validators';

interface PhoneInputProps {
  /** Prefix for ids and test ids: `${id}-code`, `${id}-number`. */
  id: string;
  value: PhoneParts;
  onChange: (value: PhoneParts) => void;
  onBlur?: () => void;
  errors?: PhoneErrors;
  disabled?: boolean;
}

const inputClass = (hasError: boolean) =>
  `block px-3 py-2 border rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 ${
    hasError
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  }`;
const LABEL = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2';
const FIELD_ERROR = 'mt-1 text-sm text-red-600 dark:text-red-400';

// "44" reads as "+44" so the country code always shows its plus sign.
const withPlus = (code: string) => (code && !code.startsWith('+') ? `+${code}` : code);

/**
 * Phone number entered as a country code plus the rest, side by side at every
 * width. Errors show under the part that is wrong; the hint shows only when
 * there is no error.
 */
export function PhoneInput({ id, value, onChange, onBlur, errors = {}, disabled }: PhoneInputProps) {
  const messages = [errors.phoneCode, errors.phoneNumber].filter(Boolean);

  return (
    // min-w-0: a fieldset is min-content wide by default and would overflow a
    // narrow grid cell.
    <fieldset
      className="min-w-0"
      onBlur={(e) => {
        // Moving between the two parts is not leaving the field.
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) onBlur?.();
      }}
    >
      <legend className={LABEL}>Phone</legend>
      <div className="flex gap-2">
        <input
          id={`${id}-code`}
          data-testid={`${id}-code`}
          type="tel"
          inputMode="tel"
          autoComplete="tel-country-code"
          aria-label="Country code"
          aria-invalid={!!errors.phoneCode}
          maxLength={4}
          placeholder="+1"
          value={value.code}
          disabled={disabled}
          onChange={(e) => onChange({ ...value, code: withPlus(e.target.value.trim()) })}
          className={`w-20 shrink-0 ${inputClass(!!errors.phoneCode)}`}
        />
        <input
          id={`${id}-number`}
          data-testid={`${id}-number`}
          type="tel"
          inputMode="tel"
          autoComplete="tel-national"
          aria-label="Phone number"
          aria-invalid={!!errors.phoneNumber}
          placeholder="555 123 4567"
          value={value.number}
          disabled={disabled}
          onChange={(e) => onChange({ ...value, number: e.target.value })}
          className={`min-w-0 flex-1 ${inputClass(!!errors.phoneNumber)}`}
        />
      </div>
      {messages.length > 0 ? (
        messages.map((message) => (
          <p key={message} className={FIELD_ERROR}>
            {message}
          </p>
        ))
      ) : (
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Country code, then the number without the leading 0
        </p>
      )}
    </fieldset>
  );
}
