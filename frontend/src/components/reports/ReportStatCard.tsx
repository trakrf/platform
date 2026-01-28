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
        p-4 flex items-center justify-between
        ${onClick ? 'cursor-pointer hover:border-blue-500 dark:hover:border-blue-400 transition-colors' : ''}
      `}
    >
      <div>
        <p className="text-sm text-gray-500 dark:text-gray-400">{title}</p>
        <p className="text-3xl font-bold text-gray-900 dark:text-white mt-1">
          {value.toLocaleString()}
        </p>
        {subtitle && (
          <p className="text-sm text-green-600 dark:text-green-400 mt-1">
            {subtitle}
          </p>
        )}
      </div>
      <div className={`w-12 h-12 rounded-xl ${iconBgColor} flex items-center justify-center`}>
        <Icon className={`w-6 h-6 ${iconColor}`} />
      </div>
    </Wrapper>
  );
}
