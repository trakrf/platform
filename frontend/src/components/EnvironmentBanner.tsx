import { useEffect } from 'react';

type Environment = 'dev' | 'staging' | 'prod';

interface EnvConfig {
  label: string;
  titlePrefix: string;
  bgColor: string;
}

const ENV_CONFIG: Record<Environment, EnvConfig | null> = {
  dev: {
    label: 'Development Environment',
    titlePrefix: '[DEV]',
    bgColor: 'bg-orange-500',
  },
  staging: {
    label: 'Staging Environment',
    titlePrefix: '[STG]',
    bgColor: 'bg-purple-600',
  },
  prod: null,
};

function getEnvironment(): Environment {
  const env = import.meta.env.VITE_ENVIRONMENT;
  if (env === 'dev' || env === 'staging') return env;
  return 'prod'; // Default to prod (shows nothing)
}

export function EnvironmentBanner() {
  const environment = getEnvironment();
  const config = ENV_CONFIG[environment];

  // Update page title with environment prefix
  useEffect(() => {
    if (!config) return;

    const baseTitle = 'TrakRF';
    document.title = `${config.titlePrefix} ${baseTitle}`;

    return () => {
      document.title = baseTitle;
    };
  }, [config]);

  if (!config) return null;

  return (
    <div
      className={`${config.bgColor} text-white text-center text-sm py-1 font-medium`}
      data-testid="environment-banner"
    >
      {config.label}
    </div>
  );
}
