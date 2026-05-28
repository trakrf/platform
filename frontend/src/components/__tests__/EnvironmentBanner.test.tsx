import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { EnvironmentBanner } from '@/components/EnvironmentBanner';

function setEnvironmentLabel(label: string | undefined) {
  if (label === undefined) {
    delete (window as Window).__APP_CONFIG__;
  } else {
    window.__APP_CONFIG__ = { environmentLabel: label };
  }
}

describe('EnvironmentBanner', () => {
  const originalTitle = document.title;

  beforeEach(() => {
    document.title = 'TrakRF';
  });

  afterEach(() => {
    cleanup();
    document.title = originalTitle;
    setEnvironmentLabel(undefined);
  });

  it('should show banner for any non-prod environment', () => {
    setEnvironmentLabel('preview');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Preview Environment');
    expect(banner).toHaveClass('bg-purple-600');
  });

  it('should show a multi-word label verbatim (GKE dry-run)', () => {
    setEnvironmentLabel('GKE pre-prod');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toHaveTextContent('GKE pre-prod Environment');
  });

  it('should render nothing for prod environment', () => {
    setEnvironmentLabel('prod');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing for production environment', () => {
    setEnvironmentLabel('production');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when label is empty', () => {
    setEnvironmentLabel('');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when __APP_CONFIG__ is absent', () => {
    setEnvironmentLabel(undefined);
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should set page title with environment prefix', () => {
    setEnvironmentLabel('preview');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[PRE] TrakRF');
  });

  it('should not modify page title for prod environment', () => {
    setEnvironmentLabel('prod');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('TrakRF');
  });
});
