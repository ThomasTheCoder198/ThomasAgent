package repository

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const conversationColumns = `id, agent_id, user_id, title, summary, summary_through_seq, created_at, updated_at`

// ConversationRepo stores an org's conversations; every method scopes by tenant and authenticated user in ctx.
type ConversationRepo interface {
	InsertConversation(ctx context.Context, db postgres.DBTX, c model.Conversation) (model.Conversation, error)
	GetConversation(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Conversation, error)
	GetConversationForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Conversation, error)
	// ListPage returns up to limit+1 rows after (afterUpdated, afterID); the extra row tells the caller a next page exists.
	ListPage(ctx context.Context, db postgres.DBTX, afterUpdated *time.Time, afterID uuid.UUID, limit int) ([]model.Conversation, error)
	SetTitle(ctx context.Context, db postgres.DBTX, id uuid.UUID, title string) error
	SaveSummary(ctx context.Context, db postgres.DBTX, id uuid.UUID, summary string, throughSeq int64) error
	Touch(ctx context.Context, db postgres.DBTX, id uuid.UUID) error
	DeleteConversation(ctx context.Context, db postgres.DBTX, id uuid.UUID) error
	PurgeInactiveAcrossTenants(ctx context.Context, db postgres.DBTX, cutoff time.Time, batch int) (int64, error)
}

type conversationRepo struct{}

func NewConversationRepo() ConversationRepo { return conversationRepo{} }

func scanConversation(row pgx.Row) (model.Conversation, error) {
	var c model.Conversation
	err := row.Scan(&c.ID, &c.AgentID, &c.UserID, &c.Title, &c.Summary, &c.SummaryThroughSeq, &c.CreatedAt, &c.UpdatedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Conversation{}, errors.ErrConversationNotFound
	}
	if err != nil {
		return model.Conversation{}, fmt.Errorf("scan conversation: %w", err)
	}
	return c, nil
}

// InsertConversation binds the owner to ctx and proves the agent belongs to its tenant.
func (conversationRepo) InsertConversation(ctx context.Context, db postgres.DBTX, c model.Conversation) (model.Conversation, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	if c.UserID != uuid.Nil && c.UserID != userID {
		return model.Conversation{}, errors.ErrConversationNotFound
	}
	inserted, err := scanConversation(db.QueryRow(ctx, `INSERT INTO conversations (id, tenant_id, agent_id, user_id, title)
		SELECT $1, a.tenant_id, a.id, $4, $5 FROM agents a WHERE a.id = $3 AND a.tenant_id = $2
		RETURNING `+conversationColumns, c.ID, tenantID, c.AgentID, userID, c.Title))
	if stderrors.Is(err, errors.ErrConversationNotFound) {
		return model.Conversation{}, errors.ErrAgentNotFound
	}
	return inserted, err
}

func (conversationRepo) GetConversation(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Conversation, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	return scanConversation(db.QueryRow(ctx, `SELECT `+conversationColumns+` FROM conversations WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id, tenantID, userID))
}

// GetConversationForUpdate serializes writers of one conversation (message seq, runs, regenerate).
func (conversationRepo) GetConversationForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Conversation, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return model.Conversation{}, err
	}
	return scanConversation(db.QueryRow(ctx, `SELECT `+conversationColumns+` FROM conversations WHERE id = $1 AND tenant_id = $2 AND user_id = $3 FOR UPDATE`, id, tenantID, userID))
}

func (conversationRepo) ListPage(ctx context.Context, db postgres.DBTX, afterUpdated *time.Time, afterID uuid.UUID, limit int) ([]model.Conversation, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+conversationColumns+` FROM conversations
		WHERE tenant_id = $1 AND user_id = $2 AND ($3::timestamptz IS NULL OR (updated_at, id) < ($3, $4))
		ORDER BY updated_at DESC, id DESC LIMIT $5`, tenantID, userID, afterUpdated, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	list := []model.Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (conversationRepo) SetTitle(ctx context.Context, db postgres.DBTX, id uuid.UUID, title string) error {
	return execOne(ctx, db, `UPDATE conversations SET title = $4, updated_at = now() WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id, title)
}

func (conversationRepo) SaveSummary(ctx context.Context, db postgres.DBTX, id uuid.UUID, summary string, throughSeq int64) error {
	return execOne(ctx, db, `UPDATE conversations SET summary = $4, summary_through_seq = $5 WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id, summary, throughSeq)
}

// Touch moves the conversation to the top of recents.
func (conversationRepo) Touch(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	return execOne(ctx, db, `UPDATE conversations SET updated_at = now() WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id)
}

func (conversationRepo) DeleteConversation(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	return execOne(ctx, db, `DELETE FROM conversations WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id)
}

// PurgeInactiveAcrossTenants is reserved for the system job; request paths remain tenant and user scoped.
func (conversationRepo) PurgeInactiveAcrossTenants(ctx context.Context, db postgres.DBTX, cutoff time.Time, batch int) (int64, error) {
	tag, err := db.Exec(ctx, `DELETE FROM conversations WHERE id IN (
		SELECT c.id FROM conversations c WHERE c.updated_at < $1 AND NOT EXISTS (
			SELECT 1 FROM runs r WHERE r.conversation_id = c.id AND r.status = 'running')
		ORDER BY c.updated_at LIMIT $2 FOR UPDATE SKIP LOCKED)`, cutoff, batch)
	if err != nil {
		return 0, fmt.Errorf("purge inactive conversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// execOne runs a statement whose first three parameters are id, tenant and user, and maps "no row" to not found.
func execOne(ctx context.Context, db postgres.DBTX, sql string, id uuid.UUID, args ...any) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, sql, append([]any{id, tenantID, userID}, args...)...)
	if err != nil {
		return fmt.Errorf("conversation write: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrConversationNotFound
	}
	return nil
}

func authenticatedUserID(ctx context.Context) (uuid.UUID, error) {
	user, ok := authmiddleware.UserFromContext(ctx)
	if !ok || user.ID == uuid.Nil {
		return uuid.Nil, errors.ErrConversationNotFound
	}
	return user.ID, nil
}
