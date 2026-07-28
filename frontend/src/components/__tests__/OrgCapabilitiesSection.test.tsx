import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { OrgCapabilitiesSection } from '@/components/OrgCapabilitiesSection';
import { orgsApi } from '@/lib/api/orgs';

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    getOrgCapabilities: vi.fn(),
    setOrgCapabilities: vi.fn(),
  },
}));

vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

type CapsResponse = Awaited<ReturnType<typeof orgsApi.getOrgCapabilities>>;

function mockLoad(capabilities: string[], available: string[]) {
  vi.mocked(orgsApi.getOrgCapabilities).mockResolvedValueOnce({
    data: { data: { capabilities, available } },
  } as CapsResponse);
}

function mockSave(capabilities: string[], available: string[]) {
  vi.mocked(orgsApi.setOrgCapabilities).mockResolvedValueOnce({
    data: { data: { capabilities, available } },
  } as CapsResponse);
}

const ALL = ['geofence', 'inventory', 'mustering'];

describe('OrgCapabilitiesSection (TRA-1027)', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('renders a checkbox per available capability, checked for the granted ones', async () => {
    mockLoad(['geofence'], ALL);

    render(<OrgCapabilitiesSection orgId={42} />);

    const geofence = (await screen.findByLabelText(/geofence/i)) as HTMLInputElement;
    const mustering = screen.getByLabelText(/mustering/i) as HTMLInputElement;
    const inventory = screen.getByLabelText(/inventory/i) as HTMLInputElement;

    expect(geofence.checked).toBe(true);
    expect(mustering.checked).toBe(false);
    expect(inventory.checked).toBe(false);
  });

  // The vocabulary comes from the server so a capability added backend-side
  // appears here without a frontend release. A hardcoded list would drop it
  // silently — an ungrantable capability looks exactly like an ungranted one.
  it('renders a capability the frontend has never heard of', async () => {
    mockLoad([], [...ALL, 'wip_tracking']);

    render(<OrgCapabilitiesSection orgId={42} />);

    expect(await screen.findByLabelText(/wip tracking/i)).toBeInTheDocument();
  });

  it('grants a capability by submitting the whole set', async () => {
    mockLoad(['geofence'], ALL);
    mockSave(['geofence', 'mustering'], ALL);

    render(<OrgCapabilitiesSection orgId={42} />);
    fireEvent.click(await screen.findByLabelText(/mustering/i));
    fireEvent.click(screen.getByRole('button', { name: /save capabilities/i }));

    await waitFor(() => {
      expect(orgsApi.setOrgCapabilities).toHaveBeenCalledWith(42, {
        capabilities: ['geofence', 'mustering'],
      });
    });
  });

  it('revokes a capability by omitting it from the submitted set', async () => {
    mockLoad(['geofence', 'mustering'], ALL);
    mockSave(['mustering'], ALL);

    render(<OrgCapabilitiesSection orgId={42} />);
    fireEvent.click(await screen.findByLabelText(/geofence/i));
    fireEvent.click(screen.getByRole('button', { name: /save capabilities/i }));

    await waitFor(() => {
      expect(orgsApi.setOrgCapabilities).toHaveBeenCalledWith(42, {
        capabilities: ['mustering'],
      });
    });
  });

  // Revoking everything is a legitimate request, and the one most likely to be
  // mangled into "send nothing" — the backend reads a missing key as a 400.
  it('sends an explicit empty list when every capability is unchecked', async () => {
    mockLoad(['geofence'], ALL);
    mockSave([], ALL);

    render(<OrgCapabilitiesSection orgId={42} />);
    fireEvent.click(await screen.findByLabelText(/geofence/i));
    fireEvent.click(screen.getByRole('button', { name: /save capabilities/i }));

    await waitFor(() => {
      expect(orgsApi.setOrgCapabilities).toHaveBeenCalledWith(42, { capabilities: [] });
    });
  });

  it('reflects the set the server returns rather than the one it sent', async () => {
    mockLoad([], ALL);
    // Server truth wins: another operator may have granted geofence meanwhile.
    mockSave(['geofence', 'mustering'], ALL);

    render(<OrgCapabilitiesSection orgId={42} />);
    fireEvent.click(await screen.findByLabelText(/mustering/i));
    fireEvent.click(screen.getByRole('button', { name: /save capabilities/i }));

    await waitFor(() => {
      expect((screen.getByLabelText(/geofence/i) as HTMLInputElement).checked).toBe(true);
    });
  });

  it('surfaces a load failure instead of rendering an empty grant list', async () => {
    vi.mocked(orgsApi.getOrgCapabilities).mockRejectedValueOnce({
      response: { data: { error: { detail: 'Organization not found' } } },
    });

    render(<OrgCapabilitiesSection orgId={42} />);

    expect(await screen.findByText(/organization not found/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /save capabilities/i })).not.toBeInTheDocument();
  });

  it('surfaces a save failure and leaves the checkboxes as the operator set them', async () => {
    mockLoad([], ALL);
    vi.mocked(orgsApi.setOrgCapabilities).mockRejectedValueOnce({
      response: { data: { error: { detail: 'unknown capability: wip_tracking' } } },
    });

    render(<OrgCapabilitiesSection orgId={42} />);
    fireEvent.click(await screen.findByLabelText(/geofence/i));
    fireEvent.click(screen.getByRole('button', { name: /save capabilities/i }));

    expect(await screen.findByText(/unknown capability/i)).toBeInTheDocument();
    expect((screen.getByLabelText(/geofence/i) as HTMLInputElement).checked).toBe(true);
  });
});
