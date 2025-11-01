/* eslint-disable react/prop-types */
import React from 'react';
import type { LucideIcon } from 'lucide-react';

export interface FloatingActionButtonProps {
  icon: LucideIcon;
  onClick: () => void;
  ariaLabel: string;
  position?: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';
  variant?: 'primary' | 'success' | 'danger' | 'secondary';
  size?: 'small' | 'medium' | 'large';
  disabled?: boolean;
  className?: string;
}

const POSITION_CLASSES = {
  'bottom-right': 'bottom-6 right-6',
  'bottom-left': 'bottom-6 left-6',
  'top-right': 'top-6 right-6',
  'top-left': 'top-6 left-6',
} as const;

const VARIANT_CLASSES = {
  primary: 'bg-blue-500 hover:bg-blue-600 active:bg-blue-700 text-white',
  success: 'bg-green-500 hover:bg-green-600 active:bg-green-700 text-white',
  danger: 'bg-red-500 hover:bg-red-600 active:bg-red-700 text-white',
  secondary: 'bg-gray-500 hover:bg-gray-600 active:bg-gray-700 text-white',
} as const;

const SIZE_CLASSES = {
  small: 'w-12 h-12',
  medium: 'w-14 h-14',
  large: 'w-16 h-16',
} as const;

const ICON_SIZE_MAP = {
  small: 20,
  medium: 24,
  large: 28,
} as const;

export const FloatingActionButton: React.FC<FloatingActionButtonProps> = React.memo(({
  icon: Icon,
  onClick,
  ariaLabel,
  position = 'bottom-right',
  variant = 'primary',
  size = 'medium',
  disabled = false,
  className = '',
}) => {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      className={`
        fixed
        ${POSITION_CLASSES[position]}
        ${SIZE_CLASSES[size]}
        ${disabled ? 'opacity-50 cursor-not-allowed' : VARIANT_CLASSES[variant]}
        rounded-full
        shadow-lg
        flex items-center justify-center
        transition-all duration-200
        focus:outline-none focus:ring-4 focus:ring-offset-2
        ${disabled ? '' : 'hover:scale-110 active:scale-95'}
        z-50
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      <Icon size={ICON_SIZE_MAP[size]} />
    </button>
  );
});

FloatingActionButton.displayName = 'FloatingActionButton';
