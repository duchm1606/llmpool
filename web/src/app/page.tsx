'use client';

import { useState, useEffect, useCallback } from 'react';
import { OverviewCards } from '@/components/OverviewCards';
import { ActivityHeatmap } from '@/components/ActivityHeatmap';
import { UsageChart } from '@/components/UsageChart';
import { ModelStatsTable } from '@/components/ModelStatsTable';
import { CredentialManagementTable } from '@/components/CredentialManagementTable';
import { AuditTrailTable } from '@/components/AuditTrailTable';
import {
  OverviewStats,
  HeatmapDataPoint,
  TimeSeriesPoint,
  ModelStats,
  CredentialProfile,
  CopilotUsage,
  AuditResponse,
  AuditFilters,
} from '@/types/api';
import { apiClient } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import {
  generateMockOverview,
  generateMockHeatmap,
  generateMockTimeSeries,
  generateMockModelStats,
  generateMockCredentialProfiles,
  generateMockCopilotUsages,
  generateMockAuditTrail,
} from '@/lib/mock-data';

// Set to true to use mock data (for development without backend)
const USE_MOCK_DATA = process.env.NEXT_PUBLIC_USE_MOCK_DATA === 'true';

export default function DashboardPage() {
  const [overview, setOverview] = useState<OverviewStats | null>(null);
  const [heatmap, setHeatmap] = useState<HeatmapDataPoint[]>([]);
  const [timeSeries, setTimeSeries] = useState<TimeSeriesPoint[]>([]);
  const [modelStats, setModelStats] = useState<ModelStats[]>([]);
  const [credentialProfiles, setCredentialProfiles] = useState<CredentialProfile[]>([]);
  const [copilotUsages, setCopilotUsages] = useState<CopilotUsage[]>([]);
  const [auditData, setAuditData] = useState<AuditResponse | null>(null);

  const [granularity, setGranularity] = useState<'hourly' | 'daily'>('daily');
  const [auditFilters, setAuditFilters] = useState<AuditFilters>({ page: 1, page_size: 20 });

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch all data
  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      if (USE_MOCK_DATA) {
        // Use mock data
        setOverview(generateMockOverview());
        setHeatmap(generateMockHeatmap(365));
        setTimeSeries(generateMockTimeSeries(granularity, granularity === 'hourly' ? 7 : 30));
        setModelStats(generateMockModelStats());
        setCredentialProfiles(generateMockCredentialProfiles());
        setCopilotUsages(generateMockCopilotUsages());
        setAuditData(generateMockAuditTrail(auditFilters));
      } else {
        // Fetch from API
        const [
          overviewData,
          heatmapData,
          timeSeriesData,
          modelStatsData,
          credentialProfilesData,
          copilotUsagesData,
          auditTrailData,
        ] =
          await Promise.all([
            apiClient.getOverview('24h'),
            apiClient.getHeatmap(365),
            apiClient.getTimeSeries(granularity, granularity === 'hourly' ? 7 : 30),
            apiClient.getModelStats('24h'),
            apiClient.getCredentialProfiles(),
            apiClient.getCopilotUsages(),
            apiClient.getAuditTrail(auditFilters),
          ]);

        setOverview(overviewData);
        setHeatmap(heatmapData);
        setTimeSeries(timeSeriesData);
        setModelStats(modelStatsData);
        setCredentialProfiles(credentialProfilesData);
        setCopilotUsages(copilotUsagesData);
        setAuditData(auditTrailData);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch data');
    } finally {
      setLoading(false);
    }
  }, [granularity, auditFilters]);

  // Initial fetch
  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Fetch time series when granularity changes
  const handleGranularityChange = useCallback((newGranularity: 'hourly' | 'daily') => {
    setGranularity(newGranularity);
  }, []);

  // Fetch audit data when filters change
  const handleAuditFiltersChange = useCallback((newFilters: AuditFilters) => {
    setAuditFilters(newFilters);
  }, []);

  if (loading && !overview) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  if (error && !overview) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <p className="text-red-600 mb-4">{error}</p>
          <button
            onClick={fetchData}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="bg-white border-b border-gray-200">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-xl font-semibold text-gray-900">LLMPool Dashboard</h1>
              <p className="text-sm text-gray-500">
                Internal usage analytics
                {overview?.last_updated_at ? ` · Last updated ${formatDateTime(overview.last_updated_at)}` : ''}
              </p>
            </div>
            <button
              onClick={fetchData}
              disabled={loading}
              className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50"
            >
              {loading ? 'Refreshing...' : 'Refresh'}
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-6">
        {/* Overview Cards */}
        {overview && <OverviewCards stats={overview} />}

        {/* Activity Heatmap */}
        {heatmap.length > 0 && <ActivityHeatmap data={heatmap} />}

        {/* Usage Chart */}
        {timeSeries.length > 0 && (
          <UsageChart
            data={timeSeries}
            granularity={granularity}
            onGranularityChange={handleGranularityChange}
          />
        )}

        {/* Model Stats Table */}
        {modelStats.length > 0 && <ModelStatsTable data={modelStats} />}

        {/* Credential Management */}
        <CredentialManagementTable
          profiles={credentialProfiles}
          usages={copilotUsages}
          loading={loading}
          onDataRefresh={fetchData}
        />

        {/* Audit Trail Table */}
        {auditData && (
          <AuditTrailTable
            data={auditData}
            filters={auditFilters}
            onFiltersChange={handleAuditFiltersChange}
          />
        )}
      </main>

      {/* Footer */}
      <footer className="bg-white border-t border-gray-200 mt-8">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <p className="text-sm text-gray-500 text-center">
            LLMPool Internal Dashboard &middot; Data refreshes on demand
          </p>
        </div>
      </footer>
    </div>
  );
}
