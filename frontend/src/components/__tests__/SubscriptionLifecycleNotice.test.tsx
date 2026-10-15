import "@testing-library/jest-dom";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import {
  CurrentOrgSubscriptionNotice,
  SubscriptionLifecycleNotice,
} from "@/components/SubscriptionLifecycleNotice";
import { useAuthStore, useOrgStore } from "@/stores";

describe("SubscriptionLifecycleNotice", () => {
  afterEach(cleanup);

  it("does not distract perpetual active organizations", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt={null} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  const inDays = (days: number) => new Date(Date.now() + days * 86_400_000).toISOString();

  it("warns within 14 days of expiry", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt={inDays(10)} />);
    expect(screen.getByRole("status")).toHaveTextContent(/expires/i);
  });

  it("stays quiet while expiry is more than 14 days away", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt={inDays(20)} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("identifies grace separately from cutoff", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt="2000-06-15T12:00:00Z" />);
    expect(screen.getByRole("status")).toHaveTextContent(/grace period/i);
  });

  it("explains the expected capture stop after cutoff", () => {
    render(<SubscriptionLifecycleNotice isEntitled={false} subscriptionEnabled subscriptionExpiresAt="2000-06-15T12:00:00Z" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/fixed-reader capture.*stopped/i);
  });

  it("warns that fixed readers look online but are not recorded after cutoff", () => {
    render(<SubscriptionLifecycleNotice isEntitled={false} subscriptionEnabled subscriptionExpiresAt="2000-06-15T12:00:00Z" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/readers may still appear online.*not being recorded/i);
  });
});

describe("CurrentOrgSubscriptionNotice", () => {
  afterEach(() => {
    cleanup();
    useAuthStore.setState({ isAuthenticated: false } as never);
    useOrgStore.setState({ currentOrg: null } as never);
  });

  const cutOffOrg = {
    id: 1, name: "Acme", identifier: "acme", role: "admin",
    is_entitled: false, subscription_enabled: true,
    subscription_expires_at: "2000-06-15T12:00:00Z", capabilities: [],
  };

  it("shows the current org's notice to a signed-in user", () => {
    useAuthStore.setState({ isAuthenticated: true } as never);
    useOrgStore.setState({ currentOrg: cutOffOrg } as never);
    render(<CurrentOrgSubscriptionNotice />);
    expect(screen.getByRole("alert")).toHaveTextContent(/cut off/i);
  });

  it("renders nothing when signed out", () => {
    useOrgStore.setState({ currentOrg: cutOffOrg } as never);
    const { container } = render(<CurrentOrgSubscriptionNotice />);
    expect(container).toBeEmptyDOMElement();
  });
});
