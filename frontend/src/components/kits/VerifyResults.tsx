import React from 'react';
import { AlertTriangle, CheckCircle2, ChevronDown, Target, XCircle } from 'lucide-react';
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

/**
 * One tag row: "role: EPC" with the Scan tab's Target-icon Locate button.
 * No generated asset names — the role + tag number is the identity.
 */
export const TagRow: React.FC<{
  epc: string;
  onLocate: (epc: string) => void;
  label?: string | null;
  onRed?: boolean;
  icon?: React.ReactNode;
}> = ({ epc, onLocate, label, onRed, icon }) => (
  <div className="flex items-center justify-between gap-2 py-1">
    <span
      className={`flex items-center min-w-0 text-sm ${onRed ? 'text-red-100' : 'text-gray-700 dark:text-gray-300'}`}
    >
      {icon}
      {label && <span className="font-medium capitalize mr-1.5">{label}:</span>}
      <span className="font-mono truncate">{epc}</span>
    </span>
    <button
      data-testid={`kit-locate-${epc}`}
      onClick={() => onLocate(epc)}
      className={`flex items-center flex-shrink-0 text-sm font-medium transition-colors ${
        onRed ? 'text-white hover:text-red-200' : 'text-blue-600 hover:text-blue-800'
      }`}
    >
      <Target className="w-4 h-4 mr-1" />
      Locate
    </button>
  </div>
);

const MissingMemberNode: React.FC<{
  member: VerifyMissingMember;
  onLocate: (epc: string) => void;
}> = ({ member, onLocate }) => (
  <div data-testid={`kit-missing-${member.asset_id}`} className="py-1 border-t border-red-400/60">
    {member.epcs.length === 0 && (
      <div className="flex items-center py-1 text-sm font-semibold">
        <XCircle className="w-4 h-4 mr-2 flex-shrink-0" />
        <span className="font-medium capitalize mr-1.5">{member.role ?? 'tag'}:</span>
        no tags registered
      </div>
    )}
    {member.epcs.map((epc) => (
      <TagRow
        key={epc}
        epc={epc}
        onLocate={onLocate}
        label={`missing ${member.role ?? 'tag'}`}
        onRed
        icon={<XCircle className="w-4 h-4 mr-2 flex-shrink-0" />}
      />
    ))}
  </div>
);

const SeenMemberNode: React.FC<{
  member: VerifySeenMember;
  onLocate: (epc: string) => void;
  onRed?: boolean;
}> = ({ member, onLocate, onRed }) => (
  <div
    className={`py-1 ${onRed ? 'border-t border-red-400/60' : 'border-t border-green-200 dark:border-green-800'}`}
  >
    {(member.epcs ?? []).map((epc) => (
      <TagRow
        key={epc}
        epc={epc}
        onLocate={onLocate}
        label={member.role ?? 'tag'}
        onRed={onRed}
        icon={
          <CheckCircle2
            className={`w-4 h-4 mr-2 flex-shrink-0 ${onRed ? '' : 'text-green-600 dark:text-green-400'}`}
          />
        }
      />
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
          <div className="text-xl font-extrabold tracking-wide uppercase">Invalid pair</div>
          <div className="text-red-100 font-semibold">
            Lot {kit.label} — {kit.seen.length} of {kit.seen.length + kit.missing.length} present
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
        <span className="flex-1">Lot {kit.label} — valid pair</span>
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
            Wrong-pair items in this scan
          </div>
          <div className="mt-1">
            {result.unexpected.map((u) => (
              <div key={`${u.asset_id}-${u.epc}`} className="border-t border-amber-300 dark:border-amber-800">
                <TagRow
                  epc={u.epc}
                  onLocate={onLocate}
                  label={`from Lot ${u.belongs_to_kit_label}`}
                />
              </div>
            ))}
          </div>
        </div>
      )}

      {complete.map((kit) => (
        <CompleteKit key={kit.kit_id} kit={kit} onLocate={onLocate} />
      ))}

    </div>
  );
};

export default VerifyResults;
