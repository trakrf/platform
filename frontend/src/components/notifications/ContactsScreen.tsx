import { useEffect, useMemo, useState } from 'react';
import { Contact, Pencil, Plus, Search, Trash2, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { useUIStore } from '@/stores';
import {
  useNotificationRecipientMutations,
  useNotificationRecipients,
} from '@/hooks/notifications';
import { useRecipientSubscriptionCounts } from '@/hooks/notifications/useRecipientSubscriptionCounts';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ConfirmModal, EmptyState } from '@/components/shared';
import { PaidGate } from '@/components/entitlement';
import { NotificationsGate } from '@/components/notifications/NotificationsGate';
import { ContactFormModal } from '@/components/notifications/ContactFormModal';
import type { NotificationRecipient } from '@/types/notifications';

const PILL_BASE = 'inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border';
const PILL_ACTIVE =
  'bg-green-50 text-green-700 border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800';
const PILL_INACTIVE =
  'bg-gray-50 text-gray-700 border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800';

function ActivePill({ active }: { active: boolean }) {
  return (
    <span className={`${PILL_BASE} ${active ? PILL_ACTIVE : PILL_INACTIVE}`}>
      {active ? 'Active' : 'Inactive'}
    </span>
  );
}

const ICON_BUTTON = 'p-1.5 text-gray-500 dark:text-gray-400';
const CARD_BUTTON =
  'flex-1 flex items-center justify-center gap-1 sm:gap-2 px-2 sm:px-3 py-1.5 sm:py-2 text-xs sm:text-sm font-medium border rounded-lg transition-colors';

type FormState = { mode: 'create' } | { mode: 'edit'; contact: NotificationRecipient } | null;

function assetsPhrase(count: number) {
  return `${count} asset${count === 1 ? '' : 's'}`;
}

function deleteMessage(name: string, count: number) {
  const subscribed =
    count === 0
      ? 'They are not subscribed to any assets.'
      : `They are subscribed to ${assetsPhrase(count)} and will stop receiving all alerts.`;
  return `Delete ${name}? ${subscribed} Their alert history is kept.`;
}

function matches(recipient: NotificationRecipient, term: string) {
  return [recipient.name, recipient.email, recipient.phone].some((field) =>
    field?.toLowerCase().includes(term)
  );
}

// The page title comes from Header's PAGE_TITLES, so there is no in-page h1.
export default function ContactsScreen() {
  const { setActiveTab } = useUIStore();
  const { recipients, isLoading, error } = useNotificationRecipients();
  const { counts, isLoading: countsLoading } = useRecipientSubscriptionCounts();
  const { delete: deleteRecipient } = useNotificationRecipientMutations();
  const [search, setSearch] = useState('');
  const [form, setForm] = useState<FormState>(null);
  const [deleting, setDeleting] = useState<NotificationRecipient | null>(null);

  useEffect(() => {
    setActiveTab('notification-contacts');
  }, [setActiveTab]);

  const term = search.trim().toLowerCase();
  const visible = useMemo(
    () => (term ? recipients.filter((r) => matches(r, term)) : recipients),
    [recipients, term]
  );

  const confirmDelete = async () => {
    if (!deleting) return;
    try {
      await deleteRecipient(deleting.id);
      toast.success('Contact deleted');
      setDeleting(null);
    } catch (err) {
      toast.error(getApiErrorMessage(err, 'Failed to delete contact'));
    }
  };

  const countOf = (id: number) => counts.get(id) ?? 0;
  const renderCount = (id: number, label: (n: number) => string) =>
    countsLoading ? (
      <span aria-label="Loading subscription count">…</span>
    ) : (
      label(countOf(id))
    );

  const rowActions = (r: NotificationRecipient) => (
    <PaidGate surface="notifications-contacts" silentImpression>
      <div className="flex items-center justify-end gap-1">
        <button
          type="button"
          onClick={() => setForm({ mode: 'edit', contact: r })}
          className={`${ICON_BUTTON} hover:text-blue-600 dark:hover:text-blue-400`}
          aria-label={`Edit ${r.name}`}
        >
          <Pencil className="w-4 h-4" />
        </button>
        <button
          type="button"
          onClick={() => setDeleting(r)}
          className={`${ICON_BUTTON} hover:text-red-600 dark:hover:text-red-400`}
          aria-label={`Delete ${r.name}`}
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>
    </PaidGate>
  );

  const cardActions = (r: NotificationRecipient) => (
    <PaidGate surface="notifications-contacts" silentImpression className="flex-1">
      <div className="flex gap-1.5 sm:gap-2 flex-1">
        <button
          type="button"
          onClick={() => setForm({ mode: 'edit', contact: r })}
          className={`${CARD_BUTTON} text-gray-700 bg-gray-50 hover:bg-gray-100 dark:text-gray-400 dark:bg-gray-900/20 dark:hover:bg-gray-900/40 border-gray-200 dark:border-gray-800`}
          aria-label={`Edit ${r.name}`}
        >
          <Pencil className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
          <span>Edit</span>
        </button>
        <button
          type="button"
          onClick={() => setDeleting(r)}
          className={`${CARD_BUTTON} text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border-red-200 dark:border-red-800`}
          aria-label={`Delete ${r.name}`}
        >
          <Trash2 className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
          <span>Delete</span>
        </button>
      </div>
    </PaidGate>
  );

  const renderBody = () => {
    if (isLoading) {
      return <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading contacts…</p>;
    }
    if (error) {
      return (
        <p role="alert" className="p-4 text-sm text-red-600 dark:text-red-400">
          {getApiErrorMessage(error, 'Failed to load contacts')}
        </p>
      );
    }
    if (recipients.length === 0) {
      return (
        <div data-testid="contacts-empty">
          <EmptyState
            icon={Contact}
            title="No contacts yet"
            description="Add the people who should receive asset alerts."
            action={{ label: 'Add contact', onClick: () => setForm({ mode: 'create' }) }}
          />
        </div>
      );
    }
    if (visible.length === 0) {
      return (
        <div className="p-8 flex flex-col items-center text-center gap-3">
          <p className="text-sm text-gray-600 dark:text-gray-400">
            No contacts match &quot;{search.trim()}&quot;.
          </p>
          <button
            type="button"
            onClick={() => setSearch('')}
            className="px-4 py-2 text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 hover:underline transition-colors rounded"
          >
            Clear search
          </button>
        </div>
      );
    }
    return (
      <>
        <table data-testid="contacts-table" className="hidden md:table w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-800">
            <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
              <th className="py-2 px-3 font-medium">Name</th>
              <th className="px-3 font-medium">Email</th>
              <th className="px-3 font-medium">Phone</th>
              <th className="px-3 font-medium">Active</th>
              <th className="px-3 font-medium">Subscribed assets</th>
              <th className="px-3 font-medium text-right">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map((r) => (
              <tr
                key={r.id}
                data-testid={`contact-row-${r.id}`}
                className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50"
              >
                <td className="py-2 px-3 text-gray-900 dark:text-gray-100">{r.name}</td>
                <td className="px-3 text-gray-700 dark:text-gray-300">{r.email || '—'}</td>
                <td className="px-3 text-gray-700 dark:text-gray-300">{r.phone || '—'}</td>
                <td className="px-3">
                  <ActivePill active={r.is_active} />
                </td>
                <td
                  data-testid={`contact-count-${r.id}`}
                  className="px-3 text-gray-700 dark:text-gray-300"
                >
                  {renderCount(r.id, String)}
                </td>
                <td className="px-3 text-right whitespace-nowrap">{rowActions(r)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <ul className="md:hidden flex flex-col gap-2 p-2">
          {visible.map((r) => (
            <li
              key={r.id}
              data-testid={`contact-card-${r.id}`}
              className="border border-gray-200 dark:border-gray-700 rounded-lg p-3 sm:p-4"
            >
              <div className="flex items-start justify-between gap-2 mb-1">
                <h3 className="text-sm sm:text-base font-semibold text-gray-900 dark:text-white truncate">
                  {r.name}
                </h3>
                <ActivePill active={r.is_active} />
              </div>
              {r.email && (
                <p className="text-xs sm:text-sm text-gray-700 dark:text-gray-300 truncate">
                  {r.email}
                </p>
              )}
              {r.phone && (
                <p className="text-xs sm:text-sm text-gray-700 dark:text-gray-300 truncate">
                  {r.phone}
                </p>
              )}
              <p className="text-xs sm:text-sm text-gray-500 dark:text-gray-400 mt-1">
                {renderCount(r.id, (n) => `Subscribed to ${assetsPhrase(n)}`)}
              </p>
              <div className="flex pt-2 sm:pt-3 mt-2 border-t border-gray-200 dark:border-gray-700">
                {cardActions(r)}
              </div>
            </li>
          ))}
        </ul>
      </>
    );
  };

  return (
    <div className="h-full flex flex-col p-2">
      <NotificationsGate>
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 mb-4">
          <div className="relative flex-1 sm:max-w-md">
            <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <Search className="h-5 w-5 text-gray-400 dark:text-gray-500" />
            </div>
            <input
              type="text"
              data-testid="contacts-search"
              aria-label="Search contacts"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search contacts..."
              className="block w-full pl-10 pr-10 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 focus:border-transparent"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch('')}
                aria-label="Clear search"
                className="absolute inset-y-0 right-0 pr-3 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            )}
          </div>
          <PaidGate surface="notifications-contacts">
            <button
              type="button"
              data-testid="contacts-new"
              onClick={() => setForm({ mode: 'create' })}
              className="flex items-center justify-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 transition-colors"
            >
              <Plus className="w-4 h-4" />
              New contact
            </button>
          </PaidGate>
        </div>
        <div className="flex-1 overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg">
          {renderBody()}
        </div>
        <ContactFormModal
          key={form?.mode === 'edit' ? form.contact.id : 'create'}
          isOpen={form !== null}
          mode={form?.mode ?? 'create'}
          contact={form?.mode === 'edit' ? form.contact : undefined}
          onClose={() => setForm(null)}
        />
        <ConfirmModal
          isOpen={deleting !== null}
          title="Delete contact"
          message={deleting ? deleteMessage(deleting.name, countOf(deleting.id)) : ''}
          confirmText="Delete"
          onConfirm={confirmDelete}
          onCancel={() => setDeleting(null)}
        />
      </NotificationsGate>
    </div>
  );
}
