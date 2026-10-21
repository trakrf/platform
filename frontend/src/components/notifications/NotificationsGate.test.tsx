import "@testing-library/jest-dom";
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { NotificationsGate } from "./NotificationsGate";
import type { Entitlement } from "@/hooks/entitlement/useEntitlement";

const useEntitlement = vi.fn<() => Entitlement>();
vi.mock("@/hooks/entitlement/useEntitlement", () => ({
  useEntitlement: () => useEntitlement(),
}));

function entitlement(state: Entitlement["state"]): Entitlement {
  const isEntitled = state === "entitled";
  return {
    state,
    isEntitled,
    isLocked: !isEntitled,
    subscriptionExpiresAt: null,
  };
}

describe("NotificationsGate", () => {
  afterEach(cleanup);

  it("entitled: renders children and no upsell", () => {
    useEntitlement.mockReturnValue(entitlement("entitled"));
    render(
      <NotificationsGate>
        <p>Alert list</p>
      </NotificationsGate>,
    );

    expect(screen.getByText("Alert list")).toBeInTheDocument();
    expect(
      screen.queryByTestId("capability-upsell-notifications"),
    ).not.toBeInTheDocument();
  });

  it.each(["lapsed", "logged-out"] as const)(
    "%s: renders the upsell instead of children",
    (state) => {
      useEntitlement.mockReturnValue(entitlement(state));
      render(
        <NotificationsGate>
          <p>Alert list</p>
        </NotificationsGate>,
      );

      expect(
        screen.getByTestId("capability-upsell-notifications"),
      ).toBeInTheDocument();
      expect(screen.getByText("Notifications")).toBeInTheDocument();
      expect(screen.queryByText("Alert list")).not.toBeInTheDocument();
    },
  );
});
