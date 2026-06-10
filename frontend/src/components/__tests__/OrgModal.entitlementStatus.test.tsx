import React, { type ReactNode } from "react";
import "@testing-library/jest-dom";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { OrgModal } from "@/components/OrgModal";
import { useOrgStore, useAuthStore } from "@/stores";

vi.mock("@/lib/api/orgs", () => ({
  orgsApi: { listMembers: vi.fn(), update: vi.fn(), delete: vi.fn() },
}));
vi.mock("react-hot-toast", () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const renderModal = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(
    <OrgModal isOpen mode="manage" defaultTab="settings" onClose={() => {}} />,
    { wrapper: Wrapper },
  );
};

function setAdminOrg(opts: {
  is_entitled: boolean;
  subscription_expires_at: string | null;
}) {
  const org = {
    id: 1,
    name: "My Org",
    identifier: "my-org",
    role: "admin" as const,
    is_entitled: opts.is_entitled,
    subscription_enabled: true,
    subscription_expires_at: opts.subscription_expires_at,
  };
  useOrgStore.setState({ currentOrg: org, currentRole: "admin", orgs: [] });
  useAuthStore.setState({
    profile: {
      id: 1,
      email: "a@x",
      name: "A",
      is_superadmin: false,
      current_org: org,
      orgs: [],
    },
    isAuthenticated: true,
  });
}

describe("OrgModal settings tab — read-only entitlement status (TRA-975)", () => {
  beforeEach(() => {
    window.location.hash = "";
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows the trial status + expiry to a regular org admin (no superadmin)", () => {
    setAdminOrg({
      is_entitled: true,
      subscription_expires_at: "2999-06-15T12:00:00Z",
    });
    renderModal();
    expect(screen.getByLabelText("Subscription status")).toBeInTheDocument();
    expect(screen.getByText(/trial/i)).toBeInTheDocument();
    expect(screen.getByText(/2999/)).toBeInTheDocument();
  });
});
