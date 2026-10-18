package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

const (
	// messagesPerTurn: one user and one assistant message make a turn (ruling A2-15).
	messagesPerTurn = 2
	fieldAgentID    = "agentId"
)

// ChatStore is the persistence port of the M2 run loop (index interface, verbatim). Every method scopes by the
// tenant and authenticated user in ctx; writers of one conversation are serialized by its row lock.
type ChatStore interface {
	EnsureConversation(ctx context.Context, id, agentID uuid.UUID, userID uuid.UUID) (model.Conversation, bool, error)
	LoadHistory(ctx context.Context, conversationID uuid.UUID, turns int) (model.History, error)
	AppendUserMessage(ctx context.Context, conversationID uuid.UUID, msg model.Message) error
	StartRun(ctx context.Context, run model.Run) error
	DropLastAssistant(ctx context.Context, conversationID uuid.UUID, messageID string) error
	RecordStep(ctx context.Context, step model.RunStep) error
	FinishRun(ctx context.Context, run model.Run, assistant model.Message) error
	SaveSummary(ctx context.Context, conversationID uuid.UUID, summary string, throughSeq int64) error
	ListMessagesRange(ctx context.Context, conversationID uuid.UUID, afterSeq, throughSeq int64, limit int) ([]model.Message, error)
	SetTitle(ctx context.Context, conversationID uuid.UUID, title string) error
	ActiveRun(ctx context.Context, runID uuid.UUID) (model.Run, error)
}

type ChatStoreImpl struct {
	pool          *pgxpool.Pool
	conversations repository.ConversationRepo
	messages      repository.MessageRepo
	runs          repository.RunRepo
	agents        agentrepository.AgentRepo
}

func NewChatStore(pool *pgxpool.Pool, conversations repository.ConversationRepo, messages repository.MessageRepo,
	runs repository.RunRepo, agents agentrepository.AgentRepo) ChatStore {
	return &ChatStoreImpl{pool: pool, conversations: conversations, messages: messages, runs: runs, agents: agents}
}

// EnsureConversation follows ruling A2-14.
func (s *ChatStoreImpl) EnsureConversation(ctx context.Context, id, agentID uuid.UUID, userID uuid.UUID) (model.Conversation, bool, error) {
	if err := checkConversationUser(ctx, userID); err != nil {
		return model.Conversation{}, false, err
	}
	existing, err := s.conversations.GetConversation(ctx, s.pool, id)
	if err == nil {
		if err := checkConversationAgent(existing, agentID); err != nil {
			return model.Conversation{}, false, err
		}
		return existing, false, nil
	}
	if errors.CodeOf(err) != errors.CodeConversationNotFound {
		return model.Conversation{}, false, errors.Normalize(err)
	}
	if agentID == uuid.Nil {
		return model.Conversation{}, false, errors.FieldError(fieldAgentID, errors.FieldCodeRequired)
	}
	if _, err := s.agents.GetAgent(ctx, s.pool, agentID); err != nil {
		return model.Conversation{}, false, errors.Normalize(err)
	}
	created, err := s.conversations.InsertConversation(ctx, s.pool, model.Conversation{ID: id, AgentID: agentID, UserID: userID})
	if postgres.IsUniqueViolation(err) {
		return s.existingAfterConflict(ctx, id, agentID)
	}
	if err != nil {
		return model.Conversation{}, false, errors.Normalize(err)
	}
	return created, true, nil
}

func (s *ChatStoreImpl) existingAfterConflict(ctx context.Context, id, agentID uuid.UUID) (model.Conversation, bool, error) {
	// A concurrent create may win in this org and user; any other owner remains not found.
	existing, err := s.conversations.GetConversation(ctx, s.pool, id)
	if err != nil {
		return model.Conversation{}, false, errors.Normalize(err)
	}
	if err := checkConversationAgent(existing, agentID); err != nil {
		return model.Conversation{}, false, err
	}
	return existing, false, nil
}

func checkConversationUser(ctx context.Context, userID uuid.UUID) error {
	user, ok := authmiddleware.UserFromContext(ctx)
	if !ok || user.ID == uuid.Nil {
		return errors.ErrUnauthenticated
	}
	if userID != user.ID {
		return errors.ErrConversationNotFound
	}
	return nil
}

func checkConversationAgent(existing model.Conversation, agentID uuid.UUID) error {
	if agentID != uuid.Nil && agentID != existing.AgentID {
		return errors.FieldError(fieldAgentID, errors.FieldCodeInvalid)
	}
	return nil
}

func (s *ChatStoreImpl) LoadHistory(ctx context.Context, conversationID uuid.UUID, turns int) (model.History, error) {
	conversation, err := s.conversations.GetConversation(ctx, s.pool, conversationID)
	if err != nil {
		return model.History{}, errors.Normalize(err)
	}
	latest, err := s.messages.ListLatest(ctx, s.pool, conversationID, turns*messagesPerTurn)
	if err != nil {
		return model.History{}, errors.Normalize(err)
	}
	kept := make([]model.Message, 0, len(latest))
	for _, m := range latest {
		if m.Seq > conversation.SummaryThroughSeq {
			kept = append(kept, m)
		}
	}
	peak, err := s.runs.PeakInputTokens(ctx, s.pool, conversationID)
	if err != nil {
		return model.History{}, errors.Normalize(err)
	}
	return model.History{Conversation: conversation, Messages: kept, PeakInputTokens: peak}, nil
}

func (s *ChatStoreImpl) AppendUserMessage(ctx context.Context, conversationID uuid.UUID, msg model.Message) error {
	msg.Role = model.RoleUser
	return s.locked(ctx, conversationID, func(tx pgx.Tx) error {
		if err := s.messages.InsertMessage(ctx, tx, conversationID, msg); err != nil {
			return err
		}
		return s.conversations.Touch(ctx, tx, conversationID)
	})
}

// StartRun expires this conversation's dead runs first, so a crashed replica never locks it (R6).
func (s *ChatStoreImpl) StartRun(ctx context.Context, run model.Run) error {
	return s.locked(ctx, run.ConversationID, func(tx pgx.Tx) error {
		if _, err := s.runs.ExpireStale(ctx, tx, run.ConversationID); err != nil {
			return err
		}
		return s.runs.InsertRun(ctx, tx, run)
	})
}

func (s *ChatStoreImpl) DropLastAssistant(ctx context.Context, conversationID uuid.UUID, messageID string) error {
	return s.locked(ctx, conversationID, func(tx pgx.Tx) error {
		last, ok, err := s.messages.Last(ctx, tx, conversationID)
		if err != nil {
			return err
		}
		if !ok || last.ID != messageID || last.Role != model.RoleAssistant {
			return errors.ErrNotFound
		}
		return s.messages.DeleteMessage(ctx, tx, conversationID, messageID)
	})
}

func (s *ChatStoreImpl) RecordStep(ctx context.Context, step model.RunStep) error {
	run, err := s.runs.GetRun(ctx, s.pool, step.RunID)
	if err != nil {
		return errors.Normalize(err)
	}
	if _, err := s.conversations.GetConversation(ctx, s.pool, run.ConversationID); err != nil {
		return errors.ErrRunNotActive
	}
	return errors.Normalize(s.runs.InsertStep(ctx, s.pool, step))
}

func (s *ChatStoreImpl) FinishRun(ctx context.Context, run model.Run, assistant model.Message) error {
	return s.locked(ctx, run.ConversationID, func(tx pgx.Tx) error {
		if len(assistant.Parts) > 0 {
			assistant.Role = model.RoleAssistant
			if err := s.messages.InsertMessage(ctx, tx, run.ConversationID, assistant); err != nil {
				return err
			}
		}
		if err := s.runs.FinishRun(ctx, tx, run); err != nil {
			return err
		}
		return s.conversations.Touch(ctx, tx, run.ConversationID)
	})
}

func (s *ChatStoreImpl) SaveSummary(ctx context.Context, conversationID uuid.UUID, summary string, throughSeq int64) error {
	return errors.Normalize(s.conversations.SaveSummary(ctx, s.pool, conversationID, summary, throughSeq))
}

// ListMessagesRange pages the messages compaction folds (afterSeq < seq <= throughSeq, oldest first).
func (s *ChatStoreImpl) ListMessagesRange(ctx context.Context, conversationID uuid.UUID, afterSeq, throughSeq int64, limit int) ([]model.Message, error) {
	if _, err := s.conversations.GetConversation(ctx, s.pool, conversationID); err != nil {
		return nil, errors.Normalize(err)
	}
	messages, err := s.messages.ListRange(ctx, s.pool, conversationID, afterSeq, throughSeq, limit)
	return messages, errors.Normalize(err)
}

func (s *ChatStoreImpl) SetTitle(ctx context.Context, conversationID uuid.UUID, title string) error {
	return errors.Normalize(s.conversations.SetTitle(ctx, s.pool, conversationID, title))
}

func (s *ChatStoreImpl) ActiveRun(ctx context.Context, runID uuid.UUID) (model.Run, error) {
	run, err := s.runs.GetRun(ctx, s.pool, runID)
	if err != nil {
		return model.Run{}, errors.Normalize(err)
	}
	if _, err := s.conversations.GetConversation(ctx, s.pool, run.ConversationID); err != nil {
		return model.Run{}, errors.ErrRunNotActive
	}
	if run.Status != model.RunStatusRunning {
		return model.Run{}, errors.ErrRunNotActive
	}
	return run, nil
}

// locked runs fn in one transaction that holds the conversation row, so seq, runs and regenerate never race.
func (s *ChatStoreImpl) locked(ctx context.Context, conversationID uuid.UUID, fn func(tx pgx.Tx) error) error {
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.conversations.GetConversationForUpdate(ctx, tx, conversationID); err != nil {
			return err
		}
		return fn(tx)
	})
}
