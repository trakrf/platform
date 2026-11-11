import { useQuery } from '@tanstack/react-query';
import { orgsApi } from '@/lib/api/orgs';
import { useOrgStore } from '@/stores';
import type { GeofenceDefaultsView } from '@/types/org';

/**
 * The current org's geofence tuning defaults (org tier + system tier). The
 * output-device form reads it to know whether a blank field falls back to an
 * org value — an egress output's Auto-off is required unless the org supplies
 * one. Undefined while loading or when no org is selected.
 */
export function useGeofenceDefaults(): GeofenceDefaultsView | undefined {
  const orgId = useOrgStore((s) => s.currentOrg?.id);
  const { data } = useQuery({
    queryKey: ['geofenceDefaults', orgId],
    queryFn: async () => (await orgsApi.getGeofenceDefaults(orgId as number)).data.data,
    enabled: orgId != null,
  });
  return data;
}
