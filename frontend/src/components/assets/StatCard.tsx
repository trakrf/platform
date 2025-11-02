import { LucideIcon } from 'lucide-react';

interface StatCardProps {
  icon: LucideIcon;
  label: string;
  value: number;
  subtitle: string;
  variant: 'blue' | 'green' | 'gray' | 'red';
}

const VARIANT_STYLES = {
  blue: {
    bg: 'bg-blue-50 dark:bg-blue-900/20',
    border: 'border-blue-200 dark:border-blue-800',
    icon: 'text-blue-600',
    label: 'text-blue-800 dark:text-blue-200',
    value: 'text-blue-800 dark:text-blue-200',
    subtitle: 'text-blue-600 dark:text-blue-400',
  },
  green: {
    bg: 'bg-green-50 dark:bg-green-900/20',
    border: 'border-green-200 dark:border-green-800',
    icon: 'text-green-600',
    label: 'text-green-800 dark:text-green-200',
    value: 'text-green-800 dark:text-green-200',
    subtitle: 'text-green-600 dark:text-green-400',
  },
  gray: {
    bg: 'bg-gray-50 dark:bg-gray-900/20',
    border: 'border-gray-200 dark:border-gray-700',
    icon: 'text-gray-600',
    label: 'text-gray-800 dark:text-gray-200',
    value: 'text-gray-800 dark:text-gray-200',
    subtitle: 'text-gray-600 dark:text-gray-400',
  },
  red: {
    bg: 'bg-red-50 dark:bg-red-900/20',
    border: 'border-red-200 dark:border-red-800',
    icon: 'text-red-600',
    label: 'text-red-800 dark:text-red-200',
    value: 'text-red-800 dark:text-red-200',
    subtitle: 'text-red-600 dark:text-red-400',
  },
} as const;

export function StatCard({ icon: Icon, label, value, subtitle, variant }: StatCardProps) {
  const styles = VARIANT_STYLES[variant];

  return (
    <div className={`${styles.bg} border ${styles.border} rounded-lg p-2 md:p-3`}>
      <div className="flex items-center justify-between">
        <div className="w-full">
          <div className="flex items-center mb-0.5 sm:mb-1">
            <Icon className={`w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 ${styles.icon} mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0`} />
            <span className={`${styles.label} font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate`}>
              {label}
            </span>
          </div>
          <div className={`text-base sm:text-lg md:text-xl lg:text-2xl font-bold ${styles.value}`}>
            {value}
          </div>
          <div className={`${styles.subtitle} text-[10px] xs:text-xs lg:text-sm truncate`}>
            {subtitle}
          </div>
        </div>
      </div>
    </div>
  );
}
