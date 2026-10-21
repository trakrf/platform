import { useState, Fragment } from 'react';
import { ChevronRight, ChevronDown, Mail, MessageSquare } from 'lucide-react';
import { formatDate, formatTime } from '@/lib/reports/utils';
import type { NotificationAlert } from '@/types/notifications';

interface AlertListProps {
  alerts: NotificationAlert[];
  /** Hides the asset column, for lists already scoped to one asset. */
  compact?: boolean;
}

const formatSent = (iso: string) => {
  const date = new Date(iso);
  return `${formatDate(date)} ${formatTime(date)}`;
};

function TypeBadge({ type }: { type: NotificationAlert['type'] }) {
  return type === 'alarm' ? (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300 whitespace-nowrap">
      Alarm
    </span>
  ) : (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 whitespace-nowrap">
      Location change
    </span>
  );
}

function ChannelLabel({ channel }: { channel: NotificationAlert['channel'] }) {
  const Icon = channel === 'email' ? Mail : MessageSquare;
  return (
    <span className="inline-flex items-center gap-1.5 text-gray-700 dark:text-gray-300 whitespace-nowrap">
      <Icon className="w-4 h-4 text-gray-500 dark:text-gray-400" aria-hidden="true" />
      {channel === 'email' ? 'Email' : 'SMS'}
    </span>
  );
}

function AlertMessage({ alert }: { alert: NotificationAlert }) {
  const details = Object.entries(alert.details);
  return (
    <div className="space-y-3 text-sm">
      {alert.subject && (
        <p className="font-medium text-gray-900 dark:text-gray-100">{alert.subject}</p>
      )}
      <p className="p-3 rounded-lg bg-gray-100 dark:bg-gray-800 text-gray-800 dark:text-gray-200 whitespace-pre-wrap break-words">
        {alert.body}
      </p>
      {details.length > 0 && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
          {details.map(([key, value]) => (
            <Fragment key={key}>
              <dt className="text-gray-500 dark:text-gray-400 capitalize">{key}</dt>
              <dd className="text-gray-900 dark:text-gray-100">{value}</dd>
            </Fragment>
          ))}
        </dl>
      )}
    </div>
  );
}

// Single-open expander, as on the output devices screen: opening one alert
// closes the previous one.
export function AlertList({ alerts, compact = false }: AlertListProps) {
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const toggle = (id: number) => setExpandedId((current) => (current === id ? null : id));
  const columnCount = compact ? 5 : 6;

  return (
    <>
      <table className="hidden md:table w-full text-sm">
        <thead className="bg-gray-50 dark:bg-gray-800">
          <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
            <th className="w-8 px-2"></th>
            {!compact && <th className="py-2 px-3 font-medium">Asset</th>}
            <th className="py-2 px-3 font-medium">Sent</th>
            <th className="px-3 font-medium">Type</th>
            <th className="px-3 font-medium">Subscriber</th>
            <th className="px-3 font-medium">Channel</th>
          </tr>
        </thead>
        <tbody>
          {alerts.map((alert) => {
            const isExpanded = expandedId === alert.id;
            return (
              <Fragment key={alert.id}>
                <tr
                  data-testid={`alert-row-${alert.id}`}
                  onClick={() => toggle(alert.id)}
                  className="cursor-pointer border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                >
                  <td className="w-8 px-2">
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        toggle(alert.id);
                      }}
                      className="p-1.5 text-gray-500 hover:text-blue-600 dark:text-gray-400 dark:hover:text-blue-400"
                      aria-label={`${isExpanded ? 'Hide' : 'Show'} message for ${alert.asset_name}`}
                      aria-expanded={isExpanded}
                    >
                      {isExpanded ? (
                        <ChevronDown className="w-4 h-4" />
                      ) : (
                        <ChevronRight className="w-4 h-4" />
                      )}
                    </button>
                  </td>
                  {!compact && (
                    <td className="py-2 px-3">
                      <div className="text-gray-900 dark:text-gray-100">{alert.asset_name}</div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {alert.asset_external_key}
                      </div>
                    </td>
                  )}
                  <td className="py-2 px-3 text-gray-700 dark:text-gray-300 whitespace-nowrap">
                    {formatSent(alert.sent_at)}
                  </td>
                  <td className="px-3">
                    <TypeBadge type={alert.type} />
                  </td>
                  <td className="px-3 text-gray-700 dark:text-gray-300">{alert.recipient_name}</td>
                  <td className="px-3">
                    <ChannelLabel channel={alert.channel} />
                  </td>
                </tr>
                {isExpanded && (
                  <tr
                    data-testid={`alert-expanded-${alert.id}`}
                    className="border-b border-gray-100 dark:border-gray-800 bg-gray-50/50 dark:bg-gray-800/30"
                  >
                    <td colSpan={columnCount} className="px-3 py-4">
                      <AlertMessage alert={alert} />
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>

      <div className="md:hidden divide-y divide-gray-100 dark:divide-gray-800">
        {alerts.map((alert) => {
          const isExpanded = expandedId === alert.id;
          return (
            <div key={alert.id} data-testid={`alert-card-${alert.id}`} className="p-3 space-y-2">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  {!compact && (
                    <>
                      <div className="font-medium text-gray-900 dark:text-gray-100 truncate">
                        {alert.asset_name}
                      </div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {alert.asset_external_key}
                      </div>
                    </>
                  )}
                  <div className="text-xs text-gray-600 dark:text-gray-400">
                    {formatSent(alert.sent_at)}
                  </div>
                </div>
                <TypeBadge type={alert.type} />
              </div>
              <div className="flex items-center justify-between gap-2 text-sm">
                <span className="text-gray-700 dark:text-gray-300 truncate">
                  {alert.recipient_name}
                </span>
                <ChannelLabel channel={alert.channel} />
              </div>
              <button
                type="button"
                onClick={() => toggle(alert.id)}
                aria-expanded={isExpanded}
                className="text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
              >
                {isExpanded ? 'Hide message' : 'Show message'}
              </button>
              {isExpanded && <AlertMessage alert={alert} />}
            </div>
          );
        })}
      </div>
    </>
  );
}
