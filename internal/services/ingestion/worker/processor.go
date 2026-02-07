package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ProcessorConfig holds configuration for the worker processor.
type ProcessorConfig struct {
	WorkerCount  int
	BatchSize    int
	MaxRetries   int
	PollInterval time.Duration
}

// Processor processes outbox entries and submits events to EventHandler.
type Processor struct {
	outbox     OutboxReader
	eventStore EventStoreWriter
	submitter  EventSubmitter
	listenConn *pgx.Conn
	config     ProcessorConfig
	logger     *slog.Logger
}

// NewProcessor creates a new worker processor.
func NewProcessor(
	outbox OutboxReader,
	eventStore EventStoreWriter,
	submitter EventSubmitter,
	listenConn *pgx.Conn,
	config ProcessorConfig,
	logger *slog.Logger,
) *Processor {
	return &Processor{
		outbox:     outbox,
		eventStore: eventStore,
		submitter:  submitter,
		listenConn: listenConn,
		config:     config,
		logger:     logger.With("component", "ingestion-worker"),
	}
}

// Start begins processing outbox entries.
// It blocks until the context is cancelled.
func (p *Processor) Start(ctx context.Context) error {
	p.logger.Info("starting ingestion worker",
		"workers", p.config.WorkerCount,
		"batch_size", p.config.BatchSize,
		"poll_interval", p.config.PollInterval,
	)

	// Set up LISTEN for notifications
	_, err := p.listenConn.Exec(ctx, "LISTEN outbox_insert")
	if err != nil {
		return err
	}

	// Create work channel
	workCh := make(chan OutboxEntry, p.config.BatchSize)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.config.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			p.worker(ctx, workerID, workCh)
		}(i)
	}

	// Start dispatcher
	go p.dispatcher(ctx, workCh)

	// Wait for context cancellation
	<-ctx.Done()

	// Close work channel and wait for workers
	close(workCh)
	wg.Wait()

	p.logger.Info("ingestion worker stopped")
	return nil
}

// dispatcher fetches outbox entries and sends them to workers.
func (p *Processor) dispatcher(ctx context.Context, workCh chan<- OutboxEntry) {
	// Create a channel for notifications
	notifyCh := make(chan *pgconn.Notification, 1)

	// Start a single goroutine to listen for notifications
	go p.notificationListener(ctx, notifyCh)

	timer := time.NewTimer(p.config.PollInterval)
	defer timer.Stop()

	// Initial fetch
	p.fetchAndDispatch(ctx, workCh)

	for {
		select {
		case <-ctx.Done():
			return

		case notification := <-notifyCh:
			if notification != nil {
				p.logger.Debug("received NOTIFY", "payload", notification.Payload)
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(p.config.PollInterval)
				p.fetchAndDispatch(ctx, workCh)
			}

		case <-timer.C:
			p.logger.Debug("watchdog timer fired, polling outbox")
			p.fetchAndDispatch(ctx, workCh)
			timer.Reset(p.config.PollInterval)
		}
	}
}

// notificationListener continuously listens for PostgreSQL notifications.
func (p *Processor) notificationListener(ctx context.Context, notifyCh chan<- *pgconn.Notification) {
	for {
		notification, err := p.listenConn.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			p.logger.Error("error waiting for notification", "error", err)
			// Brief pause before retrying to avoid tight loop
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		select {
		case notifyCh <- notification:
		case <-ctx.Done():
			return
		}
	}
}

// fetchAndDispatch fetches pending entries and sends them to workers.
func (p *Processor) fetchAndDispatch(ctx context.Context, workCh chan<- OutboxEntry) {
	entries, err := p.outbox.FetchPending(ctx, p.config.BatchSize)
	if err != nil {
		p.logger.Error("failed to fetch pending entries", "error", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	p.logger.Debug("fetched entries from outbox", "count", len(entries))

	for _, entry := range entries {
		select {
		case workCh <- entry:
		case <-ctx.Done():
			return
		}
	}
}

// worker processes entries from the work channel.
func (p *Processor) worker(ctx context.Context, id int, workCh <-chan OutboxEntry) {
	logger := p.logger.With("worker_id", id)

	for entry := range workCh {
		if ctx.Err() != nil {
			return
		}
		p.processEntry(ctx, logger, entry)
	}
}

// processEntry processes a single outbox entry.
func (p *Processor) processEntry(ctx context.Context, logger *slog.Logger, entry OutboxEntry) {
	logger = logger.With(
		"outbox_id", entry.OutboxID,
		"event_id", entry.Payload.EventID,
		"event_type", entry.Payload.EventType,
	)

	// Check max retries
	if entry.RetryCount >= p.config.MaxRetries {
		logger.Error("max retries exceeded, leaving in outbox as evidence",
			"retry_count", entry.RetryCount,
		)
		return
	}

	// Step 1: Write to event store
	err := p.eventStore.Insert(ctx, entry.Payload)
	if err != nil {
		// Check if it's a duplicate (unique constraint violation)
		if isDuplicateError(err) {
			logger.Debug("event already in event store, skipping to submit")
		} else {
			logger.Error("failed to write to event store", "error", err)
			p.outbox.IncrementRetry(ctx, entry.OutboxID)
			return
		}
	}

	// Step 2: Submit to EventHandler
	err = p.submitter.SubmitEvent(ctx, entry.Payload)
	if err != nil {
		logger.Error("failed to submit event to EventHandler", "error", err)
		p.outbox.IncrementRetry(ctx, entry.OutboxID)
		return
	}

	// Step 3: Delete from outbox
	err = p.outbox.Delete(ctx, entry.OutboxID)
	if err != nil {
		logger.Error("failed to delete from outbox", "error", err)
		// Entry will be reprocessed, but idempotency handles it
		return
	}

	logger.Info("event processed successfully")
}

// isDuplicateError checks if the error is a unique constraint violation.
func isDuplicateError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 23505 is unique_violation
		return pgErr.Code == "23505"
	}
	return false
}
