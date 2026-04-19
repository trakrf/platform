import { useState } from 'react';
import type { CreateAPIKeyRequest, Scope } from '@/types/apiKey';
import { ScopeSelector } from './ScopeSelector';
import { ExpirySelector } from './ExpirySelector';

interface Props {
  onCreate: (req: CreateAPIKeyRequest) => void;
  onCancel: () => void;
  busy?: boolean;
}

function defaultName(): string {
  const today = new Date().toISOString().slice(0, 10);
  return `API key — ${today}`;
}

export function CreateKeyModal({ onCreate, onCancel, busy }: Props) {
  const [name, setName] = useState(defaultName());
  const [scopes, setScopes] = useState<Scope[]>([]);
  const [expires, setExpires] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (scopes.length === 0) {
      setError('Select at least one permission.');
      return;
    }
    setError(null);
    onCreate({ name, scopes, expires_at: expires });
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <form
        onSubmit={submit}
        className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-md space-y-4"
      >
        <h2 className="text-lg font-semibold">Create API key</h2>
        <div>
          <label htmlFor="key-name" className="block text-sm font-medium mb-1">
            Name
          </label>
          <input
            id="key-name"
            aria-label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={255}
            className="w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-900"
          />
        </div>
        <ScopeSelector value={scopes} onChange={setScopes} />
        <ExpirySelector value={expires} onChange={setExpires} />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm border rounded"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={busy}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded disabled:opacity-50"
          >
            Create key
          </button>
        </div>
      </form>
    </div>
  );
}
