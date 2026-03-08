package usage

import (
	"context"
	"sync"
	"testing"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"go.uber.org/zap"
)

// mockAuditRepository implements AuditRepository for testing.
type mockAuditRepository struct {
	mu        sync.Mutex
	logs      []domainusage.AuditLog
	createErr error
}

func newMockAuditRepository() *mockAuditRepository {
	return &mockAuditRepository{
		logs: make([]domainusage.AuditLog, 0),
	}
}

func (m *mockAuditRepository) Create(_ context.Context, log domainusage.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditRepository) List(_ context.Context, _ AuditLogFilter) ([]domainusage.AuditLog, error) {
	return nil, nil
}

func (m *mockAuditRepository) Count(_ context.Context, _ AuditLogFilter) (int64, error) {
	return 0, nil
}

func (m *mockAuditRepository) GetByRequestID(_ context.Context, _ string) (*domainusage.AuditLog, error) {
	return nil, nil
}

func (m *mockAuditRepository) DeleteBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockAuditRepository) AggregateByModel(_ context.Context, _, _ time.Time) ([]domainusage.ModelStats, error) {
	return nil, nil
}

func (m *mockAuditRepository) AggregateByCredential(_ context.Context, _, _ time.Time) ([]domainusage.CredentialStats, error) {
	return nil, nil
}

func (m *mockAuditRepository) AggregateHourly(_ context.Context, _, _ time.Time) ([]domainusage.HourlyStats, error) {
	return nil, nil
}

func (m *mockAuditRepository) AggregateDaily(_ context.Context, _, _ time.Time) ([]domainusage.DailyStats, error) {
	return nil, nil
}

func (m *mockAuditRepository) GetOverview(_ context.Context, _, _ time.Time) (*domainusage.Overview, error) {
	return nil, nil
}

func (m *mockAuditRepository) getLogs() []domainusage.AuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]domainusage.AuditLog, len(m.logs))
	copy(copied, m.logs)
	return copied
}

func TestManager_Publish(t *testing.T) {
	repo := newMockAuditRepository()
	pricing := domainusage.DefaultPricingConfig()
	config := ManagerConfig{
		QueueSize:     10,
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
	}
	logger := zap.NewNop()

	manager := NewManager(repo, pricing, config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)

	// Publish a record
	record := domainusage.UsageRecord{
		RequestID:        "req-123",
		Model:            "claude-sonnet-4-20250514",
		Provider:         "copilot",
		PromptTokens:     100,
		CompletionTokens: 50,
		Status:           domainusage.StatusDone,
		StartedAt:        time.Now().Add(-1 * time.Second),
		CompletedAt:      time.Now(),
	}
	manager.Publish(record)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Stop manager
	cancel()
	manager.Stop()

	// Verify the record was stored
	logs := repo.getLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	if logs[0].RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", logs[0].RequestID)
	}
	if logs[0].Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", logs[0].Model)
	}
	if logs[0].TotalTokens != 150 {
		t.Errorf("expected total_tokens 150, got %d", logs[0].TotalTokens)
	}
	if logs[0].Status != domainusage.StatusDone {
		t.Errorf("expected status done, got %s", logs[0].Status)
	}
}

func TestManager_BatchProcessing(t *testing.T) {
	repo := newMockAuditRepository()
	pricing := domainusage.DefaultPricingConfig()
	config := ManagerConfig{
		QueueSize:     100,
		BatchSize:     5,
		FlushInterval: 500 * time.Millisecond, // Long interval to test batch trigger
	}
	logger := zap.NewNop()

	manager := NewManager(repo, pricing, config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)

	// Publish 5 records to trigger batch processing
	for i := 0; i < 5; i++ {
		manager.Publish(domainusage.UsageRecord{
			RequestID:        "req-" + string(rune('a'+i)),
			Model:            "claude-opus-4-20250514",
			PromptTokens:     100,
			CompletionTokens: 50,
			Status:           domainusage.StatusDone,
		})
	}

	// Wait a bit for batch to be processed (should trigger because batch is full)
	time.Sleep(50 * time.Millisecond)

	cancel()
	manager.Stop()

	logs := repo.getLogs()
	if len(logs) != 5 {
		t.Fatalf("expected 5 logs, got %d", len(logs))
	}
}

func TestManager_DrainOnStop(t *testing.T) {
	repo := newMockAuditRepository()
	pricing := domainusage.DefaultPricingConfig()
	config := ManagerConfig{
		QueueSize:     100,
		BatchSize:     100, // Large batch size to ensure flush only on stop
		FlushInterval: 10 * time.Second,
	}
	logger := zap.NewNop()

	manager := NewManager(repo, pricing, config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)

	// Publish 3 records
	for i := 0; i < 3; i++ {
		manager.Publish(domainusage.UsageRecord{
			RequestID: "drain-" + string(rune('a'+i)),
			Model:     "claude-opus-4-20250514",
			Status:    domainusage.StatusDone,
		})
	}

	// Give queue time to accept records
	time.Sleep(10 * time.Millisecond)

	// Stop should drain the queue
	cancel()
	manager.Stop()

	logs := repo.getLogs()
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs after drain, got %d", len(logs))
	}
}

func TestManager_QueueFull(t *testing.T) {
	repo := newMockAuditRepository()
	pricing := domainusage.DefaultPricingConfig()
	config := ManagerConfig{
		QueueSize:     2, // Small queue
		BatchSize:     100,
		FlushInterval: 10 * time.Second,
	}
	logger := zap.NewNop()

	manager := NewManager(repo, pricing, config, logger)

	// Don't start the manager, so queue won't be drained
	// Publish 5 records - only 2 should be accepted
	for i := 0; i < 5; i++ {
		manager.Publish(domainusage.UsageRecord{
			RequestID: "full-" + string(rune('a'+i)),
			Model:     "claude-opus-4-20250514",
			Status:    domainusage.StatusDone,
		})
	}

	// Queue should have 2 items
	if len(manager.queue) != 2 {
		t.Errorf("expected queue length 2, got %d", len(manager.queue))
	}
}

func TestManager_PricingCalculation(t *testing.T) {
	repo := newMockAuditRepository()
	pricing := domainusage.DefaultPricingConfig()
	config := ManagerConfig{
		QueueSize:     10,
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	}
	logger := zap.NewNop()

	manager := NewManager(repo, pricing, config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)

	// Publish a record with known token counts
	// Opus 4.5: input=$15/MTok, cached=$2/MTok, output=$75/MTok
	// Model must match exactly a key in pricing config
	// 1000 input tokens = $0.015 = 15000 microdollars
	// 200 cached tokens = $0.0004 = 400 microdollars
	// 500 output tokens = $0.0375 = 37500 microdollars
	record := domainusage.UsageRecord{
		RequestID:        "price-test",
		Model:            "claude-opus-4-5", // Must match pricing config key
		PromptTokens:     1000,
		CachedTokens:     200,
		CompletionTokens: 500,
		Status:           domainusage.StatusDone,
	}
	manager.Publish(record)

	time.Sleep(100 * time.Millisecond)
	cancel()
	manager.Stop()

	logs := repo.getLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	// Verify pricing (Opus 4.5 pricing)
	log := logs[0]
	// Input: 1000 * 15 = 15000 microdollars
	// Cached: 200 * 2 = 400 microdollars
	expectedInputMicros := int64(15000 + 400)
	// Output: 500 * 75 = 37500 microdollars
	expectedOutputMicros := int64(37500)
	expectedTotalMicros := expectedInputMicros + expectedOutputMicros

	if log.InputPriceMicros != expectedInputMicros {
		t.Errorf("expected input price %d microdollars, got %d", expectedInputMicros, log.InputPriceMicros)
	}
	if log.OutputPriceMicros != expectedOutputMicros {
		t.Errorf("expected output price %d microdollars, got %d", expectedOutputMicros, log.OutputPriceMicros)
	}
	if log.TotalPriceMicros != expectedTotalMicros {
		t.Errorf("expected total price %d microdollars, got %d", expectedTotalMicros, log.TotalPriceMicros)
	}
	if log.CachedTokens != 200 {
		t.Errorf("expected cached tokens 200, got %d", log.CachedTokens)
	}
	if log.TotalTokens != 1700 {
		t.Errorf("expected total tokens 1700, got %d", log.TotalTokens)
	}
}
