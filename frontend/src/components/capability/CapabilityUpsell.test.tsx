import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import CapabilityUpsell from './CapabilityUpsell';
import { CAPABILITY_GEOFENCE } from './registry';
import { SUPPORT_EMAIL } from '@/components/entitlement/gateCopy';

describe('CapabilityUpsell', () => {
  afterEach(cleanup);

  it('renders the fixed geofence copy', () => {
    render(<CapabilityUpsell capability={CAPABILITY_GEOFENCE} label="Geofence defaults" />);

    expect(screen.getByText('Geofence')).toBeInTheDocument();
    expect(screen.getByText('Zone-based tracking with enter/exit alerts.')).toBeInTheDocument();
    expect(
      screen.getByText(/This feature isn't enabled for your organization\./)
    ).toBeInTheDocument();
  });

  it('points the contact link at the existing support address', () => {
    render(<CapabilityUpsell capability={CAPABILITY_GEOFENCE} label="Geofence defaults" />);

    const link = screen.getByTestId('capability-upsell-contact');
    expect(link).toHaveAttribute('href', expect.stringContaining(`mailto:${SUPPORT_EMAIL}`));
    expect(link).toHaveTextContent(SUPPORT_EMAIL);
  });

  // Copy is fixed; a capability with no entry must not get an invented claim.
  it('falls back to the registry label and no blurb for an uncopied capability', () => {
    render(<CapabilityUpsell capability="wip_tracking" label="WIP tracking" />);

    expect(screen.getByText('WIP tracking')).toBeInTheDocument();
    expect(
      screen.getByText(/This feature isn't enabled for your organization\./)
    ).toBeInTheDocument();
    expect(screen.queryByText(/Zone-based tracking/)).not.toBeInTheDocument();
  });
});
