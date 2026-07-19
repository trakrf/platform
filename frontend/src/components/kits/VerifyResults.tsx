import React from 'react';
import { AlertTriangle, CheckCircle2, Search } from 'lucide-react';
import type { VerifyResponse, VerifyKitResult, VerifyMissingMember } from '@/lib/api/kits';

interface VerifyResultsProps {
  result: VerifyResponse;
  onLocate: (epc: string) => void;
}

const memberLabel = (role: string | null, name: string) =>
  role ? `${name} (${role})` : name;

const MissingRow: React.FC<{ member: VerifyMissingMember; onLocate: (epc: string) => void }> = ({
  member,
  onLocate,
}) => (
  <div className="flex items-center justify-between py-2 border-t border-red-400/60">
    <span className="font-semibold">{memberLabel(member.role, member.name)}</span>
    {member.epcs.length > 0 && (
      <button
        data-testid={`locate-missing-${member.asset_id}`}
        onClick={() => onLocate(member.epcs[0])}
        className="flex items-center px-4 py-2 rounded-lg bg-white text-red-700 font-bold hover:bg-red-50 transition-colors"
      >
        <Search className="w-4 h-4 mr-2" />
        Locate
      </button>
    )}
  </div>
);

const IncompleteKit: React.FC<{ kit: VerifyKitResult; onLocate: (epc: string) => void }> = ({
  kit,
  onLocate,
}) => (
  <div
    data-testid={`kit-result-incomplete-${kit.kit_id}`}
    role="alert"
    className="w-full rounded-lg bg-red-600 text-white p-4 shadow-lg"
  >
    <div className="flex items-center mb-1">
      <AlertTriangle className="w-8 h-8 mr-3 animate-pulse" />
      <div>
        <div className="text-xl font-extrabold tracking-wide uppercase">Kit incomplete</div>
        <div className="text-red-100 font-semibold">Kit {kit.label}</div>
      </div>
    </div>
    <div className="text-sm text-red-100 mb-2">
      {kit.seen.length} of {kit.seen.length + kit.missing.length} members present — missing:
    </div>
    {kit.missing.map((m) => (
      <MissingRow key={m.asset_id} member={m} onLocate={onLocate} />
    ))}
  </div>
);

const CompleteKit: React.FC<{ kit: VerifyKitResult }> = ({ kit }) => (
  <div
    data-testid={`kit-result-complete-${kit.kit_id}`}
    className="w-full rounded-lg border border-green-300 dark:border-green-800 bg-green-50 dark:bg-green-900/20 p-4"
  >
    <div className="flex items-center text-green-800 dark:text-green-300 font-semibold">
      <CheckCircle2 className="w-5 h-5 mr-2" />
      Kit {kit.label} — complete
    </div>
    <div className="mt-1 text-sm text-green-700 dark:text-green-400">
      {kit.seen.map((m) => memberLabel(m.role, m.name)).join(', ')}
    </div>
  </div>
);

/**
 * Renders a dock-check verify response (TRA-1033). Pure props-driven —
 * exceptions (incomplete, then cross-kit unexpected) render loud and first;
 * complete kits are understated; unknown EPCs collapse out of the way.
 */
const VerifyResults: React.FC<VerifyResultsProps> = ({ result, onLocate }) => {
  const incomplete = result.kits.filter((k) => k.result === 'incomplete');
  const complete = result.kits.filter((k) => k.result !== 'incomplete');

  return (
    <div className="space-y-3">
      {incomplete.map((kit) => (
        <IncompleteKit key={kit.kit_id} kit={kit} onLocate={onLocate} />
      ))}

      {result.unexpected.length > 0 && (
        <div
          data-testid="kit-unexpected"
          role="alert"
          className="w-full rounded-lg border-2 border-amber-500 bg-amber-100 dark:bg-amber-900/30 p-4"
        >
          <div className="flex items-center text-amber-900 dark:text-amber-300 font-bold">
            <AlertTriangle className="w-5 h-5 mr-2" />
            Wrong-kit items in this scan
          </div>
          <div className="mt-1 space-y-1 text-sm text-amber-900 dark:text-amber-200">
            {result.unexpected.map((u) => (
              <div key={`${u.asset_id}-${u.epc}`} className="font-medium">
                {u.name} belongs to kit {u.belongs_to_kit_label}
              </div>
            ))}
          </div>
        </div>
      )}

      {complete.map((kit) => (
        <CompleteKit key={kit.kit_id} kit={kit} />
      ))}

      {result.kits.length === 0 && (
        <div className="text-sm text-gray-600 dark:text-gray-400">
          No kit members in this scan.
        </div>
      )}

      {result.unknown_epcs.length > 0 && (
        <details
          data-testid="kit-unknown-epcs"
          className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-3 text-sm text-gray-600 dark:text-gray-400"
        >
          <summary className="cursor-pointer">
            {result.unknown_epcs.length} unknown tag{result.unknown_epcs.length === 1 ? '' : 's'} (not registered)
          </summary>
          <div className="mt-2 space-y-1 font-mono text-xs">
            {result.unknown_epcs.map((epc) => (
              <div key={epc}>{epc}</div>
            ))}
          </div>
        </details>
      )}
    </div>
  );
};

export default VerifyResults;
