import { describe, it, expect, afterEach } from 'vitest';
import { getAppConfig, isNonProd } from './appConfig';

afterEach(() => {
  delete (window as Window).__APP_CONFIG__;
});

describe('getAppConfig', () => {
  it('returns the injected environment label', () => {
    window.__APP_CONFIG__ = { environmentLabel: 'GKE pre-prod' };
    expect(getAppConfig().environmentLabel).toBe('GKE pre-prod');
  });

  it('defaults to empty label when global is absent', () => {
    delete (window as Window).__APP_CONFIG__;
    expect(getAppConfig().environmentLabel).toBe('');
  });

  it('defaults to empty label when global lacks the field', () => {
    window.__APP_CONFIG__ = {};
    expect(getAppConfig().environmentLabel).toBe('');
  });
});

describe('isNonProd', () => {
  it.each(['preview', 'GKE pre-prod', 'staging', 'dev'])(
    'is true for non-prod label %j',
    (label) => {
      expect(isNonProd(label)).toBe(true);
    }
  );

  it.each(['', 'prod', 'production'])('is false for %j', (label) => {
    expect(isNonProd(label)).toBe(false);
  });
});
