import { useMemo, useState, type FormEvent } from 'react';
import toast from 'react-hot-toast';
import {
  useAssetSubscriptionMutations,
  useNotificationRecipientMutations,
  useNotificationRecipients,
} from '@/hooks/notifications';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import {
  joinPhone,
  validateContactForm,
  type ContactErrors,
  type ContactFormInput,
} from '@/lib/notifications/validators';
import { PaidGate } from '@/components/entitlement';
import { PhoneInput } from './PhoneInput';
import type { NotificationChannel } from '@/types/notifications';

const CHANNELS: NotificationChannel[] = ['email', 'sms'];
const CHANNEL_LABELS: Record<NotificationChannel, string> = { email: 'Email', sms: 'SMS' };

type Mode = 'existing' | 'new';
type NewField = 'name' | 'email' | 'phone';

interface RetryState {
  recipientId: number;
  recipientName: string;
  done: NotificationChannel[];
  failed: { channel: NotificationChannel; message: string }[];
}

interface AddSubscriberFormProps {
  assetId: number;
  assetName: string;
  onDone: () => void;
  onCancel: () => void;
}

const inputClass = (hasError: boolean) =>
  `block w-full px-3 py-2 border rounded-lg ${
    hasError
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`;

const LABEL = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1';
const FIELD_ERROR = 'mt-1 text-sm text-red-600 dark:text-red-400';

function channelList(channels: NotificationChannel[]) {
  return CHANNELS.filter((c) => channels.includes(c))
    .map((c) => CHANNEL_LABELS[c])
    .join(' and ');
}

function isConflict(error: unknown) {
  return (error as { response?: { status?: number } })?.response?.status === 409;
}

/**
 * Subscribe a contact to one asset by email, SMS or both. A new contact is
 * created first; if a subscribe call then fails the contact is kept and only
 * the failed channels are offered for retry.
 */
export function AddSubscriberForm({ assetId, assetName, onDone, onCancel }: AddSubscriberFormProps) {
  const { recipients, isLoading } = useNotificationRecipients();
  const { create } = useNotificationRecipientMutations();
  const { subscribe } = useAssetSubscriptionMutations(assetId);

  const contacts = useMemo(
    () => recipients.filter((r) => r.is_active).sort((a, b) => a.name.localeCompare(b.name)),
    [recipients]
  );
  const noContacts = !isLoading && contacts.length === 0;

  const [modeChoice, setModeChoice] = useState<Mode | null>(null);
  const mode: Mode = modeChoice ?? (noContacts ? 'new' : 'existing');

  const [search, setSearch] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [form, setForm] = useState<ContactFormInput>({
    name: '',
    email: '',
    phoneCode: '',
    phoneNumber: '',
  });
  // New-contact fields are checked once left, and all of them after a submit
  // attempt; from then on the messages follow what is typed.
  const [touched, setTouched] = useState<Partial<Record<NewField, boolean>>>({});
  const [submitted, setSubmitted] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const [unchecked, setUnchecked] = useState<Set<NotificationChannel>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [retry, setRetry] = useState<RetryState | null>(null);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return contacts;
    return contacts.filter((r) =>
      [r.name, r.email, r.phone].some((v) => v?.toLowerCase().includes(q))
    );
  }, [contacts, search]);
  const selected = filtered.find((r) => r.id === selectedId);
  const phone = joinPhone({ code: form.phoneCode, number: form.phoneNumber });

  const hasDetail: Record<NotificationChannel, boolean> =
    mode === 'existing'
      ? { email: !!selected?.email, sms: !!selected?.phone }
      : { email: form.email.trim() !== '', sms: phone !== '' };
  const checked = CHANNELS.filter((c) => hasDetail[c] && !unchecked.has(c));
  const locked = submitting || retry !== null;
  const needsContact = mode === 'existing' && !selected;

  const found: ContactErrors = mode === 'new' ? validateContactForm(form) : {};
  const show = (field: NewField) => submitted || !!touched[field];
  const fieldErrors: ContactErrors = {
    name: show('name') ? found.name : undefined,
    email: show('email') ? found.email : undefined,
    phoneCode: show('phone') ? found.phoneCode : undefined,
    phoneNumber: show('phone') ? found.phoneNumber : undefined,
    contact: submitted || (touched.email && touched.phone) ? found.contact : undefined,
  };

  const setField = (patch: Partial<ContactFormInput>) => {
    setForm((prev) => ({ ...prev, ...patch }));
    setServerError(null);
  };
  const touch = (field: NewField) => setTouched((prev) => ({ ...prev, [field]: true }));

  const toggleChannel = (channel: NotificationChannel, on: boolean) => {
    setUnchecked((prev) => {
      const next = new Set(prev);
      if (on) next.delete(channel);
      else next.add(channel);
      return next;
    });
  };

  const runSubscribe = async (
    recipientId: number,
    recipientName: string,
    channels: NotificationChannel[],
    done: NotificationChannel[]
  ) => {
    let failed: RetryState['failed'];
    let allDone = done;
    try {
      const result = await subscribe({ recipientId, channels });
      allDone = [...done, ...result.succeeded.map((s) => s.channel)];
      failed = result.failed.map(({ channel, error }) => ({
        channel,
        message: getApiErrorMessage(error, 'Subscribe failed'),
      }));
    } catch (err) {
      const message = getApiErrorMessage(err, 'Subscribe failed');
      failed = channels.map((channel) => ({ channel, message }));
    }
    if (failed.length === 0) {
      toast.success(`${recipientName} will get ${channelList(allDone)} alerts for ${assetName}`);
      onDone();
      return;
    }
    setRetry({ recipientId, recipientName, done: allDone, failed });
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (locked) return;

    setSubmitted(true);
    setServerError(null);
    if (needsContact || checked.length === 0 || Object.values(found).some(Boolean)) return;

    setSubmitting(true);
    try {
      let recipient = selected;
      if (mode === 'new') {
        try {
          recipient = await create({
            name: form.name.trim(),
            email: form.email.trim() || undefined,
            phone: phone || undefined,
          });
        } catch (err) {
          const message = getApiErrorMessage(err, 'Failed to create contact');
          setServerError(
            isConflict(err)
              ? `${message.replace(/\.?$/, '.')} That email or phone already belongs to another contact — pick them under "Existing contact" instead.`
              : message
          );
          return;
        }
      }
      if (recipient) await runSubscribe(recipient.id, recipient.name, checked, []);
    } finally {
      setSubmitting(false);
    }
  };

  const handleRetry = async () => {
    if (!retry || submitting) return;
    setSubmitting(true);
    try {
      await runSubscribe(
        retry.recipientId,
        retry.recipientName,
        retry.failed.map((f) => f.channel),
        retry.done
      );
    } finally {
      setSubmitting(false);
    }
  };

  const modeButton = (value: Mode, label: string) => (
    <button
      type="button"
      data-testid={`add-subscriber-mode-${value}`}
      aria-pressed={mode === value}
      disabled={locked}
      onClick={() => {
        setModeChoice(value);
        setTouched({});
        setSubmitted(false);
        setServerError(null);
      }}
      className={`flex-1 px-3 py-1.5 text-sm font-medium rounded-md transition-colors disabled:opacity-50 ${
        mode === value
          ? 'bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 shadow-sm'
          : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100'
      }`}
    >
      {label}
    </button>
  );

  return (
    <form
      onSubmit={handleSubmit}
      noValidate
      data-testid="add-subscriber-form"
      aria-label="Add subscriber"
      className="mb-4 p-3 sm:p-4 space-y-4 border border-gray-200 dark:border-gray-700 rounded-lg bg-gray-50 dark:bg-gray-900/30"
    >
      <div className="flex gap-1 p-1 rounded-lg bg-gray-200 dark:bg-gray-700" role="group" aria-label="Contact type">
        {modeButton('existing', 'Existing contact')}
        {modeButton('new', 'New contact')}
      </div>

      {serverError && (
        <div role="alert" className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
          <p className="text-sm text-red-600 dark:text-red-400">{serverError}</p>
        </div>
      )}

      {mode === 'existing' ? (
        <div>
          <label htmlFor="add-subscriber-contact-search" className={LABEL}>
            Contact
          </label>
          <input
            id="add-subscriber-contact-search"
            data-testid="add-subscriber-contact-search"
            type="search"
            placeholder="Search by name, email or phone"
            value={search}
            disabled={locked}
            onChange={(e) => setSearch(e.target.value)}
            className={inputClass(false)}
          />
          {/* Radio rows rather than a native list box: nothing looks picked
              until the user picks, and long details wrap on narrow screens. */}
          {filtered.length > 0 && (
            <div
              role="radiogroup"
              aria-label="Choose a contact"
              data-testid="add-subscriber-contact-select"
              className="mt-2 max-h-48 overflow-y-auto border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700"
            >
              {filtered.map((r) => (
                <label
                  key={r.id}
                  className={`flex items-start gap-3 px-3 py-2 cursor-pointer ${
                    r.id === selectedId ? 'bg-blue-50 dark:bg-blue-900/20' : 'hover:bg-gray-50 dark:hover:bg-gray-700/50'
                  }`}
                >
                  <input
                    type="radio"
                    name="add-subscriber-contact"
                    data-testid={`add-subscriber-contact-${r.id}`}
                    value={r.id}
                    checked={r.id === selectedId}
                    disabled={locked}
                    onChange={() => {
                      setSelectedId(r.id);
                      setUnchecked(new Set());
                    }}
                    className="mt-1 h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 focus:ring-blue-500"
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-gray-900 dark:text-gray-100">
                      {r.name}
                    </span>
                    <span className="block text-xs text-gray-500 dark:text-gray-400 break-all">
                      {[r.email, r.phone].filter(Boolean).join(' · ')}
                    </span>
                  </span>
                </label>
              ))}
            </div>
          )}
          {filtered.length === 0 && (
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {isLoading ? 'Loading contacts…' : 'No contacts match your search.'}
            </p>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {noContacts && (
            <p className="text-sm text-gray-600 dark:text-gray-400">
              This organization has no contacts yet. Add one to subscribe them.
            </p>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {(
              [
                ['name', 'Name', 'text'],
                ['email', 'Email', 'email'],
              ] as const
            ).map(([field, label, type]) => (
              <div key={field} className={field === 'name' ? 'sm:col-span-2' : undefined}>
                <label htmlFor={`add-subscriber-${field}`} className={LABEL}>
                  {label}
                  {field === 'name' && <span className="text-red-500"> *</span>}
                </label>
                <input
                  id={`add-subscriber-${field}`}
                  data-testid={`add-subscriber-${field}`}
                  type={type}
                  value={form[field]}
                  disabled={locked}
                  aria-invalid={!!fieldErrors[field]}
                  onChange={(e) => setField({ [field]: e.target.value })}
                  onBlur={() => touch(field)}
                  className={inputClass(!!fieldErrors[field])}
                />
                {fieldErrors[field] && <p className={FIELD_ERROR}>{fieldErrors[field]}</p>}
              </div>
            ))}
            <PhoneInput
              id="add-subscriber-phone"
              value={{ code: form.phoneCode, number: form.phoneNumber }}
              onChange={({ code, number }) => setField({ phoneCode: code, phoneNumber: number })}
              onBlur={() => touch('phone')}
              errors={{ phoneCode: fieldErrors.phoneCode, phoneNumber: fieldErrors.phoneNumber }}
              disabled={locked}
            />
          </div>
          {fieldErrors.contact && <p className={FIELD_ERROR}>{fieldErrors.contact}</p>}
        </div>
      )}

      <fieldset>
        <legend className={LABEL}>Send alerts by</legend>
        <div className="flex flex-col sm:flex-row gap-2 sm:gap-6">
          {CHANNELS.map((channel) => (
            <div key={channel}>
              <label className="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input
                  type="checkbox"
                  data-testid={`add-subscriber-channel-${channel}`}
                  checked={checked.includes(channel)}
                  disabled={locked || needsContact || !hasDetail[channel]}
                  onChange={(e) => toggleChannel(channel, e.target.checked)}
                  className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500 disabled:opacity-50"
                />
                {CHANNEL_LABELS[channel]}
              </label>
              {!hasDetail[channel] && !needsContact && (
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {mode === 'new'
                    ? channel === 'email'
                      ? 'Enter an email to use this'
                      : 'Enter a phone number to use this'
                    : channel === 'email'
                      ? 'No email on file'
                      : 'No phone on file'}
                </p>
              )}
            </div>
          ))}
        </div>
        {needsContact ? (
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">Choose a contact first</p>
        ) : (
          submitted &&
          checked.length === 0 && <p className={FIELD_ERROR}>Choose at least one channel</p>
        )}
      </fieldset>

      {retry && (
        <div role="alert" className="p-3 space-y-1 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm">
          {retry.done.length > 0 && (
            <p className="text-gray-700 dark:text-gray-300">
              {retry.recipientName} is subscribed by {channelList(retry.done)}.
            </p>
          )}
          {retry.failed.map(({ channel, message }) => (
            <p key={channel} className="text-red-600 dark:text-red-400">
              {CHANNEL_LABELS[channel]} failed: {message}
            </p>
          ))}
          <p className="text-gray-600 dark:text-gray-400">
            {retry.recipientName} is saved as a contact; retry to finish subscribing.
          </p>
        </div>
      )}

      <div className="flex flex-col-reverse sm:flex-row sm:justify-end gap-2 sm:gap-3">
        <button
          type="button"
          onClick={retry ? onDone : onCancel}
          disabled={submitting}
          className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
        >
          {retry ? 'Close' : 'Cancel'}
        </button>
        <PaidGate surface="notifications-subscriptions" silentImpression className="sm:w-auto">
          {retry ? (
            <button
              type="button"
              data-testid="add-subscriber-retry"
              onClick={handleRetry}
              disabled={submitting}
              className="w-full px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
            >
              {submitting ? 'Retrying…' : 'Retry subscribe'}
            </button>
          ) : (
            <button
              type="submit"
              data-testid="add-subscriber-submit"
              disabled={submitting || needsContact || checked.length === 0}
              className="w-full px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
            >
              {submitting ? 'Subscribing…' : 'Subscribe'}
            </button>
          )}
        </PaidGate>
      </div>
    </form>
  );
}
