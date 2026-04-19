import { useState } from 'react';

interface Props {
  apiKey: string;
  onClose: () => void;
}

export function ShowOnceModal({ apiKey, onClose }: Props) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(apiKey);
    setCopied(true);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-lg space-y-4">
        <h2 className="text-lg font-semibold">API key created</h2>
        <div className="bg-amber-100 dark:bg-amber-900/30 border border-amber-300 text-amber-900 dark:text-amber-200 rounded px-3 py-2 text-sm">
          <strong>This is the only time you&apos;ll see the full key.</strong> Copy
          it now. If you lose it, revoke this key and create a new one.
        </div>
        <div className="bg-gray-100 dark:bg-gray-900 rounded p-3 break-all font-mono text-xs">
          {apiKey}
        </div>
        <div className="flex justify-between gap-2">
          <button
            type="button"
            onClick={copy}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded"
          >
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button
            type="button"
            onClick={onClose}
            disabled={!copied}
            className="px-4 py-2 text-sm border rounded disabled:opacity-50"
          >
            I&apos;ve saved it
          </button>
        </div>
      </div>
    </div>
  );
}
