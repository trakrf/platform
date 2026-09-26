import "@testing-library/jest-dom";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SubscriptionLifecycleNotice } from "@/components/SubscriptionLifecycleNotice";

describe("SubscriptionLifecycleNotice", () => {
  afterEach(cleanup);

  it("does not distract perpetual active organizations", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt={null} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("warns before expiry", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt="2999-06-15T12:00:00Z" />);
    expect(screen.getByRole("status")).toHaveTextContent(/expires/i);
  });

  it("identifies grace separately from cutoff", () => {
    render(<SubscriptionLifecycleNotice isEntitled subscriptionEnabled subscriptionExpiresAt="2000-06-15T12:00:00Z" />);
    expect(screen.getByRole("status")).toHaveTextContent(/grace period/i);
  });

  it("explains the expected capture stop after cutoff", () => {
    render(<SubscriptionLifecycleNotice isEntitled={false} subscriptionEnabled subscriptionExpiresAt="2000-06-15T12:00:00Z" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/fixed-reader capture.*stopped/i);
  });
});
