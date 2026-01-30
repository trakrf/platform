import type { LucideIcon } from 'lucide-react';

interface ReportStatCardProps {
  title: string;
  value: number;
  subtitle?: string;
  icon: LucideIcon;
  iconColor?: string;
  iconBgColor?: string;
  onClick?: () => void;
}

export function ReportStatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  iconColor = 'text-blue-500',
  iconBgColor = 'bg-blue-500/10',
  onClick,
}: ReportStatCardProps) {
  const Wrapper = onClick ? 'button' : 'div';

  return (
    <Wrapper
      onClick={onClick}
      className={`
        bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700
        p-3 md:p-4 flex items-center justify-between gap-2
        ${onClick ? 'cursor-pointer hover:border-blue-500 dark:hover:border-blue-400 transition-colors' : ''}
      `}
    >
      <div className="min-w-0 flex-1">
        <p className="text-xs md:text-sm text-gray-500 dark:text-gray-400 truncate">{title}</p>
        <p className="text-2xl md:text-3xl font-bold text-gray-900 dark:text-white mt-0.5 md:mt-1">
          {value.toLocaleString()}
        </p>
        {subtitle && (
          <p className="text-xs md:text-sm text-green-600 dark:text-green-400 mt-0.5 md:mt-1 truncate">
            {subtitle}
          </p>
        )}
      </div>
      <div className={`w-10 h-10 md:w-12 md:h-12 rounded-xl ${iconBgColor} flex items-center justify-center flex-shrink-0`}>
        <Icon className={`w-5 h-5 md:w-6 md:h-6 ${iconColor}`} />
      </div>
    </Wrapper>
  );
}
