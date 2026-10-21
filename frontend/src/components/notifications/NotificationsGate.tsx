import type { ReactNode } from "react";
import { useEntitlement } from "@/hooks/entitlement/useEntitlement";
import CapabilityUpsell from "@/components/capability/CapabilityUpsell";

/**
 * Gates the notifications surfaces on the org's paid entitlement.
 *
 * This is the paid/lapsed entitlement check (`useEntitlement().isLocked`), not
 * a capability grant — there is no `notifications` capability in the registry.
 * It renders in the capability upsell card style by product decision. With no
 * CAPABILITY_UPSELL_COPY entry the card shows the heading and contact line
 * only, which is intended: that copy is owned elsewhere.
 */
export function NotificationsGate({ children }: { children: ReactNode }) {
  const { isLocked } = useEntitlement();
  if (isLocked)
    return (
      <CapabilityUpsell capability="notifications" label="Notifications" />
    );
  return <>{children}</>;
}
