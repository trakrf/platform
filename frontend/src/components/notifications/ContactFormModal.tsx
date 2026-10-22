import React, { useState, type FormEvent } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import { PaidGate } from '@/components/entitlement';
import { useNotificationRecipientMutations } from '@/hooks/notifications';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import {
  joinPhone,
  splitPhone,
  validateContactForm,
  type ContactErrors,
  type ContactFormInput,
} from '@/lib/notifications/validators';
import { PhoneInput } from './PhoneInput';
import type {
  CreateNotificationRecipientRequest,
  NotificationRecipient,
  UpdateNotificationRecipientRequest,
} from '@/types/notifications';

interface ContactFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  contact?: NotificationRecipient;
  onClose: () => void;
}

type TextField = 'name' | 'email';
type Touched = Partial<Record<TextField | 'phone', boolean>>;
type ServerErrors = Pick<ContactErrors, 'email' | 'phoneNumber'> & { form?: string };

// The backend cannot clear a field, so an existing email or phone can only be
// replaced, never emptied.
const CANNOT_REMOVE = "This can't be removed yet — replace it instead";

const INPUT_BASE =
  'block w-full px-3 py-2 border rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50';
const inputClass = (hasError: boolean) =>
  `${INPUT_BASE} ${
    hasError
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  }`;
const LABEL = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2';
const FIELD_ERROR = 'mt-1 text-sm text-red-600 dark:text-red-400';
const PLACEHOLDERS: Record<TextField, string> = {
  name: 'e.g., Jane Smith',
  email: 'name@example.com',
};

// Outer gate returns null when closed so the stateful body unmounts each
// open/close cycle (mirrors OutputDeviceFormModal).
export function ContactFormModal(props: ContactFormModalProps) {
  if (!props.isOpen) return null;
  return <ContactFormModalBody {...props} />;
}

// A 409 detail that names the field goes under it; otherwise, when only one of
// email/phone was sent, that one is the duplicate.
function conflictField(detail: string, sent: { email?: string; phone?: string }) {
  const text = detail.toLowerCase();
  if (text.includes('email')) return 'email';
  if (text.includes('phone')) return 'phone';
  if (sent.email && !sent.phone) return 'email';
  if (sent.phone && !sent.email) return 'phone';
  return null;
}

function ContactFormModalBody({ isOpen, mode, contact, onClose }: ContactFormModalProps) {
  const initialPhone = splitPhone(contact?.phone);
  const [values, setValues] = useState<ContactFormInput>({
    name: contact?.name ?? '',
    email: contact?.email ?? '',
    phoneCode: initialPhone.code,
    phoneNumber: initialPhone.number,
  });
  const [isActive, setIsActive] = useState(contact?.is_active ?? true);
  // Errors show for a field once it has been left, and for every field after a
  // submit attempt; from then on they update as the user types.
  const [touched, setTouched] = useState<Touched>({});
  const [submitted, setSubmitted] = useState(false);
  const [serverErrors, setServerErrors] = useState<ServerErrors>({});
  const [loading, setLoading] = useState(false);
  const { create, update } = useNotificationRecipientMutations();

  useEscapeToClose(isOpen, onClose, loading);

  const validate = (input: ContactFormInput): ContactErrors => {
    const result = validateContactForm(input);
    if (mode === 'edit' && contact) {
      if (contact.email && !input.email.trim()) result.email = CANNOT_REMOVE;
      if (contact.phone && !joinPhone({ code: input.phoneCode, number: input.phoneNumber })) {
        result.phoneNumber = CANNOT_REMOVE;
      }
    }
    return result;
  };

  const found = validate(values);
  const show = (field: TextField | 'phone') => submitted || !!touched[field];
  const errors: ContactErrors & { form?: string } = {
    name: show('name') ? found.name : undefined,
    email: serverErrors.email ?? (show('email') ? found.email : undefined),
    phoneCode: show('phone') ? found.phoneCode : undefined,
    phoneNumber: serverErrors.phoneNumber ?? (show('phone') ? found.phoneNumber : undefined),
    contact:
      submitted || (touched.email && touched.phone) ? found.contact : undefined,
    form: serverErrors.form,
  };

  const handleChange = (patch: Partial<ContactFormInput>) => {
    setValues((prev) => ({ ...prev, ...patch }));
    setServerErrors({});
  };

  const handleBlur = (field: TextField | 'phone') =>
    setTouched((prev) => ({ ...prev, [field]: true }));

  const buildPayload = () => {
    const name = values.name.trim();
    const email = values.email.trim();
    const phone = joinPhone({ code: values.phoneCode, number: values.phoneNumber });
    if (mode === 'create') {
      const data: CreateNotificationRecipientRequest = { name };
      if (email) data.email = email;
      if (phone) data.phone = phone;
      return data;
    }
    const data: UpdateNotificationRecipientRequest = {};
    if (name !== contact?.name) data.name = name;
    if (email && email !== (contact?.email ?? '')) data.email = email;
    if (phone && phone !== (contact?.phone ?? '')) data.phone = phone;
    if (isActive !== contact?.is_active) data.is_active = isActive;
    return data;
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    if (Object.values(found).some(Boolean)) return;

    const payload = buildPayload();
    if (mode === 'edit' && Object.keys(payload).length === 0) {
      onClose();
      return;
    }

    setLoading(true);
    try {
      if (mode === 'create') {
        await create(payload as CreateNotificationRecipientRequest);
        toast.success('Contact added');
      } else if (contact) {
        await update({ id: contact.id, updates: payload });
        toast.success('Contact updated');
      }
      onClose();
    } catch (err) {
      const message = getApiErrorMessage(err, 'Failed to save contact. Please try again.');
      const status = (err as { response?: { status?: number } })?.response?.status;
      const field = status === 409 ? conflictField(message, payload) : null;
      if (field === 'email') setServerErrors({ email: 'Another contact already uses this email' });
      else if (field === 'phone')
        setServerErrors({ phoneNumber: 'Another contact already uses this phone number' });
      else setServerErrors({ form: message });
    } finally {
      setLoading(false);
    }
  };

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !loading) onClose();
  };

  const textField = (field: TextField, label: string) => (
    <div>
      <label htmlFor={`contact-${field}`} className={LABEL}>
        {label}
        {field === 'name' && <span className="text-red-500"> *</span>}
      </label>
      <input
        id={`contact-${field}`}
        data-testid={`contact-${field}`}
        type={field === 'email' ? 'email' : 'text'}
        value={values[field]}
        onChange={(e) => handleChange({ [field]: e.target.value })}
        onBlur={() => handleBlur(field)}
        disabled={loading}
        aria-invalid={!!errors[field]}
        className={inputClass(!!errors[field])}
        placeholder={PLACEHOLDERS[field]}
      />
      {errors[field] && <p className={FIELD_ERROR}>{errors[field]}</p>}
    </div>
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 sm:p-4"
      onClick={handleBackdropClick}
    >
      <div className="fixed inset-0 sm:relative sm:inset-auto w-full sm:max-w-lg bg-white dark:bg-gray-900 sm:rounded-lg shadow-xl sm:max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'New contact' : `Edit contact: ${contact?.name}`}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form data-testid="contact-form" onSubmit={handleSubmit} noValidate className="px-6 py-6 space-y-6">
          {errors.form && (
            <div
              role="alert"
              className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg"
            >
              <p className="text-sm text-red-600 dark:text-red-400">{errors.form}</p>
            </div>
          )}

          {textField('name', 'Name')}
          {textField('email', 'Email')}
          <PhoneInput
            id="contact-phone"
            value={{ code: values.phoneCode, number: values.phoneNumber }}
            onChange={({ code, number }) => handleChange({ phoneCode: code, phoneNumber: number })}
            onBlur={() => handleBlur('phone')}
            errors={{ phoneCode: errors.phoneCode, phoneNumber: errors.phoneNumber }}
            disabled={loading}
          />
          {errors.contact && (
            <p role="alert" className={FIELD_ERROR}>
              {errors.contact}
            </p>
          )}

          {mode === 'edit' && (
            <div className="flex items-center">
              <input
                type="checkbox"
                id="contact-active"
                data-testid="contact-active"
                checked={isActive}
                onChange={(e) => setIsActive(e.target.checked)}
                disabled={loading}
                className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500 disabled:opacity-50"
              />
              <label htmlFor="contact-active" className="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                Active
              </label>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-4 border-t border-gray-200 dark:border-gray-700">
            <button
              type="button"
              onClick={onClose}
              disabled={loading}
              className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
            >
              Cancel
            </button>
            <PaidGate surface="notifications-contacts" silentImpression>
              <button
                type="submit"
                data-testid="contact-submit"
                disabled={loading}
                className="px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
              >
                {loading ? 'Saving...' : mode === 'create' ? 'Add contact' : 'Save changes'}
              </button>
            </PaidGate>
          </div>
        </form>
      </div>
    </div>
  );
}
