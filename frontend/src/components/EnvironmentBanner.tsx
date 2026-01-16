import { useEffect } from 'react';

export function EnvironmentBanner() {
  const env = import.meta.env.VITE_ENVIRONMENT as string | undefined;

  // No banner for prod/production, empty, or undefined
  const isNonProd = typeof env === 'string' && env.length > 0 && env !== 'prod' && env !== 'production' && env !== 'undefined';

  // Update page title with environment prefix
  useEffect(() => {
    if (!isNonProd) return;

    const baseTitle = 'TrakRF';
    const prefix = env.toUpperCase().slice(0, 3);
    document.title = `[${prefix}] ${baseTitle}`;

    return () => {
      document.title = baseTitle;
    };
  }, [env, isNonProd]);

  if (!isNonProd) return null;

  return (
    <div
      className="bg-purple-600 text-white text-center text-sm py-1 font-medium"
      data-testid="environment-banner"
    >
      {env.charAt(0).toUpperCase() + env.slice(1)} Environment
    </div>
  );
}
