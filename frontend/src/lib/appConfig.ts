// Runtime app config, published onto window.__APP_CONFIG__ by an inline script
// the backend injects into index.html at serve time (TRA-853). This keeps
// environment identity out of the build: one immutable bundle renders the
// correct banner in any environment based on the pod's ENVIRONMENT_LABEL.

export interface AppConfig {
  environmentLabel: string;
}

declare global {
  interface Window {
    __APP_CONFIG__?: {
      environmentLabel?: string;
    };
  }
}

// Reads the injected config fresh each call. Defaults to an empty label when
// the global is absent (local dev, or index.html served without the backend).
export function getAppConfig(): AppConfig {
  const raw = typeof window !== 'undefined' ? window.__APP_CONFIG__ : undefined;
  return {
    environmentLabel: raw?.environmentLabel ?? '',
  };
}

// True for any deployed non-production environment (preview, GKE dry-run, etc.).
// Drives both the environment banner's visibility and the SPA's test-hook gate,
// mirroring the backend testhandler's "APP_ENV != production" rule.
export function isNonProd(label: string): boolean {
  return label !== '' && label !== 'prod' && label !== 'production';
}
