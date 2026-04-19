import { useState } from 'react';

type Preset = 'never' | '30d' | '90d' | '1y' | 'custom';

interface Props {
  value: string | null; // ISO 8601 or null
  onChange: (next: string | null) => void;
}

function daysFromNow(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString();
}

export function ExpirySelector({ value, onChange }: Props) {
  const [preset, setPreset] = useState<Preset>(value ? 'custom' : 'never');
  const [customDate, setCustomDate] = useState(value ?? '');

  const pick = (p: Preset) => {
    setPreset(p);
    switch (p) {
      case 'never':
        onChange(null);
        break;
      case '30d':
        onChange(daysFromNow(30));
        break;
      case '90d':
        onChange(daysFromNow(90));
        break;
      case '1y':
        onChange(daysFromNow(365));
        break;
      case 'custom':
        onChange(customDate || null);
        break;
    }
  };

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Expiration
      </legend>
      {(
        [
          ['never', 'Never'],
          ['30d', '30 days'],
          ['90d', '90 days'],
          ['1y', '1 year'],
          ['custom', 'Custom date'],
        ] as const
      ).map(([key, label]) => (
        <label key={key} className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name="expiry"
            value={key}
            checked={preset === key}
            onChange={() => pick(key)}
          />
          {label}
        </label>
      ))}
      {preset === 'custom' && (
        <div className="mt-2">
          <label htmlFor="custom-date" className="block text-xs mb-1">
            Expiry date
          </label>
          <input
            id="custom-date"
            type="date"
            aria-label="Expiry date"
            value={customDate.slice(0, 10)}
            onChange={(e) => {
              const iso = new Date(e.target.value).toISOString();
              setCustomDate(iso);
              onChange(iso);
            }}
            className="border rounded px-2 py-1 text-sm bg-white dark:bg-gray-800"
          />
        </div>
      )}
    </fieldset>
  );
}
