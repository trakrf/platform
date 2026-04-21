import type { Scope } from '@/types/apiKey';

type ResourceLevel = 'none' | 'read' | 'readwrite';

interface Props {
  value: Scope[];
  onChange: (next: Scope[]) => void;
}

type ResourceKey = 'assets' | 'locations' | 'scans';

const RESOURCES: { key: ResourceKey; label: string; hasWrite: boolean }[] = [
  { key: 'assets',    label: 'Assets',    hasWrite: true },
  { key: 'locations', label: 'Locations', hasWrite: true },
  { key: 'scans',     label: 'Scans',     hasWrite: true },
];

function levelFor(resource: ResourceKey, scopes: Scope[]): ResourceLevel {
  const read = scopes.includes(`${resource}:read` as Scope);
  const write = scopes.includes(`${resource}:write` as Scope);
  if (read && write) return 'readwrite';
  if (read) return 'read';
  return 'none';
}

function scopesFor(resource: ResourceKey, level: ResourceLevel): Scope[] {
  if (level === 'none') return [];
  if (level === 'read') return [`${resource}:read` as Scope];
  return [`${resource}:read` as Scope, `${resource}:write` as Scope];
}

export function ScopeSelector({ value, onChange }: Props) {
  const setLevel = (resource: ResourceKey, level: ResourceLevel) => {
    const without = value.filter(
      (s) => !s.startsWith(`${resource}:`),
    );
    onChange([...without, ...scopesFor(resource, level)]);
  };

  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Permissions
      </legend>
      {RESOURCES.map((r) => {
        const current = levelFor(r.key, value);
        return (
          <div key={r.key} className="flex items-center gap-3">
            <label
              htmlFor={`scope-${r.key}`}
              className="w-24 text-sm text-gray-800 dark:text-gray-200"
            >
              {r.label}
            </label>
            <select
              id={`scope-${r.key}`}
              aria-label={r.label}
              value={current}
              onChange={(e) => setLevel(r.key, e.target.value as ResourceLevel)}
              className="border rounded px-2 py-1 text-sm bg-white dark:bg-gray-800"
            >
              <option value="none">None</option>
              <option value="read">Read</option>
              {r.hasWrite && <option value="readwrite">Read + Write</option>}
            </select>
          </div>
        );
      })}
    </fieldset>
  );
}
