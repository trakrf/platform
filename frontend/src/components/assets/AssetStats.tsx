import React, { useMemo } from 'react';
import { User, Laptop, Package, Archive, HelpCircle } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { Container } from '@/components/shared';
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

    // Calculate percentages
    const typeStats = (Object.keys(TYPE_INFO) as AssetType[]).map((type) => ({
      type,
      count: byType[type] || 0,
      percentage: total > 0 ? Math.round(((byType[type] || 0) / total) * 100) : 0,
    }));

    return { total, active, inactive, typeStats };
  }, [cache.byId.size]);

  return (
    <div className={className}>
      {/* Summary Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <Container padding="small" border={true} rounded={true}>
          <div className="text-center">
            <p className="text-sm font-medium text-gray-600 dark:text-gray-400 mb-1">
              Total Assets
            </p>
            <p className="text-3xl font-bold text-gray-900 dark:text-white">
              {stats.total}
            </p>
          </div>
        </Container>

        <Container padding="small" border={true} rounded={true}>
          <div className="text-center">
            <p className="text-sm font-medium text-gray-600 dark:text-gray-400 mb-1">
              Active
            </p>
            <p className="text-3xl font-bold text-green-600 dark:text-green-400">
              {stats.active}
            </p>
          </div>
        </Container>

        <Container padding="small" border={true} rounded={true}>
          <div className="text-center">
            <p className="text-sm font-medium text-gray-600 dark:text-gray-400 mb-1">
              Inactive
            </p>
            <p className="text-3xl font-bold text-gray-600 dark:text-gray-400">
              {stats.inactive}
            </p>
          </div>
        </Container>
      </div>

      {/* By Type Breakdown */}
      <Container padding="small" border={true} rounded={true}>
        <h3 className="text-base font-semibold text-gray-900 dark:text-white mb-4">
          By Type
        </h3>
        <div className="space-y-3">
          {stats.typeStats.map(({ type, count, percentage }) => {
            const { icon: Icon, label } = TYPE_INFO[type];
            return (
              <div key={type} className="space-y-1">
                <div className="flex items-center justify-between text-sm">
                  <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 text-gray-500 dark:text-gray-400" />
                    <span className="font-medium text-gray-700 dark:text-gray-300">
                      {label}:
                    </span>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="text-gray-900 dark:text-white font-semibold">
                      {count}
                    </span>
                    <span className="text-gray-600 dark:text-gray-400 min-w-[3rem] text-right">
                      {percentage}%
                    </span>
                  </div>
                </div>
                <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
                  <div
                    className="bg-blue-600 dark:bg-blue-500 h-2 rounded-full transition-all duration-300"
                    style={{ width: `${percentage}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </Container>
    </div>
  );
}
