/**
 * WebhooksSection — org webhook registration in Org Settings (TRA-1043).
 *
 * One webhook per organization, one event type (`asset.moved`), fired only when
 * an asset's location actually changes.
 *
 * Visibility is the caller's responsibility — OrgSettingsScreen only mounts this
 * for an admin. The backend independently requires admin (403 otherwise), so
 * this is a UI affordance, not the security boundary.
 */

import { useCallback, useEffect, useState } from 'react';
import { webhooksApi } from '@/lib/api/webhooks';
import { extractErrorMessage } from '@/lib/asset/helpers';
import type { Webhook, WebhookTestResult } from '@/types/webhook';
import toast from 'react-hot-toast';

/**
 * Mirrors the backend's webhook.Mask: prefix + ellipsis + last four. Applied
 * client-side to the create response so the cleartext secret lives in exactly
 * one place on screen — the reveal-once banner. Without this the "will not be
 * shown again" warning sits directly above a full-length copy of the secret,
 * which reads as a lie and stays on screen until the next load.
 *
 * Idempotent: a value the API already masked is returned unchanged.
 */
function maskSecret(secret: string): string {
  if (!secret || secret.includes('…')) return secret;
  if (secret.length < 'whsec_'.length + 8) return 'whsec_…';
  return `whsec_…${secret.slice(-4)}`;
}

interface WebhooksSectionProps {
  /**
   * true when rendered as its own screen rather than as a section inside a
   * settings form: drops the top divider and promotes the heading, since there
   * is nothing above it to separate from.
   */
  standalone?: boolean;
}

export function WebhooksSection({ standalone = false }: WebhooksSectionProps) {
  const [webhook, setWebhook] = useState<Webhook | null>(null);
  const [url, setUrl] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  /**
   * The cleartext secret, held only for as long as this screen stays mounted.
   * The API returns it exactly once, on create, and there is no way to read it
   * back — so the copy-it-now warning below is literal.
   */
  const [revealedSecret, setRevealedSecret] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<WebhookTestResult | null>(null);

  const load = useCallback(async () => {
    setIsLoading(true);
    try {
      const wh = await webhooksApi.get();
      setWebhook(wh);
      setUrl(wh?.url ?? '');
      setEnabled(wh?.enabled ?? true);
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to load webhook'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const handleSave = async () => {
    if (isSaving) return;
    setError(null);
    setTestResult(null);
    setIsSaving(true);
    try {
      if (webhook) {
        const updated = await webhooksApi.update(webhook.id, { url, enabled });
        setWebhook(updated);
        toast.success('Webhook updated');
      } else {
        const created = await webhooksApi.create({ url, enabled });
        // The cleartext goes to the reveal-once banner and nowhere else; the
        // persistent row below renders the masked form immediately, matching
        // what every subsequent load will show.
        setWebhook({ ...created, secret: maskSecret(created.secret) });
        setRevealedSecret(created.secret);
        toast.success('Webhook created');
      }
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to save webhook'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!webhook || isSaving) return;
    setError(null);
    setIsSaving(true);
    try {
      await webhooksApi.remove(webhook.id);
      setWebhook(null);
      setUrl('');
      setEnabled(true);
      setRevealedSecret(null);
      setTestResult(null);
      toast.success('Webhook deleted');
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to delete webhook'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleTest = async () => {
    if (!webhook || isTesting) return;
    setError(null);
    setTestResult(null);
    setIsTesting(true);
    try {
      setTestResult(await webhooksApi.test(webhook.id));
    } catch (err) {
      setError(extractErrorMessage(err, 'Failed to send test event'));
    } finally {
      setIsTesting(false);
    }
  };

  const isDirty = webhook ? url !== webhook.url || enabled !== webhook.enabled : url.trim() !== '';

  return (
    <section className={standalone ? '' : 'mt-8 border-t border-gray-700 pt-6'}>
      <h2 className={`font-semibold text-white mb-1 ${standalone ? 'text-2xl' : 'text-lg'}`}>
        Webhooks
      </h2>
      <p className="text-sm text-gray-400 mb-4">
        Receive an <code className="text-gray-300">asset.moved</code> event when an asset is
        scanned at a different location than it was last seen at. Rescans at the same location
        send nothing. Deliveries are signed with HMAC-SHA256 and are at-most-once — a delivery
        that fails twice is dropped, and order is not guaranteed.
      </p>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {revealedSecret && (
        <div className="bg-amber-900/20 border border-amber-700 rounded-lg p-3 mb-4">
          <p className="text-amber-300 text-sm font-medium mb-1">
            Copy your signing secret now — it will not be shown again.
          </p>
          <code
            data-testid="webhook-secret"
            className="block break-all text-xs text-amber-100 bg-gray-900/60 rounded p-2"
          >
            {revealedSecret}
          </code>
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-400 text-sm">Loading webhook…</p>
      ) : (
        <div className="space-y-4">
          <div>
            <label htmlFor="webhook-url" className="block text-sm font-medium text-gray-300 mb-2">
              Endpoint URL
            </label>
            <input
              id="webhook-url"
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/trakrf/hooks"
              disabled={isSaving}
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:opacity-50"
            />
            <p className="text-gray-500 text-sm mt-1">
              Must be an https URL reachable from the internet.
            </p>
          </div>

          {webhook && (
            <div>
              <span className="block text-sm font-medium text-gray-300 mb-1">Signing secret</span>
              {/* break-all: the masked form is short, but a long value must wrap
                  inside the card rather than running past its right edge. */}
              <code className="block break-all text-sm text-gray-400">{webhook.secret}</code>
            </div>
          )}

          <div className="flex items-center gap-3">
            <input
              id="webhook-enabled"
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              disabled={isSaving}
              className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-2 focus:ring-blue-500"
            />
            <label htmlFor="webhook-enabled" className="text-sm font-medium text-gray-300">
              Deliver events
            </label>
          </div>

          {testResult && (
            <div
              data-testid="webhook-test-result"
              className="bg-gray-900/40 border border-gray-700 rounded-lg p-3"
            >
              <p className="text-sm text-gray-300">
                Test event responded with status {testResult.status_code}
              </p>
              {testResult.error && (
                <p className="text-sm text-red-400 mt-1">{testResult.error}</p>
              )}
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={handleSave}
              disabled={isSaving || !isDirty || url.trim() === ''}
              className="bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSaving ? 'Saving...' : webhook ? 'Save webhook' : 'Create webhook'}
            </button>

            {webhook && (
              <>
                <button
                  type="button"
                  onClick={handleTest}
                  disabled={isTesting}
                  className="px-4 py-2 text-sm border border-gray-600 rounded-lg text-gray-300 hover:text-white hover:border-gray-400 transition-colors disabled:opacity-50"
                >
                  {isTesting ? 'Sending...' : 'Send test event'}
                </button>
                <button
                  type="button"
                  onClick={handleDelete}
                  disabled={isSaving}
                  className="px-4 py-2 text-sm border border-red-600/50 rounded-lg text-red-400 hover:bg-red-600/20 transition-colors disabled:opacity-50"
                >
                  Delete
                </button>
              </>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
