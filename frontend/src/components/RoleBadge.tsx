/**
 * RoleBadge - Displays organization role as a colored badge
 */

import type { OrgRole } from '@/types/org';

interface RoleBadgeProps {
  role: OrgRole;
  className?: string;
}

const ROLE_STYLES: Record<OrgRole, { bg: string; text: string; label: string }> = {
  owner: { bg: 'bg-purple-100', text: 'text-purple-800', label: 'Owner' },
  admin: { bg: 'bg-red-100', text: 'text-red-800', label: 'Admin' },
  manager: { bg: 'bg-blue-100', text: 'text-blue-800', label: 'Manager' },
  operator: { bg: 'bg-green-100', text: 'text-green-800', label: 'Operator' },
  viewer: { bg: 'bg-gray-100', text: 'text-gray-800', label: 'Viewer' },
};

export function RoleBadge({ role, className = '' }: RoleBadgeProps) {
  const style = ROLE_STYLES[role] || ROLE_STYLES.viewer;

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${style.bg} ${style.text} ${className}`}
    >
      {style.label}
    </span>
  );
}
