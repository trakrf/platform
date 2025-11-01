/* eslint-disable react/prop-types */
import React from 'react';

export interface ContainerProps {
  children: React.ReactNode;
  variant?: 'white' | 'gray' | 'transparent';
  padding?: 'none' | 'small' | 'medium' | 'large';
  border?: boolean;
  rounded?: boolean;
  className?: string;
}

const VARIANT_CLASSES = {
  white: 'bg-white dark:bg-gray-800',
  gray: 'bg-gray-50 dark:bg-gray-900/20',
  transparent: 'bg-transparent',
} as const;

const PADDING_CLASSES = {
  none: '',
  small: 'p-3',
  medium: 'p-4 md:p-6',
  large: 'p-6 md:p-8',
} as const;

export const Container: React.FC<ContainerProps> = React.memo(({
  children,
  variant = 'white',
  padding = 'medium',
  border = true,
  rounded = true,
  className = '',
}) => {
  return (
    <div
      className={`
        ${VARIANT_CLASSES[variant]}
        ${PADDING_CLASSES[padding]}
        ${border ? 'border border-gray-200 dark:border-gray-700' : ''}
        ${rounded ? 'rounded-lg' : ''}
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      {children}
    </div>
  );
});

Container.displayName = 'Container';
