/* eslint-disable react/prop-types */
import React from 'react';
import type { LucideIcon } from 'lucide-react';

export interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: {
    label: string;
    onClick: () => void;
  };
  variant?: 'default' | 'info' | 'warning';
  className?: string;
}

const VARIANT_CLASSES = {
  default: {
    container: 'bg-gray-50 dark:bg-gray-900/20 border-gray-200 dark:border-gray-700',
    icon: 'text-gray-400 dark:text-gray-500',
    title: 'text-gray-900 dark:text-gray-100',
    description: 'text-gray-600 dark:text-gray-400',
  },
  info: {
    container: 'bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800',
    icon: 'text-blue-500 dark:text-blue-400',
    title: 'text-blue-900 dark:text-blue-100',
    description: 'text-blue-700 dark:text-blue-300',
  },
  warning: {
    container: 'bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800',
    icon: 'text-yellow-500 dark:text-yellow-400',
    title: 'text-yellow-900 dark:text-yellow-100',
    description: 'text-yellow-700 dark:text-yellow-300',
  },
} as const;

export const EmptyState: React.FC<EmptyStateProps> = React.memo(({
  icon: Icon,
  title,
  description,
  action,
  variant = 'default',
  className = '',
}) => {
  const styles = VARIANT_CLASSES[variant];

  return (
    <div
      className={`
        ${styles.container}
        border rounded-lg p-8 md:p-12
        flex flex-col items-center justify-center text-center
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      {Icon && (
        <Icon
          size={48}
          className={`${styles.icon} mb-4`}
          aria-hidden="true"
        />
      )}

      <h3 className={`text-lg font-semibold mb-2 ${styles.title}`}>
        {title}
      </h3>

      {description && (
        <p className={`text-sm max-w-md mb-6 ${styles.description}`}>
          {description}
        </p>
      )}

      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="px-4 py-2 bg-blue-500 hover:bg-blue-600 active:bg-blue-700 text-white rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
        >
          {action.label}
        </button>
      )}
    </div>
  );
});

EmptyState.displayName = 'EmptyState';
