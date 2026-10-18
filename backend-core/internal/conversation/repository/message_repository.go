package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	messageColumns = `id, role, seq, parts, metadata, created_at`
	fieldMessageID = "id"
)

var emptyMetadata = json.RawMessage(`{}`)

// MessageRepo stores UIMessages per conversation; seq orders them and is assigned here.
type MessageRepo interface {
	InsertMessage(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, m model.Message) error
	ListLatest(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, limit int) ([]model.Message, error)
	ListAll(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) ([]model.Message, error)
	ListRange(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, afterSeq, throughSeq int64, limit int) ([]model.Message, error)
	Last(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (model.Message, bool, error)
	DeleteMessage(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, id string) error
}

type messageRepo struct{}

func NewMessageRepo() MessageRepo { return messageRepo{} }

// InsertMessage takes the next seq inside the statement. Writers of one conversation are serialized by the caller
// (GetConversationForUpdate), so two inserts never race for a seq. A repeated client id is a conflict on "id".
// The row is selected from the tenant's own conversation, so another org's conversation is CONVERSATION_NOT_FOUND
// and its seq is never read or advanced.
func (messageRepo) InsertMessage(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, m model.Message) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	metadata := m.Metadata
	if len(metadata) == 0 {
		metadata = emptyMetadata
	}
	tag, err := db.Exec(ctx, `INSERT INTO messages (conversation_id, id, tenant_id, seq, role, parts, metadata)
		SELECT c.id, $2, c.tenant_id, COALESCE(MAX(m.seq), 0) + 1, $4, $5, $6
		FROM conversations c LEFT JOIN messages m ON m.conversation_id = c.id AND m.tenant_id = c.tenant_id
		WHERE c.id = $1 AND c.tenant_id = $3 GROUP BY c.id, c.tenant_id`,
		conversationID, m.ID, tenantID, m.Role, m.Parts, metadata)
	if postgres.IsUniqueViolation(err) {
		return errors.FieldConflict(fieldMessageID)
	}
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrConversationNotFound
	}
	return nil
}

// ListLatest returns the last limit messages, oldest first.
func (messageRepo) ListLatest(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, limit int) ([]model.Message, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	return queryMessages(ctx, db, `SELECT `+messageColumns+` FROM (
		SELECT `+messageColumns+` FROM messages WHERE conversation_id = $1 AND tenant_id = $2 ORDER BY seq DESC LIMIT $3
	) latest ORDER BY seq`, conversationID, tenantID, limit)
}

func (messageRepo) ListAll(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) ([]model.Message, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	return queryMessages(ctx, db, `SELECT `+messageColumns+` FROM messages WHERE conversation_id = $1 AND tenant_id = $2 ORDER BY seq`,
		conversationID, tenantID)
}

// ListRange returns at most limit messages with afterSeq < seq <= throughSeq, oldest first. Compaction (M2b) pages
// through it so turns older than the loaded history window are summarized too (index amendment 16).
func (messageRepo) ListRange(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, afterSeq, throughSeq int64, limit int) ([]model.Message, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	return queryMessages(ctx, db, `SELECT `+messageColumns+` FROM messages
		WHERE conversation_id = $1 AND tenant_id = $2 AND seq > $3 AND seq <= $4 ORDER BY seq LIMIT $5`,
		conversationID, tenantID, afterSeq, throughSeq, limit)
}

func (repo messageRepo) Last(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (model.Message, bool, error) {
	latest, err := repo.ListLatest(ctx, db, conversationID, 1)
	if err != nil || len(latest) == 0 {
		return model.Message{}, false, err
	}
	return latest[0], true, nil
}

func (messageRepo) DeleteMessage(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, id string) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `DELETE FROM messages WHERE conversation_id = $1 AND id = $2 AND tenant_id = $3`, conversationID, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrNotFound
	}
	return nil
}

func queryMessages(ctx context.Context, db postgres.DBTX, sql string, args ...any) ([]model.Message, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	list := []model.Message{}
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Seq, &m.Parts, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}
