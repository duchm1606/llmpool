import {
  OverviewStats,
  HeatmapDataPoint,
  TimeSeriesPoint,
  ModelStats,
  CredentialStats,
  CredentialProfile,
  CopilotUsage,
  AuditResponse,
  AuditFilters,
} from '@/types/api';

// Generate mock data for development/demo purposes
export function generateMockOverview(): OverviewStats {
  return {
    total_requests: 125847,
    total_tokens: 45672890,
    total_cost: 342.58,
    avg_latency_ms: 234,
    error_rate: 0.023,
    period: '24h',
    last_updated_at: new Date().toISOString(),
    total_cached_tokens: 9234512,
  };
}

export function generateMockHeatmap(days: number = 365): HeatmapDataPoint[] {
  const data: HeatmapDataPoint[] = [];
  const today = new Date();

  for (let i = days - 1; i >= 0; i--) {
    const date = new Date(today);
    date.setDate(date.getDate() - i);
    const dateStr = date.toISOString().split('T')[0];

    // Generate realistic-looking activity patterns
    const dayOfWeek = date.getDay();
    const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;
    const baseCount = isWeekend ? 50 : 200;
    const variance = Math.random() * 150;
    const count = Math.floor(baseCount + variance);

    data.push({ date: dateStr, count });
  }

  return data;
}

export function generateMockTimeSeries(granularity: 'hourly' | 'daily', days: number): TimeSeriesPoint[] {
  const data: TimeSeriesPoint[] = [];
  const now = new Date();

  if (granularity === 'hourly') {
    const hours = Math.min(days * 24, 168); // Max 7 days of hourly data
    for (let i = hours - 1; i >= 0; i--) {
      const timestamp = new Date(now);
      timestamp.setHours(timestamp.getHours() - i);
      timestamp.setMinutes(0, 0, 0);

      const hour = timestamp.getHours();
      const isBusinessHours = hour >= 9 && hour <= 18;
      const baseRequests = isBusinessHours ? 500 : 100;

      data.push({
        timestamp: timestamp.toISOString(),
        requests: Math.floor(baseRequests + Math.random() * 200),
        tokens: Math.floor((baseRequests + Math.random() * 200) * 350),
        cost: Number(((baseRequests + Math.random() * 200) * 0.002).toFixed(2)),
      });
    }
  } else {
    for (let i = days - 1; i >= 0; i--) {
      const timestamp = new Date(now);
      timestamp.setDate(timestamp.getDate() - i);
      timestamp.setHours(0, 0, 0, 0);

      const dayOfWeek = timestamp.getDay();
      const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;
      const baseRequests = isWeekend ? 2000 : 5000;

      data.push({
        timestamp: timestamp.toISOString(),
        requests: Math.floor(baseRequests + Math.random() * 2000),
        tokens: Math.floor((baseRequests + Math.random() * 2000) * 350),
        cost: Number(((baseRequests + Math.random() * 2000) * 0.002).toFixed(2)),
      });
    }
  }

  return data;
}

export function generateMockModelStats(): ModelStats[] {
  const models = [
    { model: 'claude-opus-4-5', provider: 'anthropic' },
    { model: 'claude-opus-4-6', provider: 'anthropic' },
  ];

  return models.map((m) => {
    const tokensIn = Math.floor(500000 + Math.random() * 2000000);
    const cachedTokens = Math.floor(100000 + Math.random() * 900000);
    const tokensOut = Math.floor(200000 + Math.random() * 800000);

    return {
      ...m,
      requests: Math.floor(5000 + Math.random() * 20000),
      tokens_in: tokensIn,
      cached_tokens: cachedTokens,
      tokens_out: tokensOut,
      total_tokens: tokensIn + cachedTokens + tokensOut,
      cost: Number((50 + Math.random() * 150).toFixed(2)),
      avg_latency_ms: Math.floor(100 + Math.random() * 400),
      error_count: Math.floor(Math.random() * 30),
      canceled_count: Math.floor(Math.random() * 20),
    };
  });
}

export function generateMockCredentialStats(): CredentialStats[] {
  const credentials = [
    {
      credential_type: 'copilot',
      credential_account_id: 'acct-copilot-01',
      credential_id: 'cred-copilot-001',
    },
    {
      credential_type: 'codex',
      credential_account_id: 'acct-codex-01',
      credential_id: 'cred-codex-001',
    },
    {
      credential_type: 'copilot',
      credential_account_id: 'acct-copilot-02',
      credential_id: 'cred-copilot-002',
    },
  ];

  return credentials.map((credential) => {
    const tokensIn = Math.floor(500000 + Math.random() * 1500000);
    const cachedTokens = Math.floor(80000 + Math.random() * 500000);
    const tokensOut = Math.floor(250000 + Math.random() * 1000000);
    return {
      credential_id: credential.credential_id,
      credential_type: credential.credential_type,
      credential_account_id: credential.credential_account_id,
      requests: Math.floor(3000 + Math.random() * 12000),
      tokens_in: tokensIn,
      cached_tokens: cachedTokens,
      tokens_out: tokensOut,
      total_tokens: tokensIn + cachedTokens + tokensOut,
      cost: Number((35 + Math.random() * 120).toFixed(2)),
      error_count: Math.floor(Math.random() * 40),
      canceled_count: Math.floor(Math.random() * 30),
    };
  });
}

export function generateMockCredentialProfiles(): CredentialProfile[] {
  const now = Date.now();
  return [
    {
      id: 'cred-copilot-001',
      type: 'copilot',
      account_id: 'octocat',
      email: 'octocat@github.test',
      enabled: true,
      expired: new Date(now + 45 * 60 * 1000).toISOString(),
      last_refresh_at: new Date(now - 5 * 60 * 1000).toISOString(),
    },
    {
      id: 'cred-copilot-002',
      type: 'copilot',
      account_id: 'hubot',
      email: 'hubot@github.test',
      enabled: true,
      expired: new Date(now + 25 * 60 * 1000).toISOString(),
      last_refresh_at: new Date(now - 10 * 60 * 1000).toISOString(),
    },
  ];
}

export function generateMockCopilotUsages(): CopilotUsage[] {
  return [
    {
      credential_id: 'cred-copilot-001',
      login: 'octocat',
      quota_reset_date: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
      quota_reset_date_utc: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
      fetched_at: new Date().toISOString(),
      quota_snapshots: {
        premium_interactions: {
          entitlement: 300,
          remaining: 238,
          percent_remaining: 79,
          quota_id: 'premium_interactions',
          unlimited: false,
        },
      },
    },
    {
      credential_id: 'cred-copilot-002',
      login: 'hubot',
      quota_reset_date: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
      quota_reset_date_utc: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
      fetched_at: new Date().toISOString(),
      quota_snapshots: {
        premium_interactions: {
          entitlement: 300,
          remaining: 121,
          percent_remaining: 40.33,
          quota_id: 'premium_interactions',
          unlimited: false,
        },
      },
    },
  ];
}

export function generateMockAuditTrail(filters: AuditFilters): AuditResponse {
  const page = filters.page || 1;
  const pageSize = filters.page_size || 20;
  const total = 1247;

  const models = ['claude-opus-4-5', 'claude-opus-4-6'];
  const providers = ['copilot'];
  const credentials = ['cred-opus-001', 'cred-opus-002', 'cred-opus-003'];

  const entries = Array.from({ length: pageSize }, (_, i) => {
    const isError = Math.random() < 0.05;
    const timestamp = new Date();
    timestamp.setMinutes(timestamp.getMinutes() - (page - 1) * pageSize - i);

    return {
      id: `req-${Date.now()}-${i}`,
      request_id: `request-${Date.now()}-${i}`,
      timestamp: timestamp.toISOString(),
      model: models[Math.floor(Math.random() * models.length)],
      provider: providers[Math.floor(Math.random() * providers.length)],
      credential_id: credentials[Math.floor(Math.random() * credentials.length)],
      tokens_in: Math.floor(100 + Math.random() * 2000),
      tokens_out: Math.floor(50 + Math.random() * 1000),
      cost: Number((0.001 + Math.random() * 0.05).toFixed(4)),
      latency_ms: Math.floor(50 + Math.random() * 500),
      status: isError ? 'failed' : Math.random() < 0.02 ? 'canceled' : 'done',
      error_message: isError ? 'Rate limit exceeded' : undefined,
    } as const;
  });

  return {
    entries,
    total,
    page,
    page_size: pageSize,
    total_pages: Math.ceil(total / pageSize),
  };
}
