import { useMemo } from 'react';
import { User, Laptop, Package, Archive, HelpCircle, BarChart3, CheckCircle, XCircle } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { StatCard } from './StatCard';
import { TypeCard } from './TypeCard';
import type { AssetType } from '@/types/assets';

interface AssetStatsProps {
  className?: string;
}

const TYPE_INFO = {
  person: { icon: User, label: 'People' },
  device: { icon: Laptop, label: 'Devices' },
  asset: { icon: Package, label: 'Assets' },
  inventory: { icon: Archive, label: 'Inventory' },
  other: { icon: HelpCircle, label: 'Other' },
} as const;

export function AssetStats({ className = '' }: AssetStatsProps) {
  const cache = useAssetStore((state) => state.cache);

  const stats = useMemo(() => {
    const assets = Array.from(cache.byId.values());
    const total = assets.length;
    const active = assets.filter((a) => a.is_active).length;
    const inactive = total - active;

    const byType = assets.reduce(
      (acc, asset) => {
        acc[asset.type] = (acc[asset.type] || 0) + 1;
        return acc;
      },
      {} as Record<AssetType, number>
    );

    const typeStats = (Object.keys(TYPE_INFO) as AssetType[]).map((type) => ({
      type,
      count: byType[type] || 0,
      percentage: total > 0 ? Math.round(((byType[type] || 0) / total) * 100) : 0,
    }));

    return { total, active, inactive, typeStats };
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

      {/* By Type Breakdown */}
      <div>
        <h3 className="text-base font-semibold text-gray-900 dark:text-white mb-3">
          By Type
        </h3>
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-2 md:gap-3">
          {stats.typeStats.map(({ type, count, percentage }) => {
            const { icon, label } = TYPE_INFO[type];
            return (
              <TypeCard
                key={type}
                icon={icon}
                label={label}
                count={count}
                percentage={percentage}
              />
            );
          })}
        </div>
      </div>
    </div>
  );
}
