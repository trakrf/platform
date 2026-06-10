import "@testing-library/jest-dom";
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { OrgEntitlementStatus } from "@/components/OrgEntitlementStatus";

describe("OrgEntitlementStatus (TRA-975)", () => {
  afterEach(cleanup);

  it('shows "Active" when entitled with no expiry', () => {
    render(
      <OrgEntitlementStatus isEntitled={true} subscriptionExpiresAt={null} />,
    );
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("shows a trial with the expiry date when entitled with a future expiry", () => {
    render(
      <OrgEntitlementStatus
        isEntitled={true}
        subscriptionExpiresAt={"2999-06-15T12:00:00Z"}
      />,
    );
    expect(screen.getByText(/trial/i)).toBeInTheDocument();
    expect(screen.getByText(/2999/)).toBeInTheDocument();
  });

  it('shows "Expired" with the date when not entitled and the expiry is in the past', () => {
    render(
      <OrgEntitlementStatus
        isEntitled={false}
        subscriptionExpiresAt={"2000-06-15T12:00:00Z"}
      />,
    );
    expect(screen.getByText(/expired/i)).toBeInTheDocument();
    expect(screen.getByText(/2000/)).toBeInTheDocument();
  });

  it('shows "Inactive" when not entitled with no expiry (manually disabled)', () => {
    render(
      <OrgEntitlementStatus isEntitled={false} subscriptionExpiresAt={null} />,
    );
    expect(screen.getByText(/inactive/i)).toBeInTheDocument();
  });
});
