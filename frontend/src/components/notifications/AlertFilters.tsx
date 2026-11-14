import { useId, useState } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import type { Asset } from '@/types/assets';
import type { NotificationAlertFilters, NotificationRecipient } from '@/types/notifications';

export type AlertFilterValues = Omit<NotificationAlertFilters, 'page' | 'per_page'>;

interface AlertFiltersProps {
  value: AlertFilterValues;
  onChange: (next: AlertFilterValues) => void;
  assets: Pick<Asset, 'id' | 'name'>[];
  recipients: Pick<NotificationRecipient, 'id' | 'name'>[];
}

const FILTER_KEYS = ['asset_id', 'type', 'recipient_id', 'channel', 'from', 'to'] as const;

export function countActiveFilters(value: AlertFilterValues): number {
  return FILTER_KEYS.filter((key) => value[key] !== undefined && value[key] !== '').length;
}

const LABEL_CLASS = 'text-sm font-medium text-gray-700 dark:text-gray-300';
const CONTROL_CLASS =
  'block w-full md:w-auto xl:w-full xl:max-w-none xl:text-sm py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 focus:border-transparent';
const SELECT_CLASS = `${CONTROL_CLASS} pl-3 pr-10`;
const DATE_CLASS = `${CONTROL_CLASS} px-3`;

const byName = <T extends { name: string }>(items: T[]) =>
  [...items].sort((a, b) => a.name.localeCompare(b.name));

const toNumber = (raw: string) => (raw === '' ? undefined : Number(raw));

export function AlertFilters({ value, onChange, assets, recipients }: AlertFiltersProps) {
  const id = useId();
  const [open, setOpen] = useState(false);
  // A To date before From is shown with an error but never applied.
  const [rejectedTo, setRejectedTo] = useState<string | null>(null);

  const toError = rejectedTo !== null && !!value.from && rejectedTo < value.from;
  const activeCount = countActiveFilters(value);
  const showClear = activeCount > 0 || toError;

  const set = <K extends keyof AlertFilterValues>(key: K, next: AlertFilterValues[K]) => {
    const updated: AlertFilterValues = { ...value, [key]: next };
    if (next === undefined || next === '') delete updated[key];
    return updated;
  };

  const handleFrom = (raw: string) => {
    const from = raw || undefined;
    const pendingTo = toError ? rejectedTo : (value.to ?? null);
    setRejectedTo(null);
    const updated = set('from', from);
    if (pendingTo && from && pendingTo < from) {
      setRejectedTo(pendingTo);
      delete updated.to;
    } else if (pendingTo) {
      updated.to = pendingTo;
    }
    onChange(updated);
  };

  const handleTo = (raw: string) => {
    if (raw && value.from && raw < value.from) {
      setRejectedTo(raw);
      onChange(set('to', undefined));
      return;
    }
    setRejectedTo(null);
    onChange(set('to', raw || undefined));
  };

  const handleClear = () => {
    setRejectedTo(null);
    onChange({});
  };

  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-controls={`${id}-panel`}
        data-testid="alert-filters-toggle"
        className="md:hidden inline-flex items-center gap-2 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
      >
        <SlidersHorizontal className="h-4 w-4" aria-hidden="true" />
        Filters
        {activeCount > 0 && (
          <span className="inline-flex items-center justify-center px-2 py-0.5 text-xs font-bold text-blue-700 bg-blue-100 dark:text-blue-300 dark:bg-blue-900/40 rounded-full">
            {activeCount}
          </span>
        )}
      </button>

      <div
        id={`${id}-panel`}
        data-testid="alert-filters-panel"
        className={`${open ? 'flex' : 'hidden'} md:flex flex-col md:flex-row md:flex-wrap md:items-start xl:grid xl:grid-cols-6 gap-3 mt-3 md:mt-0`}
      >
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-asset`} className={LABEL_CLASS}>
            Asset
          </label>
          <select
            id={`${id}-asset`}
            data-testid="alert-filter-asset"
            value={value.asset_id ?? ''}
            onChange={(e) => onChange(set('asset_id', toNumber(e.target.value)))}
            className={`${SELECT_CLASS} md:max-w-[14rem]`}
          >
            <option value="">All assets</option>
            {byName(assets).map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-type`} className={LABEL_CLASS}>
            Type
          </label>
          <select
            id={`${id}-type`}
            data-testid="alert-filter-type"
            value={value.type ?? ''}
            onChange={(e) =>
              onChange(set('type', (e.target.value || undefined) as AlertFilterValues['type']))
            }
            className={SELECT_CLASS}
          >
            <option value="">All types</option>
            <option value="alarm">Alarm</option>
            <option value="location_change">Location change</option>
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-subscriber`} className={LABEL_CLASS}>
            Subscriber
          </label>
          <select
            id={`${id}-subscriber`}
            data-testid="alert-filter-subscriber"
            value={value.recipient_id ?? ''}
            onChange={(e) => onChange(set('recipient_id', toNumber(e.target.value)))}
            className={`${SELECT_CLASS} md:max-w-[14rem]`}
          >
            <option value="">All subscribers</option>
            {byName(recipients).map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-channel`} className={LABEL_CLASS}>
            Channel
          </label>
          <select
            id={`${id}-channel`}
            data-testid="alert-filter-channel"
            value={value.channel ?? ''}
            onChange={(e) =>
              onChange(
                set('channel', (e.target.value || undefined) as AlertFilterValues['channel'])
              )
            }
            className={SELECT_CLASS}
          >
            <option value="">All channels</option>
            <option value="email">Email</option>
            <option value="sms">SMS</option>
          </select>
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-from`} className={LABEL_CLASS}>
            From
          </label>
          <input
            type="date"
            id={`${id}-from`}
            data-testid="alert-filter-from"
            value={value.from ?? ''}
            onChange={(e) => handleFrom(e.target.value)}
            className={DATE_CLASS}
          />
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-to`} className={LABEL_CLASS}>
            To
          </label>
          <input
            type="date"
            id={`${id}-to`}
            data-testid="alert-filter-to"
            value={toError ? (rejectedTo ?? '') : (value.to ?? '')}
            min={value.from}
            onChange={(e) => handleTo(e.target.value)}
            aria-invalid={toError}
            aria-describedby={toError ? `${id}-to-error` : undefined}
            className={`${DATE_CLASS} ${toError ? 'border-red-500 dark:border-red-500' : ''}`}
          />
          {toError && (
            <p
              id={`${id}-to-error`}
              data-testid="alert-filter-to-error"
              className="text-xs text-red-600 dark:text-red-400"
            >
              To must be on or after From.
            </p>
          )}
        </div>

        {showClear && (
          <button
            type="button"
            onClick={handleClear}
            data-testid="alert-filters-clear"
            className="self-start justify-self-start md:mt-6 xl:mt-0 xl:col-span-6 py-2.5 xl:py-0 text-sm text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 font-medium"
          >
            Clear filters
          </button>
        )}
      </div>
    </div>
  );
}
