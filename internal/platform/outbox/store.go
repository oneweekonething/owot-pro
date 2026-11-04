// Package outbox persists events in the business transaction and retries delivery.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

type eventPO struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	Topic         string    `gorm:"size:128;not null"`
	TenantID      int64     `gorm:"not null;default:0"`
	Payload       string    `gorm:"type:longtext;not null"`
	Status        int       `gorm:"not null;default:0;index:idx_outbox_status_retry,priority:1"`
	RetryCount    int       `gorm:"not null;default:0"`
	NextRetryTime time.Time `gorm:"not null;index:idx_outbox_status_retry,priority:2"`
	CreateTime    time.Time `gorm:"autoCreateTime"`
	UpdateTime    time.Time `gorm:"autoUpdateTime"`
}

func (eventPO) TableName() string { return "sys_outbox_event" }

type Event struct {
	ID       int64           `json:"id"`
	Topic    string          `json:"topic"`
	TenantID int64           `json:"tenantId"`
	Payload  json.RawMessage `json:"payload"`
}

type Broadcaster interface {
	Broadcast(context.Context, Event) error
}

type RedisBroadcaster struct{ client redis.UniversalClient }

func NewRedisBroadcaster(client redis.UniversalClient) *RedisBroadcaster {
	return &RedisBroadcaster{client: client}
}

func (b *RedisBroadcaster) Broadcast(ctx context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = b.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, fmt.Sprintf("%s:version:%d", event.Topic, event.TenantID))
		pipe.Publish(ctx, event.Topic, data)
		return nil
	})
	return err
}

type Store struct {
	db        *gorm.DB
	broadcast Broadcaster
	logger    *slog.Logger
}

func NewStore(db *gorm.DB, broadcast Broadcaster, logger *slog.Logger) *Store {
	return &Store{db: db, broadcast: broadcast, logger: logger}
}

func Migrate(db *gorm.DB) error { return db.AutoMigrate(&eventPO{}) }

// Publish requires a transaction so callers cannot accidentally split business data and events.
func (s *Store) Publish(ctx context.Context, topic string, payload []byte) error {
	if !dbx.InTransaction(ctx) {
		return errors.New("outbox: publish requires a transaction")
	}
	if topic == "" || len(topic) > 128 || !json.Valid(payload) {
		return errors.New("outbox: invalid event")
	}
	row := eventPO{Topic: topic, TenantID: ctxmeta.TenantID(ctx), Payload: string(payload), NextRetryTime: time.Now()}
	if err := dbx.FromContext(ctx, s.db).Create(&row).Error; err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}
	dbx.AfterCommitOnce(ctx, s, func(ctx context.Context) {
		attemptCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := s.Flush(attemptCtx, 100); err != nil {
			s.logger.Warn("outbox immediate delivery failed; relay will retry", "error", err)
		}
	})
	return nil
}

// Run belongs to the composition root's lifecycle. Delivery is at least once;
// consumers must deduplicate side effects by Event.ID.
func (s *Store) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			if err := s.Flush(attemptCtx, 100); err != nil && ctx.Err() == nil {
				s.logger.Warn("outbox relay delivery failed", "error", err)
			}
			cancel()
		}
	}
}

func (s *Store) Flush(ctx context.Context, limit int) error {
	var failures []error
	for i := 0; i < limit; i++ {
		found, err := s.deliver(ctx)
		if err != nil {
			failures = append(failures, err)
		}
		if !found || ctx.Err() != nil {
			break
		}
	}
	return errors.Join(failures...)
}

func (s *Store) deliver(ctx context.Context) (bool, error) {
	var found bool
	var deliveryErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("status = 0 AND next_retry_time <= ?", time.Now())
		lock := clause.Locking{Strength: "UPDATE"}
		if s.db.Name() == "mysql" {
			lock.Options = "SKIP LOCKED"
		}
		var row eventPO
		if err := query.Clauses(lock).Order("id ASC").First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		found = true
		deliveryErr = s.broadcast.Broadcast(ctx, Event{ID: row.ID, Topic: row.Topic, TenantID: row.TenantID, Payload: json.RawMessage(row.Payload)})
		updates := map[string]any{"status": 1}
		if deliveryErr != nil {
			delay := time.Second * time.Duration(1<<min(row.RetryCount+1, 5))
			updates = map[string]any{"retry_count": row.RetryCount + 1, "next_retry_time": time.Now().Add(delay)}
		}
		return tx.Model(&eventPO{}).Where("id = ?", row.ID).Updates(updates).Error
	})
	return found, errors.Join(err, deliveryErr)
}
