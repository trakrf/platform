import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ScanDeviceForm } from './ScanDeviceForm';
import { useOrgStore } from '@/stores/orgStore';

afterEach(cleanup);

function setOrg(identifier: string | null) {
  useOrgStore.setState({
    currentOrg: identifier === null ? null : { id: 1, name: 'Org', identifier, role: 'admin' },
  });
}

describe('ScanDeviceForm publish_topic prefill (TRA-922)', () => {
  const noop = vi.fn();

  it('pre-fills the create form with the {org_slug}/ prefix', () => {
    setOrg('organized-chaos');
    render(<ScanDeviceForm mode="create" onSubmit={noop} onCancel={noop} />);
    const input = screen.getByLabelText(/publish topic/i) as HTMLInputElement;
    expect(input.value).toBe('organized-chaos/');
  });

  it('leaves the create form empty when no org slug is available', () => {
    setOrg(null);
    render(<ScanDeviceForm mode="create" onSubmit={noop} onCancel={noop} />);
    const input = screen.getByLabelText(/publish topic/i) as HTMLInputElement;
    expect(input.value).toBe('');
  });

  it('does not prefix in edit mode — the stored topic shows verbatim', () => {
    setOrg('organized-chaos');
    render(
      <ScanDeviceForm
        mode="edit"
        device={{
          id: 1,
          org_id: 1,
          name: 'Legacy',
          type: 'csl_cs463',
          transport: 'mqtt',
          publish_topic: 'trakrf.id/legacy/reads',
          serial_number: null,
          model: null,
          description: '',
          is_active: true,
          metadata: {},
          valid_from: '',
          valid_to: null,
          created_at: '',
          updated_at: '',
        }}
        onSubmit={noop}
        onCancel={noop}
      />
    );
    const input = screen.getByLabelText(/publish topic/i) as HTMLInputElement;
    expect(input.value).toBe('trakrf.id/legacy/reads');
  });
});
