import { LucideIcon } from 'lucide-react';

interface TypeCardProps {
  icon: LucideIcon;
  label: string;
  count: number;
  percentage: number;
}

export function TypeCard({ icon: Icon, label, count, percentage }: TypeCardProps) {
  const isActive = count > 0;

  return (
    <div
      className={`
        relative overflow-hidden rounded-lg border transition-all duration-200
        ${isActive
          ? 'bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-900 border-gray-200 dark:border-gray-700 hover:shadow-md hover:scale-105'
          : 'bg-gray-50 dark:bg-gray-900/50 border-gray-200 dark:border-gray-800 opacity-60'
        }
      `}
    >
      {/* Background Pattern */}
      <div className="absolute top-0 right-0 w-20 h-20 opacity-5">
        <Icon className="w-full h-full" />
      </div>

      {/* Content */}
      <div className="relative p-3 md:p-4">
        <div className="flex items-center gap-2 mb-2">
          <div className={`
            p-1.5 rounded-lg
            ${isActive
              ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
              : 'bg-gray-200 dark:bg-gray-800 text-gray-400 dark:text-gray-600'
            }
          `}>
            <Icon className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          </div>
          <span className={`
            text-xs sm:text-sm font-semibold truncate
            ${isActive
              ? 'text-gray-900 dark:text-white'
              : 'text-gray-500 dark:text-gray-600'
            }
          `}>
            {label}
          </span>
        </div>

        <div className="flex items-baseline gap-2">
          <span className={`
            text-xl sm:text-2xl font-bold
            ${isActive
              ? 'text-gray-900 dark:text-white'
              : 'text-gray-400 dark:text-gray-600'
            }
          `}>
            {count}
          </span>
          {isActive && (
            <span className="text-xs text-gray-500 dark:text-gray-400">
              {percentage}%
            </span>
          )}
        </div>

        {/* Progress indicator */}
        {isActive && percentage > 0 && (
          <div className="mt-2 w-full bg-gray-200 dark:bg-gray-700 rounded-full h-1">
            <div
              className="bg-blue-500 dark:bg-blue-400 h-1 rounded-full transition-all duration-500"
              style={{ width: `${percentage}%` }}
            />
          </div>
        )}
      </div>
    </div>
  );
}
