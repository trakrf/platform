import { describe, it, expect } from 'vitest';
import { gateCopy } from './gateCopy';

describe('gateCopy', () => {
  it('logged-out is a start-free-trial signup CTA', () => {
    const c = gateCopy('logged-out');
    expect(c.action).toBe('signup');
    expect(c.ctaLabel.toLowerCase()).toContain('trial');
    expect(c.message.length).toBeGreaterThan(0);
  });

  it('lapsed is a renew/contact CTA', () => {
    const c = gateCopy('lapsed');
    expect(c.action).toBe('contact');
    expect(c.message.toLowerCase()).toContain('renew');
  });
});
