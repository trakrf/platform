interface ProgressBarProps {
  value: number;
  max: number;
  variant?: 'blue' | 'green' | 'yellow' | 'red';
  className?: string;
}

export function ProgressBar({ value, max, variant = 'blue', className = '' }: ProgressBarProps) {
  const percentage = Math.min(100, Math.max(0, (value / max) * 100));

  const variantClasses = {
    blue: 'bg-blue-500',
    green: 'bg-green-500',
    yellow: 'bg-yellow-500',
    red: 'bg-red-500',
  };

  return (
    <div
      className={`w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden ${className}`}
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-label={`Progress: ${percentage.toFixed(0)}%`}
    >
      <div
        className={`h-full ${variantClasses[variant]} transition-all duration-300 ease-out`}
        style={{ width: `${percentage}%` }}
      />
    </div>
  );
}
