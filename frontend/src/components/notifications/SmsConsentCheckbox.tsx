const TERMS_URL = 'https://docs.trakrf.id/terms';
const PRIVACY_URL = 'https://docs.trakrf.id/privacy';
const LINK = 'text-blue-600 dark:text-blue-400 underline';

interface SmsConsentCheckboxProps {
  contactName: string;
  phone: string;
  assetName: string;
  /** SMS is ticked under "Send alerts by"; until then the box is shown but inactive. */
  smsChosen: boolean;
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}

/**
 * The operator's confirmation that a contact agreed to TrakRF SMS alerts.
 * Carrier and toll-free rules require it to be an active choice with the
 * program disclosures beside it, so the parent never pre-ticks it. It is always
 * shown, so the terms are visible before anyone chooses SMS.
 */
export function SmsConsentCheckbox({
  contactName,
  phone,
  assetName,
  smsChosen,
  checked,
  disabled,
  onChange,
}: SmsConsentCheckboxProps) {
  return (
    <div
      data-testid="add-subscriber-sms-consent"
      className="p-3 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800"
    >
      <label className="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          type="checkbox"
          data-testid="add-subscriber-sms-consent-checkbox"
          checked={smsChosen && checked}
          disabled={disabled || !smsChosen}
          onChange={(e) => onChange(e.target.checked)}
          className="mt-0.5 h-4 w-4 shrink-0 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500 disabled:opacity-50"
        />
        <span>
          I confirm {contactName} has agreed to receive recurring automated SMS alerts from TrakRF
          about {assetName} at {phone}. Consent is not a condition of purchase. Message frequency
          varies. Message and data rates may apply. Reply HELP for help or STOP to opt out. See the{' '}
          <a href={TERMS_URL} target="_blank" rel="noreferrer" className={LINK}>
            Terms and Conditions
          </a>{' '}
          and{' '}
          <a href={PRIVACY_URL} target="_blank" rel="noreferrer" className={LINK}>
            Privacy Policy
          </a>
          .
        </span>
      </label>
      {!(smsChosen && checked) && (
        <p className="mt-1 ml-6 text-xs text-gray-500 dark:text-gray-400">
          {smsChosen
            ? 'Required to subscribe by SMS. Untick SMS if they have not agreed.'
            : 'Choose SMS above to subscribe by text; this confirmation is then required.'}
        </p>
      )}
    </div>
  );
}
