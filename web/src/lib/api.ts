import {
  OverviewStats,
  HeatmapDataPoint,
  TimeSeriesPoint,
  ModelStats,
  CredentialStats,
  CredentialProfile,
  CredentialStatusUpdateResponse,
  CopilotDeviceCodeResponse,
  CopilotDeviceStatusResponse,
  CopilotUsage,
  AuditResponse,
  AuditFilters,
} from '@/types/api';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8081';

type BackendOverview = {
  total_requests: number;
  total_prompt_tokens: number;
  total_cached_tokens: number;
  total_completion_tokens: number;
  total_tokens: number;
  total_price_micros: number;
  success_count: number;
  failed_count: number;
  canceled_count: number;
  avg_duration_ms: number;
};

type BackendDailyStats = {
  day: string;
  request_count: number;
  total_tokens: number;
  total_price_micros: number;
  success_count: number;
  failed_count: number;
};

type BackendHourlyStats = {
  hour: string;
  request_count: number;
  total_tokens: number;
  total_price_micros: number;
  success_count: number;
  failed_count: number;
};

type BackendModelStats = {
  model: string;
  request_count: number;
  prompt_tokens: number;
  cached_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_price_micros: number;
  success_count: number;
  failed_count: number;
  canceled_count: number;
};

type BackendDashboardStats = {
  overview: BackendOverview;
  hourly_stats: BackendHourlyStats[] | null;
  daily_stats: BackendDailyStats[] | null;
  model_stats: BackendModelStats[] | null;
  credential_stats:
    | {
        credential_id: string;
        credential_type: string;
        credential_account_id: string;
        request_count: number;
        prompt_tokens: number;
        cached_tokens: number;
        completion_tokens: number;
        total_tokens: number;
        total_price_micros: number;
        failed_count: number;
        canceled_count: number;
      }[]
    | null;
  generated_at?: string;
};

type BackendAuditLog = {
  id: string;
  request_id: string;
  model: string;
  provider: string;
  credential_id: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_price_micros: number;
  status: 'done' | 'failed' | 'canceled';
  error_message?: string;
  duration_ms: number;
  created_at: string;
};

type BackendAuditResponse = {
  data: BackendAuditLog[];
  total: number;
  limit: number;
  offset: number;
  page: number;
  page_size: number;
  total_pages: number;
};

type BackendCredentialProfile = {
  id: string;
  type: string;
  account_id: string;
  email: string;
  enabled: boolean;
  expired: string;
  last_refresh_at: string;
};

type BackendCredentialProfilesResponse = {
  data: BackendCredentialProfile[];
  count: number;
};

type BackendCredentialStatusUpdateResponse = {
  id: string;
  enabled: boolean;
  expired?: string;
  last_refresh_at?: string;
};

type BackendCopilotUsage = {
  credential_id: string;
  login?: string;
  quota_reset_date?: string;
  quota_reset_date_utc?: string;
  quota_snapshots?: {
    chat?: {
      entitlement: number;
      remaining: number;
      percent_remaining: number;
      quota_id: string;
      unlimited: boolean;
    };
    completions?: {
      entitlement: number;
      remaining: number;
      percent_remaining: number;
      quota_id: string;
      unlimited: boolean;
    };
    premium_interactions?: {
      entitlement: number;
      remaining: number;
      percent_remaining: number;
      quota_id: string;
      unlimited: boolean;
    };
  };
  session_quota?: {
    window_start_utc: string;
    window_end_utc: string;
    minute_window_start_utc: string;
    minute_window_end_utc: string;
    requests_per_minute: number;
    requests_this_minute: number;
    remaining_this_minute: number;
    requests_per_session: number;
    requests_this_session: number;
    remaining_this_session: number;
    first_initiator_used: boolean;
  };
  fetched_at: string;
};

type BackendCopilotUsagesResponse = {
  usages: BackendCopilotUsage[];
  count: number;
};

const microsToUSD = (micros: number): number => micros / 1_000_000;

function periodForDays(days: number): string {
  if (days <= 1) {
    return 'today';
  }
  if (days <= 7) {
    return '7d';
  }
  if (days <= 30) {
    return '30d';
  }
  if (days <= 90) {
    return '90d';
  }
  return '365d';
}

function inferProvider(model: string): string {
  const lower = model.toLowerCase();
  if (lower.includes('claude') || lower.includes('opus')) {
    return 'anthropic';
  }
  if (lower.includes('gpt') || lower.includes('o1') || lower.includes('o3')) {
    return 'openai';
  }
  if (lower.includes('gemini')) {
    return 'google';
  }
  return 'copilot';
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  private async fetch<T>(endpoint: string, options?: RequestInit): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
    });

    if (!response.ok) {
      throw new Error(`API Error: ${response.status} ${response.statusText}`);
    }

    return response.json();
  }

  async getOverview(period: string = '24h'): Promise<OverviewStats> {
    const mappedPeriod = period === '24h' ? 'today' : period;
    const stats = await this.fetch<BackendDashboardStats>(`/v1/internal/usage/stats?period=${mappedPeriod}`);
    const overview = stats.overview;
    const total = overview.total_requests || 0;

    return {
      total_requests: overview.total_requests,
      total_tokens: overview.total_tokens,
      total_cost: microsToUSD(overview.total_price_micros),
      avg_latency_ms: overview.avg_duration_ms,
      error_rate: total > 0 ? overview.failed_count / total : 0,
      period,
      last_updated_at: stats.generated_at,
      total_cached_tokens: overview.total_cached_tokens || 0,
    };
  }

  async getHeatmap(days: number = 365): Promise<HeatmapDataPoint[]> {
    const period = periodForDays(days);
    const stats = await this.fetch<BackendDashboardStats>(`/v1/internal/usage/stats?period=${period}`);
    return (stats.daily_stats || []).map((point) => ({
      date: point.day,
      count: point.request_count,
    }));
  }

  async getTimeSeries(granularity: 'hourly' | 'daily' = 'daily', days: number = 30): Promise<TimeSeriesPoint[]> {
    const period = granularity === 'hourly' ? (days <= 1 ? 'today' : '7d') : periodForDays(days);
    const stats = await this.fetch<BackendDashboardStats>(`/v1/internal/usage/stats?period=${period}`);
    if (granularity === 'hourly') {
      return (stats.hourly_stats || []).map((point) => ({
        timestamp: point.hour,
        requests: point.request_count,
        tokens: point.total_tokens,
        cost: microsToUSD(point.total_price_micros),
      }));
    }
    return (stats.daily_stats || []).map((point) => ({
      timestamp: point.day,
      requests: point.request_count,
      tokens: point.total_tokens,
      cost: microsToUSD(point.total_price_micros),
    }));
  }

  async getModelStats(period: string = '24h'): Promise<ModelStats[]> {
    const mappedPeriod = period === '24h' ? 'today' : period;
    const stats = await this.fetch<BackendDashboardStats>(`/v1/internal/usage/stats?period=${mappedPeriod}`);
    return (stats.model_stats || []).map((model) => ({
      model: model.model,
      provider: inferProvider(model.model),
      requests: model.request_count,
      tokens_in: model.prompt_tokens,
      cached_tokens: model.cached_tokens,
      tokens_out: model.completion_tokens,
      total_tokens: model.total_tokens,
      cost: microsToUSD(model.total_price_micros),
      avg_latency_ms: 0,
      error_count: model.failed_count,
      canceled_count: model.canceled_count,
    }));
  }

  async getCredentialStats(period: string = '24h'): Promise<CredentialStats[]> {
    const mappedPeriod = period === '24h' ? 'today' : period;
    const stats = await this.fetch<BackendDashboardStats>(`/v1/internal/usage/stats?period=${mappedPeriod}`);
    return (stats.credential_stats || []).map((credential) => ({
      credential_id: credential.credential_id,
      credential_type: credential.credential_type,
      credential_account_id: credential.credential_account_id,
      requests: credential.request_count,
      tokens_in: credential.prompt_tokens,
      cached_tokens: credential.cached_tokens,
      tokens_out: credential.completion_tokens,
      total_tokens: credential.total_tokens,
      cost: microsToUSD(credential.total_price_micros),
      error_count: credential.failed_count,
      canceled_count: credential.canceled_count,
    }));
  }

  async getCredentialProfiles(): Promise<CredentialProfile[]> {
    const payload = await this.fetch<BackendCredentialProfilesResponse>('/v1/internal/auth-profiles');
    return (payload.data || []).map((profile) => ({
      id: profile.id,
      type: profile.type,
      account_id: profile.account_id,
      email: profile.email,
      enabled: profile.enabled,
      expired: profile.expired,
      last_refresh_at: profile.last_refresh_at,
    }));
  }

  async updateCredentialStatus(credentialId: string, enabled: boolean): Promise<CredentialStatusUpdateResponse> {
    const payload = await this.fetch<BackendCredentialStatusUpdateResponse>(
      `/v1/internal/auth-profiles/${encodeURIComponent(credentialId)}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      }
    );

    return {
      id: payload.id,
      enabled: payload.enabled,
      expired: payload.expired,
      last_refresh_at: payload.last_refresh_at,
    };
  }

  async refreshCredentialQuota(credentialId: string): Promise<void> {
    const params = new URLSearchParams();
    params.set('credential_id', credentialId);
    await this.fetch<{ message: string }>(`/v1/internal/auth-profiles/refresh?${params.toString()}`, {
      method: 'POST',
    });
  }

  async getCopilotUsages(): Promise<CopilotUsage[]> {
    const payload = await this.fetch<BackendCopilotUsagesResponse>('/v1/internal/usage');
    return (payload.usages || []).map((usage) => ({
      credential_id: usage.credential_id,
      login: usage.login,
      quota_reset_date: usage.quota_reset_date,
      quota_reset_date_utc: usage.quota_reset_date_utc,
      quota_snapshots: usage.quota_snapshots,
      session_quota: usage.session_quota,
      fetched_at: usage.fetched_at,
    }));
  }

  async startCopilotDeviceFlow(): Promise<CopilotDeviceCodeResponse> {
    return this.fetch<CopilotDeviceCodeResponse>('/v1/internal/oauth/copilot-device-code', {
      method: 'POST',
    });
  }

  async getCopilotDeviceStatus(deviceCode: string): Promise<CopilotDeviceStatusResponse> {
    const params = new URLSearchParams();
    params.set('device_code', deviceCode);
    return this.fetch<CopilotDeviceStatusResponse>(`/v1/internal/oauth/copilot-device-status?${params.toString()}`);
  }

  async getAuditTrail(filters: AuditFilters = {}): Promise<AuditResponse> {
    const params = new URLSearchParams();
    const page = filters.page ?? 1;
    const pageSize = filters.page_size ?? 20;
    const offset = (page - 1) * pageSize;

    if (filters.model) {
      params.set('model', filters.model);
    }
    if (filters.provider) {
      params.set('provider', filters.provider);
    }
    if (filters.credential_id) {
      params.set('credential_id', filters.credential_id);
    }
    if (filters.status) {
      params.set('status', filters.status);
    }
    if (filters.start_date) {
      params.set('start_time', new Date(filters.start_date).toISOString());
    }
    if (filters.end_date) {
      params.set('end_time', new Date(filters.end_date).toISOString());
    }
    params.set('limit', String(pageSize));
    params.set('offset', String(offset));

    const query = params.toString();
    const payload = await this.fetch<BackendAuditResponse>(`/v1/internal/usage/audit${query ? `?${query}` : ''}`);
    return {
      entries: payload.data.map((entry) => ({
        id: entry.id,
        request_id: entry.request_id,
        timestamp: entry.created_at,
        model: entry.model,
        provider: entry.provider,
        credential_id: entry.credential_id,
        tokens_in: entry.prompt_tokens,
        tokens_out: entry.completion_tokens,
        cost: microsToUSD(entry.total_price_micros),
        latency_ms: entry.duration_ms,
        status: entry.status,
        error_message: entry.error_message,
      })),
      total: payload.total,
      page: payload.page,
      page_size: payload.page_size,
      total_pages: payload.total_pages,
    };
  }
}

export const apiClient = new ApiClient(API_BASE_URL);
