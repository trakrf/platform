/**
 * WebhooksScreen — standalone webhook management (TRA-1043).
 *
 * Mirrors APIKeysScreen: its own tab, reached from the "Manage webhooks →"
 * button in org settings, rather than living inline in the settings form.
 * Webhooks are an integration surface like API keys, not an org attribute like
 * the org name, so they get the room to grow (Phase 2 adds delivery logs and
 * N subscriptions).
 *
 * The admin check here is a UI affordance; the backend independently requires
 * admin on every /api/v1/webhooks route.
 */

import { useOrgStore } from '@/stores';
import { WebhooksSection } from './WebhooksSection';

export default function WebhooksScreen() {
  const { currentRole } = useOrgStore();
  const isAdmin = currentRole === 'owner' || currentRole === 'admin';

  if (!isAdmin) {
    return (
      <div className="max-w-3xl mx-auto py-8 px-4">
        <h1 className="text-2xl font-semibold">Webhooks</h1>
        <p className="mt-4 text-gray-600 dark:text-gray-400">
          Only organization admins can manage webhooks.
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-3xl mx-auto py-8 px-4">
      <WebhooksSection standalone />
    </div>
  );
}
