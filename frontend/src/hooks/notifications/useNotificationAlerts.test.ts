import React, { type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  useNotificationAlerts,
  alertQuery,
  toInstant,
} from "./useNotificationAlerts";
import { notificationsApi } from "@/lib/api/notifications";
import type { NotificationAlert } from "@/types/notifications";

vi.mock("@/lib/api/notifications");

const mockOrgState = { currentOrg: { id: 7, name: "Test Org" } };
vi.mock("@/stores/orgStore", () => ({
  useOrgStore: Object.assign(
    vi.fn((selector) => (selector ? selector(mockOrgState) : mockOrgState)),
    { getState: () => mockOrgState },
  ),
}));

const alert: NotificationAlert = {
  id: "alert-20261020T110000Z-alarm-2814749767106561",
  asset_id: 5,
  asset_name: "Forklift 3",
  asset_external_key: "FL-003",
  type: "alarm",
  occurred_at: "2026-10-20T10:59:55Z",
  sent_at: "2026-10-20T11:00:00Z",
  recipient_id: 1,
  recipient_name: "Ana",
  channel: "email",
  subject: "Alarm",
  body: "Forklift 3 left through Gate A",
  details: { location: "Gate A" },
};

const wrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(
      QueryClientProvider,
      { client: queryClient },
      children,
    );
  };
};

describe("useNotificationAlerts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(notificationsApi.listAlerts).mockResolvedValue({
      data: { data: [alert], pagination: { total: 41, page: 3, per_page: 20 } },
    } as never);
  });

  it("returns the page and total the endpoint sends", async () => {
    const { result } = renderHook(() => useNotificationAlerts({ page: 3 }), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.alerts).toEqual([alert]);
    expect(result.current.total).toBe(41);
    expect(result.current.page).toBe(3);
    expect(result.current.perPage).toBe(20);
    expect(notificationsApi.listAlerts).toHaveBeenCalledWith({
      page: 3,
      per_page: 20,
    });
  });

  it("surfaces a failed request as an error with no alerts", async () => {
    const failure = new Error("boom");
    vi.mocked(notificationsApi.listAlerts).mockRejectedValue(failure);
    const { result } = renderHook(() => useNotificationAlerts(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.error).toBe(failure));
    expect(result.current.alerts).toEqual([]);
    expect(result.current.total).toBe(0);
  });
});

describe("alertQuery", () => {
  it("leaves unset filters out, because the server rejects empty values", () => {
    expect(alertQuery({}, 1, 20)).toEqual({ page: 1, per_page: 20 });
  });

  it("passes every set filter through", () => {
    expect(
      alertQuery(
        {
          asset_id: 5,
          type: "location_change",
          recipient_id: 2,
          channel: "sms",
        },
        2,
        10,
      ),
    ).toEqual({
      page: 2,
      per_page: 10,
      asset_id: 5,
      type: "location_change",
      recipient_id: 2,
      channel: "sms",
    });
  });

  it("sends bare dates as the edges of that local day", () => {
    const query = alertQuery({ from: "2026-10-20", to: "2026-10-20" }, 1, 20);
    expect(query.from).toBe(new Date("2026-10-20T00:00:00.000").toISOString());
    expect(query.to).toBe(new Date("2026-10-20T23:59:59.999").toISOString());
  });
});

describe("toInstant", () => {
  it("leaves an instant unchanged", () => {
    expect(toInstant("2026-10-20T08:00:00Z", "start")).toBe(
      "2026-10-20T08:00:00Z",
    );
  });
});
