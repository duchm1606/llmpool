'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiClient } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import { CopilotDeviceCodeResponse, CopilotDeviceStatusResponse, CopilotUsage, CredentialProfile } from '@/types/api';

type ToastKind = 'success' | 'error' | 'info';

type ToastItem = {
  id: number;
  kind: ToastKind;
  message: string;
};

type DeviceSession = {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  intervalSec: number;
  expiresAt: number;
};

type QuotaView = {
  id: string;
  percent: number | null;
  label: string;
  sublabel?: string;
};

const POLL_FALLBACK_INTERVAL_MS = 5_000;

interface CredentialManagementTableProps {
  profiles: CredentialProfile[];
  usages: CopilotUsage[];
  loading: boolean;
  onDataRefresh: () => Promise<void>;
}

function quotaFromUsage(usage?: CopilotUsage): QuotaView {
  const snapshots = usage?.quota_snapshots;
  const selected = snapshots?.premium_interactions || snapshots?.chat || snapshots?.completions;

  if (!selected) {
    return {
      id: 'unknown',
      percent: null,
      label: 'N/A',
    };
  }

  if (selected.unlimited) {
    return {
      id: selected.quota_id || 'unlimited',
      percent: 100,
      label: 'Unlimited',
    };
  }

  const total = Number(selected.entitlement) || 0;
  const remaining = Number(selected.remaining) || 0;
  const used = Math.max(total - remaining, 0);
  const percent = total > 0 ? Math.max(0, Math.min((used / total) * 100, 100)) : null;

  return {
    id: selected.quota_id || 'quota',
    percent,
    label: total > 0 ? `${used}/${total}` : 'N/A',
  };
}

function sessionQuotaFromUsage(usage?: CopilotUsage): QuotaView {
	const sessionQuota = usage?.session_quota;
	if (!sessionQuota) {
		return {
			id: 'llmpool-session',
			percent: null,
			label: 'N/A',
		};
	}

	const total = Number(sessionQuota.requests_per_session) || 0;
	const used = Number(sessionQuota.requests_this_session) || 0;
	const percent = total > 0 ? Math.max(0, Math.min((used / total) * 100, 100)) : null;

	return {
		id: 'llmpool-session',
		percent,
		label: total > 0 ? `${used}/${total}` : 'N/A',
		sublabel: formatSessionResetLabel(sessionQuota.window_end_utc),
	};
}

function formatSessionResetLabel(windowEnd?: string): string | undefined {
	if (!windowEnd) {
		return undefined;
	}

	const endMs = new Date(windowEnd).getTime();
	if (Number.isNaN(endMs)) {
		return undefined;
	}

	const remainingMs = Math.max(0, endMs - Date.now());
	const totalMinutes = Math.floor(remainingMs / 60000);
	const hours = Math.floor(totalMinutes / 60);
	const minutes = totalMinutes % 60;

	return `Reset in ${hours}h ${minutes}m`;
}

function statusErrorMessage(resp: CopilotDeviceStatusResponse): string {
  if (resp.error_message) {
    return resp.error_message;
  }
  if (resp.error_code) {
    switch (resp.error_code) {
      case 'expired_token':
        return 'Device code expired. Please start again.';
      case 'access_denied':
        return 'Authorization was denied by GitHub.';
      case 'no_subscription':
        return 'GitHub Copilot subscription is required.';
      default:
        return `Authentication failed (${resp.error_code}).`;
    }
  }
  return 'Authentication failed.';
}

function isStartResponseValid(resp: CopilotDeviceCodeResponse): resp is Required<
  Pick<CopilotDeviceCodeResponse, 'device_code' | 'user_code' | 'verification_uri' | 'expires_in'>
> &
  CopilotDeviceCodeResponse {
  return Boolean(resp.device_code && resp.user_code && resp.verification_uri && resp.expires_in);
}

function ToastStack({ items }: { items: ToastItem[] }) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="fixed top-4 right-4 z-50 space-y-2">
      {items.map((toast) => (
        <div
          key={toast.id}
          className={`min-w-64 max-w-md rounded-md border px-3 py-2 text-sm shadow-md ${
            toast.kind === 'success'
              ? 'border-emerald-300 bg-emerald-50 text-emerald-800'
              : toast.kind === 'error'
              ? 'border-red-300 bg-red-50 text-red-800'
              : 'border-blue-300 bg-blue-50 text-blue-800'
          }`}
        >
          {toast.message}
        </div>
      ))}
    </div>
  );
}

export function CredentialManagementTable({ profiles, usages, loading, onDataRefresh }: CredentialManagementTableProps) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [creating, setCreating] = useState(false);
  const [polling, setPolling] = useState(false);
  const [activeDevice, setActiveDevice] = useState<DeviceSession | null>(null);
  const [reauthCredentialId, setReauthCredentialId] = useState<string | null>(null);
  const [statusUpdatingId, setStatusUpdatingId] = useState<string | null>(null);
  const [quotaRefreshingId, setQuotaRefreshingId] = useState<string | null>(null);
  const [nowMs, setNowMs] = useState<number>(Date.now());

  const pollTimeoutRef = useRef<number | null>(null);
  const pollingInFlightRef = useRef(false);
  const toastCounterRef = useRef(0);
  const activeDeviceRef = useRef<DeviceSession | null>(null);

  const pushToast = useCallback((kind: ToastKind, message: string) => {
    toastCounterRef.current += 1;
    const id = toastCounterRef.current;
    setToasts((prev) => [...prev, { id, kind, message }]);
    window.setTimeout(() => {
      setToasts((prev) => prev.filter((item) => item.id !== id));
    }, 4000);
  }, []);

  const stopPolling = useCallback(() => {
    if (pollTimeoutRef.current !== null) {
      window.clearTimeout(pollTimeoutRef.current);
      pollTimeoutRef.current = null;
    }
    pollingInFlightRef.current = false;
    setPolling(false);
  }, []);

  useEffect(() => {
    activeDeviceRef.current = activeDevice;
  }, [activeDevice]);

  const usageByCredential = useMemo(() => {
    const map = new Map<string, CopilotUsage>();
    for (const usage of usages) {
      if (usage.credential_id) {
        map.set(usage.credential_id, usage);
      }
    }
    return map;
  }, [usages]);

  const rows = useMemo(() => {
    return profiles
      .filter((profile) => profile.type === 'copilot')
      .map((profile) => {
        const usage = usageByCredential.get(profile.id);
        const quota = quotaFromUsage(usage);
        const sessionQuota = sessionQuotaFromUsage(usage);
        return {
          profile,
          usage,
          quota,
          sessionQuota,
        };
      })
      .sort((a, b) => {
        if (a.profile.account_id && b.profile.account_id) {
          return a.profile.account_id.localeCompare(b.profile.account_id);
        }
        return a.profile.id.localeCompare(b.profile.id);
      });
  }, [profiles, usageByCredential]);

  useEffect(() => {
    return () => {
      if (pollTimeoutRef.current !== null) {
        window.clearTimeout(pollTimeoutRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (!activeDevice) {
      return;
    }
    const timer = window.setInterval(() => {
      setNowMs(Date.now());
    }, 1000);

    return () => {
      window.clearInterval(timer);
    };
  }, [activeDevice]);

  const scheduleNextPoll = useCallback(
    (device: DeviceSession, delayMs: number) => {
      if (pollTimeoutRef.current !== null) {
        window.clearTimeout(pollTimeoutRef.current);
        pollTimeoutRef.current = null;
      }

      pollTimeoutRef.current = window.setTimeout(async () => {
        const currentDevice = activeDeviceRef.current;
        if (!currentDevice || currentDevice.deviceCode !== device.deviceCode || pollingInFlightRef.current) {
          return;
        }

        if (Date.now() >= currentDevice.expiresAt) {
          stopPolling();
          activeDeviceRef.current = null;
          setActiveDevice(null);
          setReauthCredentialId(null);
          pushToast('error', 'Device code expired. Please start again.');
          return;
        }

        pollingInFlightRef.current = true;
        try {
          const statusResp = await apiClient.getCopilotDeviceStatus(currentDevice.deviceCode);
          if (statusResp.status === 'ok') {
            stopPolling();
            activeDeviceRef.current = null;
            setActiveDevice(null);
            setReauthCredentialId(null);
            pushToast('success', `GitHub account connected: ${statusResp.account_id || 'success'}`);
            await onDataRefresh();
            return;
          }

          if (statusResp.status === 'error') {
            stopPolling();
            activeDeviceRef.current = null;
            setActiveDevice(null);
            setReauthCredentialId(null);
            pushToast('error', statusErrorMessage(statusResp));
            return;
          }

          const nextDelay = statusResp.slow_down
            ? Math.max((currentDevice.intervalSec + 5) * 1000, POLL_FALLBACK_INTERVAL_MS)
            : Math.max(currentDevice.intervalSec * 1000, POLL_FALLBACK_INTERVAL_MS);
          scheduleNextPoll(currentDevice, nextDelay);
        } catch (error) {
          stopPolling();
          activeDeviceRef.current = null;
          setActiveDevice(null);
          setReauthCredentialId(null);
          pushToast('error', error instanceof Error ? error.message : 'Polling failed');
        } finally {
          pollingInFlightRef.current = false;
        }
      }, delayMs);
    },
    [onDataRefresh, pushToast, stopPolling]
  );

  const startDeviceFlow = useCallback(
    async (credentialId?: string) => {
      setCreating(true);
      stopPolling();
      activeDeviceRef.current = null;
      setActiveDevice(null);

      try {
        const resp = await apiClient.startCopilotDeviceFlow();
        if (resp.status !== 'ok') {
          throw new Error(resp.error || 'Failed to start device flow');
        }
        if (!isStartResponseValid(resp)) {
          throw new Error('Invalid device flow response from server');
        }

        const intervalSec = resp.interval && resp.interval > 0 ? resp.interval : 5;
        const session: DeviceSession = {
          deviceCode: resp.device_code,
          userCode: resp.user_code,
          verificationUri: resp.verification_uri,
          intervalSec,
          expiresAt: Date.now() + resp.expires_in * 1000,
        };

        activeDeviceRef.current = session;
        setActiveDevice(session);
        setReauthCredentialId(credentialId || null);
        setPolling(true);
        pushToast('info', 'Device code created. Complete authentication in GitHub.');
        scheduleNextPoll(session, Math.max(intervalSec * 1000, POLL_FALLBACK_INTERVAL_MS));
      } catch (error) {
        pushToast('error', error instanceof Error ? error.message : 'Failed to start device flow');
      } finally {
        setCreating(false);
      }
    },
    [pushToast, scheduleNextPoll, stopPolling]
  );

  const handleToggleStatus = useCallback(
    async (profile: CredentialProfile) => {
      if (!profile.id || statusUpdatingId || creating || polling) {
        return;
      }

      setStatusUpdatingId(profile.id);
      const nextEnabled = !profile.enabled;
      try {
        await apiClient.updateCredentialStatus(profile.id, nextEnabled);
        await onDataRefresh();
        pushToast('success', `${profile.account_id || profile.id} ${nextEnabled ? 'enabled' : 'disabled'}.`);
      } catch (error) {
        pushToast('error', error instanceof Error ? error.message : 'Failed to update credential status');
      } finally {
        setStatusUpdatingId(null);
      }
    },
    [creating, onDataRefresh, polling, pushToast, statusUpdatingId]
  );

  const handleRefreshQuota = useCallback(
    async (profileId: string) => {
      if (!profileId || quotaRefreshingId || creating || polling) {
        return;
      }

      setQuotaRefreshingId(profileId);
      try {
        await apiClient.refreshCredentialQuota(profileId);
        await onDataRefresh();
        pushToast('success', `Quota refreshed for ${profileId}.`);
      } catch (error) {
        pushToast('error', error instanceof Error ? error.message : 'Failed to refresh quota');
      } finally {
        setQuotaRefreshingId(null);
      }
    },
    [creating, onDataRefresh, polling, pushToast, quotaRefreshingId]
  );

  const handleCopyAndOpenLogin = useCallback(async () => {
    if (!activeDevice) {
      return;
    }

    let copied = false;
    try {
      await navigator.clipboard.writeText(activeDevice.userCode);
      copied = true;
    } catch {
      copied = false;
    }

    const loginWindow = window.open(activeDevice.verificationUri, '_blank', 'noopener,noreferrer');
    if (!loginWindow) {
      pushToast('error', 'Could not open login page (popup blocked).');
      if (copied) {
        pushToast('info', 'Code copied to clipboard.');
      }
      return;
    }

    if (copied) {
      pushToast('info', 'Code copied and login page opened.');
      return;
    }

    pushToast('error', 'Login page opened, but code copy failed.');
  }, [activeDevice, pushToast]);

  const activeExpiresInSec = activeDevice ? Math.max(0, Math.floor((activeDevice.expiresAt - nowMs) / 1000)) : 0;

  return (
    <>
      <ToastStack items={toasts} />

      <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-medium text-gray-700">Credential Management</h3>
            <p className="text-xs text-gray-500 mt-0.5">Connect GitHub Copilot accounts and monitor quota.</p>
          </div>
          <button
            onClick={() => startDeviceFlow()}
            disabled={creating || polling}
            className="px-3 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50"
          >
            {creating ? 'Starting...' : 'Create Account'}
          </button>
        </div>

        {activeDevice && (
          <div className="px-4 py-3 border-b border-blue-100 bg-blue-50">
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
              <div className="space-y-1">
                <p className="text-sm text-blue-900">Use one click to copy the code and open the login page:</p>
                <p className="text-2xl tracking-widest font-semibold text-blue-900">{activeDevice.userCode}</p>
                <p className="text-xs text-blue-800">
                  Expires in {activeExpiresInSec}s{reauthCredentialId ? ` • Re-auth for ${reauthCredentialId}` : ''}
                </p>
                <p className="text-xs text-blue-700">{activeDevice.verificationUri}</p>
              </div>
              <div className="flex items-center">
                <button
                  onClick={handleCopyAndOpenLogin}
                  className="px-3 py-1.5 text-xs font-medium text-white bg-blue-700 rounded-md hover:bg-blue-800"
                >
                  Copy Code + Open Login
                </button>
                <button
                  onClick={() => {
                    stopPolling();
                    activeDeviceRef.current = null;
                    setActiveDevice(null);
                    setReauthCredentialId(null);
                  }}
                  className="ml-2 px-3 py-1.5 text-xs font-medium text-blue-700 bg-white border border-blue-200 rounded-md hover:bg-blue-100"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        )}

        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Account ID</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Expire Time</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Quota Status</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Action</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {rows.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-4 py-6 text-sm text-gray-500 text-center">
                    {loading ? 'Loading credentials...' : 'No GitHub Copilot credentials yet.'}
                  </td>
                </tr>
              )}

              {rows.map((row) => {
                const percent = row.quota.percent;
                const progressWidth = percent === null ? '0%' : `${percent}%`;
                const showReauthLoading = creating && reauthCredentialId === row.profile.id;
                const showStatusLoading = statusUpdatingId === row.profile.id;
                const showQuotaLoading = quotaRefreshingId === row.profile.id;

                return (
                  <tr key={row.profile.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                      <div className="font-medium">{row.profile.account_id || '-'}</div>
                      <div className="text-xs text-gray-500 font-mono mt-0.5">{row.profile.id}</div>
                      <div className="mt-1">
                        <span
                          className={`inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ${
                            row.profile.enabled
                              ? 'bg-emerald-100 text-emerald-700'
                              : 'bg-gray-100 text-gray-600'
                          }`}
                        >
                          {row.profile.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-600">
                      {row.profile.expired ? formatDateTime(row.profile.expired) : '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 min-w-56">
	                      <div className="space-y-3">
	                        <div>
	                          <div className="flex items-center justify-between text-xs mb-1">
	                            <span className="text-gray-500">{row.quota.id}</span>
	                            <span className="font-medium text-gray-700">{row.quota.label}</span>
	                          </div>
	                          <div className="h-2 rounded-full bg-gray-100 overflow-hidden">
	                            <div className="h-full bg-blue-500" style={{ width: progressWidth }} />
	                          </div>
	                        </div>
	                        <div>
	                          <div className="flex items-center justify-between text-xs mb-1">
	                            <span className="text-gray-500">{row.sessionQuota.id}</span>
	                            <span className="font-medium text-gray-700">{row.sessionQuota.label}</span>
	                          </div>
	                          {row.sessionQuota.sublabel && (
	                            <div className="text-[11px] text-gray-500 mb-1">{row.sessionQuota.sublabel}</div>
	                          )}
	                          <div className="h-2 rounded-full bg-gray-100 overflow-hidden">
	                            <div
	                              className="h-full bg-emerald-500"
	                              style={{ width: row.sessionQuota.percent === null ? '0%' : `${row.sessionQuota.percent}%` }}
	                            />
	                          </div>
	                        </div>
	                      </div>
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap text-sm">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => startDeviceFlow(row.profile.id)}
                          disabled={creating || polling || showStatusLoading || showQuotaLoading}
                          className="px-3 py-1.5 text-xs font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-md hover:bg-blue-100 disabled:opacity-50"
                        >
                          {showReauthLoading ? 'Starting...' : 'Re-auth'}
                        </button>
                        <button
                          onClick={() => handleRefreshQuota(row.profile.id)}
                          disabled={creating || polling || showStatusLoading || showQuotaLoading}
                          className="px-3 py-1.5 text-xs font-medium text-indigo-700 bg-indigo-50 border border-indigo-200 rounded-md hover:bg-indigo-100 disabled:opacity-50"
                        >
                          {showQuotaLoading ? 'Refreshing...' : 'Refresh Quota'}
                        </button>
                        <button
                          onClick={() => handleToggleStatus(row.profile)}
                          disabled={creating || polling || showStatusLoading || showQuotaLoading}
                          className="px-3 py-1.5 text-xs font-medium text-gray-700 bg-gray-50 border border-gray-200 rounded-md hover:bg-gray-100 disabled:opacity-50"
                        >
                          {showStatusLoading
                            ? 'Saving...'
                            : row.profile.enabled
                            ? 'Disable'
                            : 'Enable'}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}
