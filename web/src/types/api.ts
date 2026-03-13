// API Types for LLMPool Internal Dashboard

export interface OverviewStats {
  total_requests: number;
  total_tokens: number;
  total_cost: number;
  avg_latency_ms: number;
  error_rate: number;
  period: string;
  last_updated_at?: string;
  total_cached_tokens?: number;
}

export interface HeatmapDataPoint {
  date: string; // YYYY-MM-DD
  count: number;
}

export interface TimeSeriesPoint {
  timestamp: string; // ISO8601
  requests: number;
  tokens: number;
  cost: number;
}

export interface ModelStats {
  model: string;
  provider: string;
  requests: number;
  tokens_in: number;
  cached_tokens: number;
  tokens_out: number;
  total_tokens: number;
  cost: number;
  avg_latency_ms: number;
  error_count: number;
  canceled_count?: number;
}

export interface CredentialStats {
  credential_id: string;
  credential_type: string;
  credential_account_id: string;
  requests: number;
  tokens_in: number;
  cached_tokens: number;
  tokens_out: number;
  total_tokens: number;
  cost: number;
  error_count: number;
  canceled_count: number;
}

export interface CredentialProfile {
  id: string;
  type: string;
  account_id: string;
  email: string;
  enabled: boolean;
  expired: string;
  last_refresh_at: string;
}

export interface CredentialStatusUpdateResponse {
  id: string;
  enabled: boolean;
  expired?: string;
  last_refresh_at?: string;
}

export interface CopilotDeviceCodeResponse {
  status: 'ok' | 'error';
  device_code?: string;
  user_code?: string;
  verification_uri?: string;
  expires_in?: number;
  interval?: number;
  error?: string;
}

export interface CopilotDeviceStatusResponse {
  status: 'ok' | 'wait' | 'error';
  account_id?: string;
  error_code?: string;
  error_message?: string;
  slow_down?: boolean;
}

export interface CopilotQuotaSnapshot {
  entitlement: number;
  remaining: number;
  percent_remaining: number;
  quota_id: string;
  unlimited: boolean;
}

export interface CopilotQuotaSnapshots {
  chat?: CopilotQuotaSnapshot;
  completions?: CopilotQuotaSnapshot;
  premium_interactions?: CopilotQuotaSnapshot;
}

export interface SessionQuotaUsage {
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
}

export interface CopilotUsage {
  credential_id: string;
  login?: string;
  quota_reset_date?: string;
  quota_reset_date_utc?: string;
  quota_snapshots?: CopilotQuotaSnapshots;
  session_quota?: SessionQuotaUsage;
  fetched_at: string;
}

export interface AuditEntry {
  id: string;
  request_id: string;
  timestamp: string;
  model: string;
  provider: string;
  credential_id: string;
  tokens_in: number;
  tokens_out: number;
  cost: number;
  latency_ms: number;
  status: 'done' | 'failed' | 'canceled';
  error_message?: string;
}

export interface AuditResponse {
  entries: AuditEntry[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface AuditFilters {
  model?: string;
  provider?: string;
  credential_id?: string;
  status?: 'done' | 'failed' | 'canceled';
  start_date?: string;
  end_date?: string;
  page?: number;
  page_size?: number;
}
