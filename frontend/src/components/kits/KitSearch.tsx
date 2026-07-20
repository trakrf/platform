import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Search } from 'lucide-react';
import { kitsApi, type KitSummary } from '@/lib/api/kits';
import { useKitStore } from '@/stores/kitStore';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import { buildLocateHash } from '@/utils/kitUtils';
import { TagRow, KitMetadataRows } from './VerifyResults';

/**
 * Typed lookup for the flattened Kits surface (TRA-1033): one box searched as
 * Lot # substring AND tag value, merged. Expanding a result loads the full
 * pair record — members with tag numbers (Locate per tag) + QA details.
 */

const KitDetail: React.FC<{ kitId: number }> = ({ kitId }) => {
  const { data, isLoading, error } = useQuery({
    queryKey: ['kit-detail', kitId],
    queryFn: async () => (await kitsApi.get(kitId)).data.data,
    staleTime: 30_000,
  });

  if (isLoading) {
    return <div className="py-2 text-sm text-gray-500 dark:text-gray-400">Loading…</div>;
  }
  if (error || !data) {
    return (
      <div className="py-2 text-sm text-red-600 dark:text-red-400">
        {getApiErrorMessage(error, 'Failed to load pair')}
      </div>
    );
  }
  return (
    <div className="mt-2">
      <KitMetadataRows metadata={data.metadata ?? {}} />
      {data.members.map((m) => (
        <div key={m.asset_id} className="py-1.5 border-t border-gray-200 dark:border-gray-700">
          <div className="text-sm font-medium text-gray-900 dark:text-gray-100">
            {m.role ? `${m.name} (${m.role})` : m.name}
          </div>
          {m.epcs.map((epc) => (
            <TagRow
              key={epc}
              epc={epc}
              onLocate={(value) => {
                window.location.hash = buildLocateHash(value);
              }}
            />
          ))}
        </div>
      ))}
    </div>
  );
};

const KitResultRow: React.FC<{ kit: KitSummary }> = ({ kit }) => (
  <details
    data-testid={`kit-find-result-${kit.id}`}
    className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-3"
  >
    <summary className="cursor-pointer list-none flex items-center gap-3">
      <span className="font-semibold text-gray-900 dark:text-gray-100">Lot {kit.label}</span>
      {kit.status === 'closed' && (
        <span className="px-2 py-0.5 text-xs rounded bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300">
          closed
        </span>
      )}
      <span className="text-sm text-gray-500 dark:text-gray-400">
        {kit.member_count} tag{kit.member_count === 1 ? '' : 's'}
      </span>
      {kit.latest_verification && (
        <span
          className={`ml-auto text-sm font-medium ${
            kit.latest_verification.result === 'complete'
              ? 'text-green-700 dark:text-green-400'
              : 'text-red-600 dark:text-red-400'
          }`}
        >
          last check: {kit.latest_verification.result === 'complete' ? 'valid' : 'invalid'}
        </span>
      )}
    </summary>
    <KitDetail kitId={kit.id} />
  </details>
);

const KitSearch: React.FC = () => {
  const [input, setInput] = React.useState('');
  const submitted = useKitStore((state) => state.searchQuery);
  const setSubmitted = useKitStore((state) => state.setSearchQuery);

  // Session Clear resets the store query — empty the box with it.
  React.useEffect(() => {
    if (submitted === '') setInput('');
  }, [submitted]);

  const { data, isFetching, error } = useQuery({
    queryKey: ['kit-find', submitted],
    enabled: submitted !== '',
    queryFn: async () => {
      const [byLabel, byEpc] = await Promise.all([
        kitsApi.search(submitted),
        kitsApi.listByMemberEpc(submitted),
      ]);
      const seen = new Set<number>();
      const merged: KitSummary[] = [];
      for (const k of [...byLabel.data.data, ...byEpc.data.data]) {
        if (!seen.has(k.id)) {
          seen.add(k.id);
          merged.push(k);
        }
      }
      return merged;
    },
  });

  const handleSearch = () => {
    const q = input.trim();
    if (q !== '') setSubmitted(q);
  };

  return (
    <div>
      <div className="mb-2">
        <ErrorBanner error={error ? getApiErrorMessage(error, 'Search failed') : null} />
      </div>

      <div className="flex gap-2">
        <input
          type="text"
          data-testid="kit-find-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleSearch();
          }}
          placeholder="Search Lot # or tag (e.g. 1184015)"
          className="flex-1 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
        />
        <button
          data-testid="kit-find-search"
          onClick={handleSearch}
          disabled={input.trim() === '' || isFetching}
          className="flex items-center px-4 py-2 rounded-lg font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          <Search className="w-4 h-4 mr-2" />
          {isFetching ? 'Searching…' : 'Search'}
        </button>
      </div>

      {data && data.length === 0 && (
        <div className="mt-2 text-sm text-gray-600 dark:text-gray-400">
          No pairs match &ldquo;{submitted}&rdquo;.
        </div>
      )}
      {data && data.length > 0 && (
        <div className="mt-2 space-y-2">
          {data.map((kit) => (
            <KitResultRow key={kit.id} kit={kit} />
          ))}
        </div>
      )}
    </div>
  );
};

export default KitSearch;
