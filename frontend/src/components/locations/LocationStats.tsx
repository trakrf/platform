import { useMemo } from 'react';
import { Building2, BarChart3, CheckCircle, XCircle } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { StatCard } from '@/components/assets/StatCard';

interface LocationStatsProps {
  className?: string;
}

export function LocationStats({ className = '' }: LocationStatsProps) {
  const cache = useLocationStore((state) => state.cache);

  const stats = useMemo(() => {
    const locations = Array.from(cache.byId.values());
    const total = locations.length;
    const active = locations.filter((l) => l.is_active).length;
    const inactive = total - active;
    const roots = cache.rootIds.size;

    return { total, active, inactive, roots };
  }, [cache.byId.size, cache.rootIds.size]);

  return (
    <div className={className}>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-2 md:gap-3">
        <StatCard
          icon={BarChart3}
          label="Total Locations"
          value={stats.total}
          subtitle="All registered"
          variant="blue"
        />
        <StatCard
          icon={CheckCircle}
          label="Active"
          value={stats.active}
          subtitle="In use"
          variant="green"
        />
        <StatCard
          icon={XCircle}
          label="Inactive"
          value={stats.inactive}
          subtitle="Not in use"
          variant="gray"
        />
        <StatCard
          icon={Building2}
          label="Top Level"
          value={stats.roots}
          subtitle="Top level"
          variant="blue"
        />
      </div>
    </div>
  );
}
