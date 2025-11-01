/**
 * Avatar - Displays user initials in a circular badge
 */

interface AvatarProps {
  email: string;
  className?: string;
}

export function Avatar({ email, className = '' }: AvatarProps) {
  const initials = getInitials(email);

  return (
    <div className={`flex items-center justify-center w-8 h-8 rounded-full bg-blue-600 text-white font-semibold text-sm ${className}`}>
      {initials}
    </div>
  );
}

function getInitials(email: string): string {
  // Extract username before @
  const username = email.split('@')[0];

  // Split by common separators (., _, -)
  const parts = username.split(/[._-]/);

  // If we have multiple parts, take first letter of first 2 parts
  if (parts.length >= 2) {
    return parts
      .slice(0, 2)
      .map(part => part[0]?.toUpperCase() || '')
      .join('');
  }

  // Fallback to first 2 chars of username
  return username.slice(0, 2).toUpperCase();
}
