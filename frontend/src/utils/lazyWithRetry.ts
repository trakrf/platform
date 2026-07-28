import { lazy, ComponentType, LazyExoticComponent } from 'react';

// Retry logic for lazy loaded components
//
// Vite content-hashes chunk filenames, so a deploy replaces dist/assets/ and any
// tab still holding the old names starts getting 404s. The recovery is to reload
// once and pick up the new index; if the chunk is still missing after that, the
// deploy is genuinely broken and the error belongs to the ErrorBoundary.
//
// TRA-1054: the retry budget is tracked per chunk. A single shared flag that any
// successful load clears re-arms every *other* chunk on every page load, so a
// chunk that is genuinely gone reloads forever — the screen chunk resolves,
// clears the flag, and the nested chunk it renders fails with a fresh retry
// available. Measured at 176 reloads in 20s on preview before this was keyed.
const RETRY_KEY_PREFIX = 'chunk-retry:';

/**
 * A stable id for one call site. The thunk's source text names the chunk it
 * pulls, so it differs between call sites and is identical across a reload of
 * the same build. A new build changes it, which is what we want: a fresh deploy
 * re-arms the retry.
 */
function retryKey(componentImport: () => unknown): string {
  const source = componentImport.toString();
  let hash = 0;
  for (let i = 0; i < source.length; i++) {
    hash = (Math.imul(hash, 31) + source.charCodeAt(i)) | 0;
  }
  return `${RETRY_KEY_PREFIX}${(hash >>> 0).toString(36)}`;
}

export function lazyWithRetry<T extends ComponentType<any>>(
  componentImport: () => Promise<{ default: T }>
): LazyExoticComponent<T> {
  return lazy(async () => {
    const key = retryKey(componentImport);

    try {
      const component = await componentImport();
      // Re-arm this chunk only, so a later deploy in the same session can still
      // recover. Clearing a shared flag here is what caused the reload loop.
      window.sessionStorage.removeItem(key);
      return component;
    } catch (error) {
      // Read after the failure, not before: a nested chunk's load begins only
      // once its parent has mounted, so the flag must reflect the state at the
      // moment this chunk failed.
      if (window.sessionStorage.getItem(key) !== 'true') {
        window.sessionStorage.setItem(key, 'true');
        window.location.reload();
        return { default: (() => null) as unknown as T };
      }

      throw error;
    }
  });
}
