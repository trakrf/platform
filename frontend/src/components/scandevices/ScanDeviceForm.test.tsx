import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { ScanDeviceForm } from './ScanDeviceForm';
import { useOrgStore } from '@/stores/orgStore';
import type { ScanDevice } from '@/types/scandevices';

afterEach(cleanup);

const editDevice: ScanDevice = {
  id: 7,
  org_id: 1,
  name: 'Legacy Reader',
  type: 'csl_cs463',
  transport: 'mqtt',
  publish_topic: 'oc/legacy/reads',
  serial_number: null,
  model: null,
  description: '',
  is_active: true,
  metadata: {},
  valid_from: '',
  valid_to: null,
  created_at: '',
  updated_at: null,
  deleted_at: null,
};

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

describe('ScanDeviceForm de-duplication in edit mode (TRA-940)', () => {
  const noop = vi.fn();

  it('create mode still renders the Name field and Active checkbox', () => {
    render(<ScanDeviceForm mode="create" onSubmit={noop} onCancel={noop} />);
    expect(screen.getByLabelText(/^name/i)).toBeInTheDocument();
    expect(screen.getByLabelText('Active')).toBeInTheDocument();
  });

  it('edit mode hides the Name field and Active checkbox (now inline in the row)', () => {
    render(<ScanDeviceForm mode="edit" device={editDevice} onSubmit={noop} onCancel={noop} />);
    expect(screen.queryByLabelText(/^name/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Active')).not.toBeInTheDocument();
  });

  it('edit mode keeps the cascade fields (type, transport, publish_topic)', () => {
    render(<ScanDeviceForm mode="edit" device={editDevice} onSubmit={noop} onCancel={noop} />);
    expect(screen.getByLabelText(/^type/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/transport/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/publish topic/i)).toBeInTheDocument();
  });

  it('edit submit omits name and is_active (owned by the inline cells)', async () => {
    const onSubmit = vi.fn();
    render(<ScanDeviceForm mode="edit" device={editDevice} onSubmit={onSubmit} onCancel={noop} />);
    fireEvent.click(screen.getByRole('button', { name: /Update Scan Device/i }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    const payload = onSubmit.mock.calls[0][0];
    expect(payload).not.toHaveProperty('name');
    expect(payload).not.toHaveProperty('is_active');
    expect(payload).toMatchObject({ type: 'csl_cs463', transport: 'mqtt' });
  });
});
