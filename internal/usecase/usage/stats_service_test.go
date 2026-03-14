package usage

import (
	"testing"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
)

func TestPeriodToTimeRangeFromNow(t *testing.T) {
	now := time.Date(2026, 3, 7, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		period    string
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{
			name:      "today",
			period:    "today",
			wantStart: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:      "7d includes today",
			period:    "7d",
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:      "30d includes today",
			period:    "30d",
			wantStart: time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:      "90d includes today",
			period:    "90d",
			wantStart: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:      "365d includes today",
			period:    "365d",
			wantStart: time.Date(2025, 3, 8, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:    "invalid",
			period:  "12h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := periodToTimeRangeFromNow(now, tt.period)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !start.Equal(tt.wantStart) {
				t.Fatalf("start = %v, want %v", start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Fatalf("end = %v, want %v", end, tt.wantEnd)
			}
		})
	}
}

func TestFillMissingDailyStats(t *testing.T) {
	t.Run("fills full range with zero days", func(t *testing.T) {
		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 5, 15, 0, 0, 0, time.UTC)

		input := []domainusage.DailyStats{
			{Day: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), RequestCount: 2, TotalTokens: 20},
			{Day: time.Date(2026, 3, 3, 8, 30, 0, 0, time.UTC), RequestCount: 5, TotalTokens: 50},
			{Day: time.Date(2026, 3, 5, 23, 0, 0, 0, time.UTC), RequestCount: 1, TotalTokens: 10},
		}

		filled := fillMissingDailyStats(start, end, input)
		if len(filled) != 5 {
			t.Fatalf("len = %d, want 5", len(filled))
		}

		for i := 0; i < 5; i++ {
			wantDay := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
			if !filled[i].Day.Equal(wantDay) {
				t.Fatalf("filled[%d].Day = %v, want %v", i, filled[i].Day, wantDay)
			}
		}

		if filled[0].RequestCount != 2 || filled[0].TotalTokens != 20 {
			t.Fatalf("day1 values changed: %+v", filled[0])
		}
		if filled[1].RequestCount != 0 || filled[1].TotalTokens != 0 {
			t.Fatalf("day2 should be zero-filled: %+v", filled[1])
		}
		if filled[2].RequestCount != 5 || filled[2].TotalTokens != 50 {
			t.Fatalf("day3 values changed: %+v", filled[2])
		}
		if filled[3].RequestCount != 0 || filled[3].TotalTokens != 0 {
			t.Fatalf("day4 should be zero-filled: %+v", filled[3])
		}
		if filled[4].RequestCount != 1 || filled[4].TotalTokens != 10 {
			t.Fatalf("day5 values changed: %+v", filled[4])
		}
	})

	t.Run("handles nil input", func(t *testing.T) {
		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 3, 1, 0, 0, 0, time.UTC)

		filled := fillMissingDailyStats(start, end, nil)
		if len(filled) != 3 {
			t.Fatalf("len = %d, want 3", len(filled))
		}
		for i := range filled {
			if filled[i].RequestCount != 0 || filled[i].TotalTokens != 0 {
				t.Fatalf("expected zero-filled row at %d, got %+v", i, filled[i])
			}
		}
	})

	t.Run("excludes end day when end time is midnight exclusive", func(t *testing.T) {
		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)

		filled := fillMissingDailyStats(start, end, nil)
		if len(filled) != 3 {
			t.Fatalf("len = %d, want 3", len(filled))
		}
		lastDay := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
		if !filled[len(filled)-1].Day.Equal(lastDay) {
			t.Fatalf("last day = %v, want %v", filled[len(filled)-1].Day, lastDay)
		}
	})

	t.Run("returns empty when end before start", func(t *testing.T) {
		start := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 4, 23, 0, 0, 0, time.UTC)

		filled := fillMissingDailyStats(start, end, nil)
		if len(filled) != 0 {
			t.Fatalf("len = %d, want 0", len(filled))
		}
	})
}

func TestQueryToTimeRange(t *testing.T) {
	now := time.Date(2026, 3, 7, 15, 4, 5, 0, time.UTC)
	start := time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 14, 18, 28, 32, 898000000, time.UTC)

	tests := []struct {
		name      string
		query     DashboardStatsQuery
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{
			name: "uses explicit date range when provided",
			query: DashboardStatsQuery{
				Period:    "365d",
				StartDate: &start,
				EndDate:   &end,
			},
			wantStart: start,
			wantEnd:   end,
		},
		{
			name: "falls back to period when range absent",
			query: DashboardStatsQuery{
				Period: "7d",
			},
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name:      "defaults to today when query empty",
			query:     DashboardStatsQuery{},
			wantStart: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
			wantEnd:   now,
		},
		{
			name: "rejects missing end date",
			query: DashboardStatsQuery{
				StartDate: &start,
			},
			wantErr: true,
		},
		{
			name: "rejects non increasing range",
			query: DashboardStatsQuery{
				StartDate: &end,
				EndDate:   &start,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd, err := queryToTimeRange(now, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !gotStart.Equal(tt.wantStart) {
				t.Fatalf("start = %v, want %v", gotStart, tt.wantStart)
			}

			if !gotEnd.Equal(tt.wantEnd) {
				t.Fatalf("end = %v, want %v", gotEnd, tt.wantEnd)
			}
		})
	}
}
