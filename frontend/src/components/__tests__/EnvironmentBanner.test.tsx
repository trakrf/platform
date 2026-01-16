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

  it('should show orange banner for dev environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'dev');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Development Environment');
    expect(banner).toHaveClass('bg-orange-500');
  });

  it('should show purple banner for staging environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'staging');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Staging Environment');
    expect(banner).toHaveClass('bg-purple-600');
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

  it('should set page title with [DEV] prefix for dev environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'dev');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[DEV] TrakRF');
  });

  it('should set page title with [STG] prefix for staging environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'staging');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[STG] TrakRF');
  });

  it('should not modify page title for prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'prod');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('TrakRF');
  });
});
