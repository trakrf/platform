import React from 'react';
import { AlertTriangle, CheckCircle2, ChevronDown, Search, XCircle } from 'lucide-react';
import type {
  VerifyResponse,
  VerifyKitResult,
  VerifySeenMember,
  VerifyMissingMember,
} from '@/lib/api/kits';

interface VerifyResultsProps {
  result: VerifyResponse;
  onLocate: (epc: string) => void;
}

const memberLabel = (role: string | null, name: string) =>
  role ? `${name} (${role})` : name;

/** Display order + labels for the QA fields; unknown keys render verbatim. */
const QA_FIELD_LABELS: Record<string, string> = {
  part: 'Part #',
  heat: 'Heat #',
  operator: 'Operator',
  date: 'Date',
  vendor: 'Vendor',
};
const QA_FIELD_ORDER = ['part', 'heat', 'operator', 'date', 'vendor'];

/** The kit's QA details (Lot metadata), rendered inside a lot node. */
export const KitMetadataRows: React.FC<{ metadata: Record<string, string>; onRed?: boolean }> = ({
  metadata,
  onRed,
}) => {
  const keys = [
    ...QA_FIELD_ORDER.filter((k) => metadata[k]),
    ...Object.keys(metadata).filter((k) => !QA_FIELD_ORDER.includes(k) && metadata[k]),
  ];
  if (keys.length === 0) return null;
  return (
    <div
      data-testid="kit-metadata"
      className={`mb-1 grid grid-cols-2 gap-x-4 text-sm ${onRed ? 'text-red-100' : 'text-gray-600 dark:text-gray-400'}`}
    >
      {keys.map((k) => (
        <div key={k}>
          <span className="font-medium">{QA_FIELD_LABELS[k] ?? k}:</span> {metadata[k]}
        </div>
      ))}
    </div>
  );
};

/** One tag (EPC) leaf row — Locate pushes exactly this EPC. */
export const TagRow: React.FC<{
  epc: string;
  onLocate: (epc: string) => void;
  onRed?: boolean;
}> = ({ epc, onLocate, onRed }) => (
  <div className="flex items-center justify-between gap-2 py-1 pl-6">
    <span className={`font-mono text-sm truncate ${onRed ? 'text-red-100' : 'text-gray-700 dark:text-gray-300'}`}>
      {epc}
    </span>
    <button
      data-testid={`kit-locate-${epc}`}
      onClick={() => onLocate(epc)}
      className={`flex items-center flex-shrink-0 px-3 py-1 rounded font-semibold text-sm transition-colors ${
        onRed
          ? 'bg-white text-red-700 hover:bg-red-50'
          : 'bg-blue-600 text-white hover:bg-blue-700'
      }`}
    >
      <Search className="w-3.5 h-3.5 mr-1.5" />
      Locate
    </button>
  </div>
);

const MissingMemberNode: React.FC<{
  member: VerifyMissingMember;
  onLocate: (epc: string) => void;
}> = ({ member, onLocate }) => (
  <div data-testid={`kit-missing-${member.asset_id}`} className="py-1.5 border-t border-red-400/60">
    <div className="flex items-center font-semibold">
      <XCircle className="w-4 h-4 mr-2 flex-shrink-0" />
      {memberLabel(member.role, member.name)} — missing
    </div>
    {member.epcs.length === 0 && (
      <div className="pl-6 text-sm text-red-100">no tags registered</div>
    )}
    {member.epcs.map((epc) => (
      <TagRow key={epc} epc={epc} onLocate={onLocate} onRed />
    ))}
  </div>
);

const SeenMemberNode: React.FC<{
  member: VerifySeenMember;
  onLocate: (epc: string) => void;
  onRed?: boolean;
}> = ({ member, onLocate, onRed }) => (
  <div
    className={`py-1.5 ${onRed ? 'border-t border-red-400/60' : 'border-t border-green-200 dark:border-green-800'}`}
  >
    <div className={`flex items-center text-sm font-medium ${onRed ? 'text-red-100' : 'text-green-800 dark:text-green-300'}`}>
      <CheckCircle2 className="w-4 h-4 mr-2 flex-shrink-0" />
      {memberLabel(member.role, member.name)}
    </div>
    {(member.epcs ?? []).map((epc) => (
      <TagRow key={epc} epc={epc} onLocate={onLocate} onRed={onRed} />
    ))}
  </div>
);

const IncompleteKit: React.FC<{ kit: VerifyKitResult; onLocate: (epc: string) => void }> = ({
  kit,
  onLocate,
}) => (
  <details
    data-testid={`kit-result-incomplete-${kit.kit_id}`}
    role="alert"
    open
    className="w-full rounded-lg bg-red-600 text-white p-4 shadow-lg"
  >
    <summary className="cursor-pointer list-none">
      <div className="flex items-center">
        <AlertTriangle className="w-8 h-8 mr-3 animate-pulse flex-shrink-0" />
        <div className="flex-1">
          <div className="text-xl font-extrabold tracking-wide uppercase">Kit incomplete</div>
          <div className="text-red-100 font-semibold">
            Lot {kit.label} — {kit.seen.length} of {kit.seen.length + kit.missing.length} members present
          </div>
        </div>
        <ChevronDown className="w-5 h-5 flex-shrink-0" />
      </div>
    </summary>
    <div className="mt-2">
      <KitMetadataRows metadata={kit.metadata ?? {}} onRed />
      {kit.missing.map((m) => (
        <MissingMemberNode key={m.asset_id} member={m} onLocate={onLocate} />
      ))}
      {kit.seen.map((m) => (
        <SeenMemberNode key={m.asset_id} member={m} onLocate={onLocate} onRed />
      ))}
    </div>
  </details>
);

const CompleteKit: React.FC<{ kit: VerifyKitResult; onLocate: (epc: string) => void }> = ({
  kit,
  onLocate,
}) => (
  <details
    data-testid={`kit-result-complete-${kit.kit_id}`}
    className="w-full rounded-lg border border-green-300 dark:border-green-800 bg-green-50 dark:bg-green-900/20 p-4"
  >
    <summary className="cursor-pointer list-none">
      <div className="flex items-center text-green-800 dark:text-green-300 font-semibold">
        <CheckCircle2 className="w-5 h-5 mr-2 flex-shrink-0" />
        <span className="flex-1">
          Lot {kit.label} — complete · {kit.seen.length} member{kit.seen.length === 1 ? '' : 's'}
        </span>
        <ChevronDown className="w-5 h-5 flex-shrink-0" />
      </div>
    </summary>
    <div className="mt-2">
      <KitMetadataRows metadata={kit.metadata ?? {}} />
      {kit.seen.map((m) => (
        <SeenMemberNode key={m.asset_id} member={m} onLocate={onLocate} />
      ))}
    </div>
  </details>
);

/**
 * Renders a dock-check verify response as a tree (TRA-1033): one node per
 * detected lot (exceptions loud and first, expanded; complete lots collapsed),
 * members under each lot, and the scanned tag values (EPCs) as leaf rows —
 * every tag row carries a Locate button that arms exactly that EPC.
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
          className="w-full rounded-lg border-2 border-amber-500 bg-amber-100 dark:bg-amber-900/30 p-4 text-amber-900 dark:text-amber-200"
        >
          <div className="flex items-center font-bold dark:text-amber-300">
            <AlertTriangle className="w-5 h-5 mr-2 flex-shrink-0" />
            Wrong-kit items in this scan
          </div>
          <div className="mt-1">
            {result.unexpected.map((u) => (
              <div key={`${u.asset_id}-${u.epc}`} className="py-1 border-t border-amber-300 dark:border-amber-800">
                <div className="text-sm font-medium">
                  {u.name} belongs to Lot {u.belongs_to_kit_label}
                </div>
                <TagRow epc={u.epc} onLocate={onLocate} />
              </div>
            ))}
          </div>
        </div>
      )}

      {complete.map((kit) => (
        <CompleteKit key={kit.kit_id} kit={kit} onLocate={onLocate} />
      ))}

      {result.kits.length === 0 && (
        <div className="text-sm text-gray-600 dark:text-gray-400">
          No kit members in this scan.
        </div>
      )}

      {result.unknown_epcs.length > 0 && (
        <div
          data-testid="kit-unknown-epcs"
          className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
        >
          <div className="text-sm font-medium text-gray-600 dark:text-gray-400">
            Unmatched tags ({result.unknown_epcs.length}) — not registered to any asset
          </div>
          <div className="mt-1 divide-y divide-gray-100 dark:divide-gray-700">
            {result.unknown_epcs.map((epc) => (
              <TagRow key={epc} epc={epc} onLocate={onLocate} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

export default VerifyResults;
