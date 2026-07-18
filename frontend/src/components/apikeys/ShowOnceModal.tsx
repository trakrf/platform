import { useState, useEffect } from 'react';

interface Props {
  // OAuth2 client_credentials pair returned once at key creation. The grant
  // needs both values; the secret is the only irretrievable one.
  clientId: string;
  clientSecret: string;
  onClose: () => void;
}

// Fallback dwell time (ms) before dismissal is enabled without a clipboard copy —
// protects users on browsers without navigator.clipboard or insecure origins.
const DWELL_MS = 3000;

interface CredentialFieldProps {
  label: string;
  value: string;
  onCopied?: () => void;
}

function CredentialField({ label, value, onCopied }: CredentialFieldProps) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    // Guard for browsers without navigator.clipboard (insecure origins); the
    // dwell fallback still lets the user dismiss the modal after reading it.
    await navigator.clipboard?.writeText(value);
    setCopied(true);
    onCopied?.();
  };

  return (
    <div className="space-y-1">
      <div className="text-xs font-medium text-gray-600 dark:text-gray-400">{label}</div>
      <div className="flex items-stretch gap-2">
        <div className="flex-1 bg-gray-100 dark:bg-gray-900 rounded p-3 break-all font-mono text-xs">
          {value}
        </div>
        <button
          type="button"
          onClick={copy}
          aria-label={`Copy ${label}`}
          className="px-4 text-sm bg-blue-600 text-white rounded shrink-0"
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
    </div>
  );
}

export function ShowOnceModal({ clientId, clientSecret, onClose }: Props) {
  const [secretCopied, setSecretCopied] = useState(false);
  const [dwellReady, setDwellReady] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setDwellReady(true), DWELL_MS);
    return () => clearTimeout(timer);
  }, []);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-lg space-y-4">
        <h2 className="text-lg font-semibold">API key created</h2>
        <div className="bg-amber-100 dark:bg-amber-900/30 border border-amber-300 text-amber-900 dark:text-amber-200 rounded px-3 py-2 text-sm">
          <strong>This is the only time you&apos;ll see the client secret.</strong>{' '}
          Copy both values now — you need them together to request an access
          token. If you lose the secret, revoke this key and create a new one.
        </div>
        <CredentialField label="Client ID" value={clientId} />
        <CredentialField
          label="Client secret"
          value={clientSecret}
          onCopied={() => setSecretCopied(true)}
        />
        <div className="flex justify-end">
          <button
            type="button"
            onClick={onClose}
            disabled={!secretCopied && !dwellReady}
            className="px-4 py-2 text-sm border rounded disabled:opacity-50"
          >
            I&apos;ve saved it
          </button>
        </div>
      </div>
    </div>
  );
}
