import { XCircle } from 'lucide-react';

interface ErrorBannerProps {
  error: string | null;
}

export function ErrorBanner({ error }: ErrorBannerProps) {
  if (!error) return null;

  return (
    <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-3 flex items-center">
      <XCircle className="w-5 h-5 text-red-600 dark:text-red-400 mr-3" />
      <p className="text-red-800 dark:text-red-200">{error}</p>
    </div>
  );
}
