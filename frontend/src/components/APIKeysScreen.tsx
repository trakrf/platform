import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { useOrgStore } from '@/stores';
import { apiKeysApi } from '@/lib/api/apiKeys';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse, Scope } from '@/types/apiKey';
import { CreateKeyModal } from './apikeys/CreateKeyModal';
import { ShowOnceModal } from './apikeys/ShowOnceModal';
import { RevokeConfirmModal } from './apikeys/RevokeConfirmModal';

function scopeChip(scopes: Scope[], resource: 'assets' | 'locations' | 'scans'): string | null {
  const read = scopes.includes(`${resource}:read` as Scope);
  const write = scopes.includes(`${resource}:write` as Scope);
  if (read && write) return `${resource.charAt(0).toUpperCase()}${resource.slice(1)} R/W`;
  if (read) return `${resource.charAt(0).toUpperCase()}${resource.slice(1)} R`;
  return null;
}

function formatDate(iso: string | null): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString();
}

function formatRelative(iso: string | null): string {
  if (!iso) return 'Never';
  const diffMs = Date.now() - new Date(iso).getTime();
  const hours = Math.floor(diffMs / 3_600_000);
  if (hours < 1) return 'just now';
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function APIKeysScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [newKey, setNewKey] = useState<APIKeyCreateResponse | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<APIKey | null>(null);

  const isAdmin = currentRole === 'owner' || currentRole === 'admin';
  const orgId = currentOrg?.id ?? 0;

  const { data: keys, isLoading } = useQuery({
    queryKey: ['apiKeys', orgId],
    queryFn: async () => {
      const resp = await apiKeysApi.list(orgId);
      return resp.data;
    },
    enabled: isAdmin && orgId > 0,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateAPIKeyRequest) => apiKeysApi.create(orgId, req),
    onSuccess: (resp) => {
      setCreating(false);
      setNewKey(resp);
      queryClient.invalidateQueries({ queryKey: ['apiKeys', orgId] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to create key');
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (id: number) => apiKeysApi.revoke(orgId, id),
    onSuccess: () => {
      toast.success('Key revoked');
      setRevokeTarget(null);
      queryClient.invalidateQueries({ queryKey: ['apiKeys', orgId] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke key');
    },
  });

  if (!isAdmin) {
    return (
      <div className="max-w-3xl mx-auto py-8">
        <h1 className="text-2xl font-semibold">API Keys</h1>
        <p className="mt-4 text-gray-600 dark:text-gray-400">
          Only organization admins can manage API keys.
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-3xl mx-auto py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold">API Keys</h1>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="px-4 py-2 text-sm bg-blue-600 text-white rounded"
        >
          New key
        </button>
      </div>

      {isLoading && <p className="text-sm text-gray-500">Loading…</p>}

      {!isLoading && keys?.length === 0 && (
        <p className="text-sm text-gray-600 dark:text-gray-400">
          No API keys yet. Create one to let an external system talk to TrakRF.
        </p>
      )}

      {!isLoading && keys && keys.length > 0 && (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b">
              <th className="py-2">Name</th>
              <th>Scopes</th>
              <th>Created</th>
              <th>Last used</th>
              <th>Expires</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => {
              const chips = (['assets', 'locations', 'scans'] as const)
                .map((r) => scopeChip(k.scopes, r))
                .filter((x): x is string => !!x);
              return (
                <tr key={k.id} className="border-b">
                  <td className="py-2 font-medium">{k.name}</td>
                  <td className="space-x-1">
                    {chips.map((c) => (
                      <span
                        key={c}
                        className="inline-block bg-gray-100 dark:bg-gray-700 rounded px-2 py-0.5 text-xs"
                        title={k.scopes.join(', ')}
                      >
                        {c}
                      </span>
                    ))}
                  </td>
                  <td>{formatDate(k.created_at)}</td>
                  <td>{formatRelative(k.last_used_at)}</td>
                  <td>{k.expires_at ? formatDate(k.expires_at) : 'Never'}</td>
                  <td>
                    <button
                      type="button"
                      onClick={() => setRevokeTarget(k)}
                      className="text-red-600 text-xs hover:underline"
                    >
                      Revoke
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {creating && (
        <CreateKeyModal
          onCreate={(req) => createMutation.mutate(req)}
          onCancel={() => setCreating(false)}
          busy={createMutation.isPending}
        />
      )}

      {newKey && (
        <ShowOnceModal
          apiKey={newKey.key}
          onClose={() => setNewKey(null)}
        />
      )}

      {revokeTarget && (
        <RevokeConfirmModal
          keyName={revokeTarget.name}
          onConfirm={() => revokeMutation.mutate(revokeTarget.id)}
          onCancel={() => setRevokeTarget(null)}
          busy={revokeMutation.isPending}
        />
      )}
    </div>
  );
}
