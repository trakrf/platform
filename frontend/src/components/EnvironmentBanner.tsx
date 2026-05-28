import { useEffect } from 'react';
import { getAppConfig, isNonProd } from '@/lib/appConfig';

export function EnvironmentBanner() {
  const env = getAppConfig().environmentLabel;
  const showBanner = isNonProd(env);

  // Update page title with environment prefix
  useEffect(() => {
    if (!showBanner) return;

    const baseTitle = 'TrakRF';
    const prefix = env.toUpperCase().slice(0, 3);
    document.title = `[${prefix}] ${baseTitle}`;

    return () => {
      document.title = baseTitle;
    };
  }, [env, showBanner]);

  if (!showBanner) return null;

  return (
    <div
      className="bg-purple-600 text-white text-center text-sm py-1 font-medium"
      data-testid="environment-banner"
    >
      {env.charAt(0).toUpperCase() + env.slice(1)} Environment
    </div>
  );
}
