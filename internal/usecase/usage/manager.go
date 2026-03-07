package usage

import (
	"context"
	"sync"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ManagerConfig holds configuration for the usage manager.
type ManagerConfig struct {
	QueueSize     int           // Size of the internal queue
	BatchSize     int           // Number of records to process in a batch
	FlushInterval time.Duration // How often to flush batches
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		QueueSize:     10000,
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
	}
}

// Manager implements UsageManager with an internal queue.
type Manager struct {
	repo     AuditRepository
	pricing  domainusage.PricingConfig
	config   ManagerConfig
	logger   *zap.Logger
	queue    chan domainusage.UsageRecord
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewManager creates a new usage manager.
func NewManager(
	repo AuditRepository,
	pricing domainusage.PricingConfig,
	config ManagerConfig,
	logger *zap.Logger,
) *Manager {
	return &Manager{
		repo:    repo,
		pricing: pricing,
		config:  config,
		logger:  logger,
		queue:   make(chan domainusage.UsageRecord, config.QueueSize),
		stopCh:  make(chan struct{}),
	}
}

// Publish queues a usage record for async processing.
// Returns immediately (at-most-once delivery - drops if queue is full).
func (m *Manager) Publish(record domainusage.UsageRecord) {
	select {
	case m.queue <- record:
		m.logger.Debug("usage record queued",
			zap.String("request_id", record.RequestID),
			zap.String("model", record.Model),
		)
	default:
		// Queue is full, drop the record (at-most-once semantics)
		m.logger.Warn("usage queue full, dropping record",
			zap.String("request_id", record.RequestID),
			zap.String("model", record.Model),
		)
	}
}

// Start starts the background processing goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.processLoop(ctx)
	m.logger.Info("usage manager started",
		zap.Int("queue_size", m.config.QueueSize),
		zap.Int("batch_size", m.config.BatchSize),
		zap.Duration("flush_interval", m.config.FlushInterval),
	)
}

// Stop gracefully stops the manager.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
		m.wg.Wait()
		m.logger.Info("usage manager stopped")
	})
}

func (m *Manager) processLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]domainusage.UsageRecord, 0, m.config.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		m.processBatch(ctx, batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, drain queue, flush remaining and exit
			m.drainQueue(&batch)
			flush()
			return

		case <-m.stopCh:
			// Stop requested, drain queue, flush remaining and exit
			m.drainQueue(&batch)
			flush()
			return

		case record := <-m.queue:
			batch = append(batch, record)
			if len(batch) >= m.config.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func (m *Manager) drainQueue(batch *[]domainusage.UsageRecord) {
	for {
		select {
		case record := <-m.queue:
			*batch = append(*batch, record)
			if len(*batch) >= m.config.BatchSize {
				m.processBatch(context.Background(), *batch)
				*batch = (*batch)[:0]
			}
		default:
			return
		}
	}
}

func (m *Manager) processBatch(ctx context.Context, batch []domainusage.UsageRecord) {
	for _, record := range batch {
		m.processRecord(ctx, record)
	}
}

func (m *Manager) processRecord(ctx context.Context, record domainusage.UsageRecord) {
	// Calculate pricing
	inputMicros, outputMicros, totalMicros := m.pricing.CalculatePrice(
		record.Model,
		record.PromptTokens,
		record.CompletionTokens,
	)

	// Calculate duration
	durationMs := 0
	if !record.CompletedAt.IsZero() && !record.StartedAt.IsZero() {
		durationMs = int(record.CompletedAt.Sub(record.StartedAt).Milliseconds())
	}

	// Create audit log
	log := domainusage.AuditLog{
		ID:                  uuid.NewString(),
		RequestID:           record.RequestID,
		Model:               record.Model,
		Provider:            record.Provider,
		CredentialID:        record.CredentialID,
		CredentialType:      record.CredentialType,
		CredentialAccountID: record.CredentialAccountID,
		PromptTokens:        record.PromptTokens,
		CompletionTokens:    record.CompletionTokens,
		TotalTokens:         record.PromptTokens + record.CompletionTokens,
		InputPriceMicros:    inputMicros,
		OutputPriceMicros:   outputMicros,
		TotalPriceMicros:    totalMicros,
		Status:              record.Status,
		ErrorMessage:        record.ErrorMessage,
		StartedAt:           record.StartedAt,
		CompletedAt:         record.CompletedAt,
		DurationMs:          durationMs,
		Stream:              record.Stream,
	}

	// Store in database
	if err := m.repo.Create(ctx, log); err != nil {
		m.logger.Error("failed to store usage audit log",
			zap.String("request_id", record.RequestID),
			zap.Error(err),
		)
		return
	}

	m.logger.Debug("usage audit log stored",
		zap.String("request_id", record.RequestID),
		zap.String("model", record.Model),
		zap.Int("total_tokens", log.TotalTokens),
		zap.Int64("total_price_micros", totalMicros),
	)
}
