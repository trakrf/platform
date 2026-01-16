import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { EnvironmentBanner } from '@/components/EnvironmentBanner';

describe('EnvironmentBanner', () => {
  const originalTitle = document.title;

  beforeEach(() => {
    document.title = 'TrakRF';
  });

  afterEach(() => {
    cleanup();
    document.title = originalTitle;
    vi.unstubAllEnvs();
  });

  it('should show banner for any non-prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'preview');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Preview Environment');
    expect(banner).toHaveClass('bg-purple-600');
  });

  it('should capitalize environment name in banner', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'dev');
    render(<EnvironmentBanner />);

    expect(screen.getByTestId('environment-banner')).toHaveTextContent('Dev Environment');
  });

  it('should render nothing for prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'prod');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when VITE_ENVIRONMENT is empty', () => {
    vi.stubEnv('VITE_ENVIRONMENT', '');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when VITE_ENVIRONMENT is undefined', () => {
    vi.stubEnv('VITE_ENVIRONMENT', undefined);
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should set page title with environment prefix', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'preview');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[PRE] TrakRF');
  });

  it('should not modify page title for prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'prod');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('TrakRF');
  });
});
