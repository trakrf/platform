import { useMemo } from 'react';
import { BarChart3, CheckCircle, XCircle } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { StatCard } from './StatCard';

interface AssetStatsProps {
  className?: string;
}

export function AssetStats({ className = '' }: AssetStatsProps) {
  const cache = useAssetStore((state) => state.cache);

  const stats = useMemo(() => {
    const assets = Array.from(cache.byId.values());
    const total = assets.length;
    const active = assets.filter((a) => a.is_active).length;
    const inactive = total - active;

    return { total, active, inactive };
  }, [cache.byId.size]);

  return (
    <div className={className}>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-2 md:gap-3 mb-6">
        <StatCard
          icon={BarChart3}
          label="Total Assets"
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
      </div>
    </div>
  );
}
