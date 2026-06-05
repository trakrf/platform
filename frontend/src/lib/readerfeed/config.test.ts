import { describe, it, expect, afterEach } from 'vitest';
import { getReaderFeedConfig, isReaderFeedConfigured } from './config';

afterEach(() => {
  delete window.__APP_CONFIG__;
});

describe('getReaderFeedConfig', () => {
  it('reads the runtime-injected reader-feed config', () => {
    window.__APP_CONFIG__ = {
      readerFeed: {
        url: 'wss://mqtt.preview.gke.trakrf.id:8084/mqtt',
        username: 'frontend-readonly',
        password: 's3cret',
        topic: 'trakrf.id/dock-01/reads',
      },
    };
    expect(getReaderFeedConfig()).toEqual({
      url: 'wss://mqtt.preview.gke.trakrf.id:8084/mqtt',
      username: 'frontend-readonly',
      password: 's3cret',
      topic: 'trakrf.id/dock-01/reads',
    });
  });

  it('defaults the topic when blank or absent', () => {
    window.__APP_CONFIG__ = { readerFeed: { url: 'wss://x/mqtt' } };
    expect(getReaderFeedConfig().topic).toBe('trakrf.id/+/reads');

    window.__APP_CONFIG__ = { readerFeed: { url: 'wss://x/mqtt', topic: '   ' } };
    expect(getReaderFeedConfig().topic).toBe('trakrf.id/+/reads');
  });

  it('returns empty url (disabled) when the global is absent', () => {
    delete window.__APP_CONFIG__;
    expect(getReaderFeedConfig().url).toBe('');
  });

  it('returns empty url when readerFeed is absent', () => {
    window.__APP_CONFIG__ = { environmentLabel: 'preview' };
    expect(getReaderFeedConfig().url).toBe('');
  });
});

describe('isReaderFeedConfigured', () => {
  it('is false with no url', () => {
    delete window.__APP_CONFIG__;
    expect(isReaderFeedConfigured()).toBe(false);
  });

  it('is false for a whitespace-only url', () => {
    window.__APP_CONFIG__ = { readerFeed: { url: '   ' } };
    expect(isReaderFeedConfigured()).toBe(false);
  });

  it('is true once a url is present', () => {
    window.__APP_CONFIG__ = { readerFeed: { url: 'wss://x/mqtt' } };
    expect(isReaderFeedConfigured()).toBe(true);
  });
});
