/**
 * SuperadminOrgsScreen — superadmin-only all-orgs list (TRA-949).
 *
 * A flat list of every organization with its entitlement state and member
 * count. Each row links into the existing org edit screen (#org-settings?org=id)
 * where a superadmin can toggle entitlement, including for non-member orgs.
 *
 * The backend gates GET /admin/orgs strictly on is_superadmin (403 otherwise);
 * this screen is reached from a superadmin-only nav entry.
 */

import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Search } from "lucide-react";
import { orgsApi } from "@/lib/api/orgs";
import { extractErrorMessage } from "@/lib/asset/helpers";
import type { AdminOrgListItem } from "@/types/org";

function formatExpiry(iso?: string | null): string {
  if (!iso) return "Never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export default function SuperadminOrgsScreen() {
  const [orgs, setOrgs] = useState<AdminOrgListItem[]>([]);
  const [filter, setFilter] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Client-side live filter: the list is loaded in full (fine for the hundreds
  // of orgs we see today). If this ever grows to thousands, push the filter to
  // the backend as a ?q= query param on GET /admin/orgs instead.
  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return orgs;
    return orgs.filter((o) => o.name.toLowerCase().includes(q));
  }, [orgs, filter]);

  useEffect(() => {
    let active = true;
    setIsLoading(true);
    setError(null);
    orgsApi
      .listAllOrgs()
      .then((res) => {
        if (active) setOrgs(res.data.data ?? []);
      })
      .catch((err: unknown) => {
        if (active)
          setError(extractErrorMessage(err, "Failed to load organizations"));
      })
      .finally(() => {
        if (active) setIsLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="min-h-screen bg-gray-900 p-4">
      <div className="max-w-4xl mx-auto">
        <div className="flex items-center gap-4 mb-6">
          <a
            href="#scan"
            className="text-gray-400 hover:text-gray-300 transition-colors"
            aria-label="Go home"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">
            All Organizations
          </h1>
        </div>

        {error && (
          <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-6">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {isLoading ? (
          <p className="text-gray-400">Loading organizations…</p>
        ) : orgs.length === 0 ? (
          <p className="text-gray-400">No organizations found.</p>
        ) : (
          <>
            <div className="relative mb-4">
              <Search className="w-4 h-4 text-gray-500 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              <input
                type="text"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                aria-label="Filter organizations by name"
                placeholder="Filter by name…"
                className="w-full pl-9 pr-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              />
            </div>

            {filtered.length === 0 ? (
              <p className="text-gray-400">
                No organizations match “{filter}”.
              </p>
            ) : (
              <div className="overflow-x-auto rounded-lg border border-gray-700">
                <table className="w-full text-sm text-left text-gray-300">
                  <thead className="text-xs uppercase text-gray-400 bg-gray-800">
                    <tr>
                      <th scope="col" className="px-4 py-3">
                        Name
                      </th>
                      <th scope="col" className="px-4 py-3">
                        Entitled
                      </th>
                      <th scope="col" className="px-4 py-3">
                        Expires
                      </th>
                      <th scope="col" className="px-4 py-3 text-right">
                        Members
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((org) => (
                      <tr
                        key={org.id}
                        className="border-t border-gray-700 hover:bg-gray-800/50"
                      >
                        <td className="px-4 py-3">
                          <a
                            href={`#org-settings?org=${org.id}`}
                            className="text-blue-400 hover:text-blue-300 font-medium"
                          >
                            {org.name}
                          </a>
                        </td>
                        <td className="px-4 py-3">
                          {org.subscription_enabled ? (
                            <span className="text-green-400">Enabled</span>
                          ) : (
                            <span className="text-gray-500">Disabled</span>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          {formatExpiry(org.subscription_expires_at)}
                        </td>
                        <td className="px-4 py-3 text-right">
                          {org.member_count}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
