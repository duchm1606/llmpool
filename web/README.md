# LLMPool Internal Dashboard

A Next.js dashboard for monitoring LLMPool usage metrics, built with App Router, Tailwind CSS, and Highcharts.

## Features

- **Overview Cards**: Real-time stats for requests, tokens, cost, latency, and error rates
- **Activity Heatmap**: GitHub-style contribution graph showing daily activity over the past year
- **Usage Charts**: Interactive line/area charts with hourly/daily granularity toggle
- **Model Statistics**: Sortable table showing per-model usage breakdown
- **Audit Trail**: Filterable, paginated request log with search capabilities

## Prerequisites

- Node.js 18.17 or later
- npm, yarn, or pnpm

## Installation

```bash
cd web
npm install
```

## Running Locally

### Development Mode (with mock data)

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

### Development Mode (with backend)

```bash
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080 NEXT_PUBLIC_USE_MOCK_DATA=false npm run dev
```

### Production Build

```bash
npm run build
npm start
```

## Running with Docker Compose (from repo root)

```bash
docker compose up --build
```

Then open:
- Dashboard: `http://localhost:3000`
- API: `http://localhost:8080`

## Configuration

Environment variables can be set in `.env.local`:

```env
# Backend API base URL (default: http://localhost:8080)
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080

# Use mock data instead of real API (default: true for development)
NEXT_PUBLIC_USE_MOCK_DATA=false
```

## Project Structure

```
web/
├── src/
│   ├── app/
│   │   ├── globals.css      # Global styles + Tailwind imports
│   │   ├── layout.tsx       # Root layout
│   │   └── page.tsx         # Main dashboard page
│   ├── components/
│   │   ├── ActivityHeatmap.tsx   # GitHub-style heatmap
│   │   ├── AuditTrailTable.tsx   # Audit log with filters
│   │   ├── ModelStatsTable.tsx   # Per-model statistics
│   │   ├── OverviewCards.tsx     # KPI cards
│   │   └── UsageChart.tsx        # Time series chart
│   ├── lib/
│   │   ├── api.ts           # API client
│   │   ├── mock-data.ts     # Mock data generators
│   │   └── utils.ts         # Formatting utilities
│   └── types/
│       └── api.ts           # TypeScript interfaces
├── package.json
├── tailwind.config.js
├── tsconfig.json
└── next.config.mjs
```

## Backend API Endpoints (Expected)

The dashboard expects these endpoints under `/v1/internal/usage/*`:

### `GET /v1/internal/usage/stats`
Query params: `period` (`today`, `7d`, `30d`, `90d`, `365d`)

Response contains:
- `overview`
- `daily_stats` (heatmap source)
- `hourly_stats`
- `model_stats`
- `credential_stats`

The dashboard adapts this payload to overview cards, heatmap, charts, and model table.

### `GET /v1/internal/usage/audit`
Query params: `model`, `provider`, `credential_id`, `status`, `start_time`, `end_time`, `limit`, `offset`

Response:
```json
{
  "data": [
    {
      "id": "...",
      "request_id": "...",
      "model": "claude-opus-4-5",
      "provider": "copilot",
      "credential_id": "cred-...",
      "prompt_tokens": 500,
      "completion_tokens": 200,
      "total_price_micros": 3500,
      "duration_ms": 234,
      "status": "done"
    }
  ],
  "total": 1247,
  "limit": 20,
  "offset": 0,
  "page_size": 20,
  "total_pages": 63
}
```

## Development Notes

- Mock data is disabled by default; set `NEXT_PUBLIC_USE_MOCK_DATA=true` to enable
- Charts use Highcharts (free for non-commercial use)
- Responsive design works on both desktop and mobile
- All data fetching happens client-side for real-time updates
