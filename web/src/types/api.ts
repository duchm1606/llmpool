// API Types for LLMPool Internal Dashboard

export interface OverviewStats {
  total_requests: number;
  total_tokens: number;
  total_cost: number;
  avg_latency_ms: number;
  error_rate: number;
  period: string;
  last_updated_at?: string;
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
  tokens_out: number;
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
  tokens_out: number;
  total_tokens: number;
  cost: number;
  error_count: number;
  canceled_count: number;
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
