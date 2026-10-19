package service

import (
	"cmp"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	// messagesPerTurn: one user and one assistant message make a turn (ruling A2-15).
	messagesPerTurn = 2
	fieldAgentID    = "agentId"
	fieldMessage    = "message"
	fieldAssistant  = "assistantMessageId"
	fieldLocale     = "locale"
	fieldStatus     = "status"
)

// ChatStore is the persistence port of the M2 run (amendment W; the index interface, updated). Every method scopes by
// the tenant in ctx, and by the authenticated user where it names a conversation; writers of one conversation are
// serialized by its row lock. BeginRun is the API side; ClaimRun to FailRun are the worker side, fenced by the lease.
type ChatStore interface {
	EnsureConversation(ctx context.Context, id, agentID uuid.UUID, userID uuid.UUID) (model.Conversation, bool, error)
	LoadHistory(ctx context.Context, conversationID uuid.UUID, turns int) (model.History, error)
	// BeginRun applies the message mutation, inserts the run and enqueues its chat.runs job in one transaction.
	BeginRun(ctx context.Context, in model.BeginRunInput) (model.Run, string, error)
	ClaimRun(ctx context.Context, runID uuid.UUID, owner string, lease time.Duration) (model.Run, model.Lease, model.ClaimResult, error)
	RenewLease(ctx context.Context, lease model.Lease, d time.Duration) error
	RecordStep(ctx context.Context, lease model.Lease, step model.RunStep) error
	FinishRun(ctx context.Context, lease model.Lease, run model.Run, assistant model.Message) error
	FailRun(ctx context.Context, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error)
	RunStatus(ctx context.Context, runID uuid.UUID) (model.Run, error)
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
	finishTimeout time.Duration
}

func NewChatStore(pool *pgxpool.Pool, conversations repository.ConversationRepo, messages repository.MessageRepo,
	runs repository.RunRepo, agents agentrepository.AgentRepo, finishTimeout time.Duration) ChatStore {
	return &ChatStoreImpl{pool: pool, conversations: conversations, messages: messages, runs: runs, agents: agents, finishTimeout: finishTimeout}
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

// BeginRun is the only way to start a run, so no run exists without its job. It expires this conversation's dead
// runs first, so a crashed replica never locks it (R6). The request id of the call is the run's id everywhere (W9).
func (s *ChatStoreImpl) BeginRun(ctx context.Context, in model.BeginRunInput) (model.Run, string, error) {
	if err := validateBeginRun(in); err != nil {
		return model.Run{}, "", err
	}
	user, ok := authmiddleware.UserFromContext(ctx)
	if !ok || user.ID == uuid.Nil {
		return model.Run{}, "", errors.ErrConversationNotFound
	}
	// The outbox row and the run row must agree, so a missing id is settled here rather than in each of them.
	if !requestid.IsValid(requestid.FromContext(ctx)) {
		ctx = requestid.NewContext(ctx, requestid.NewJob())
	}
	run := model.Run{ID: uuid.New(), ConversationID: in.ConversationID, AssistantMessageID: in.AssistantMessageID,
		Status: model.RunStatusRunning, DeadlineAt: in.Deadline, RequestID: requestid.FromContext(ctx),
		RequestedModelID: in.RequestedModelID, Locale: cmp.Or(in.Locale, model.LocaleVI)}
	var outboxID string
	err := s.locked(ctx, in.ConversationID, func(tx pgx.Tx, conversation model.Conversation) error {
		if err := checkConversationAgent(conversation, in.AgentID); err != nil {
			return err
		}
		if _, err := s.runs.ExpireStale(ctx, tx, in.ConversationID, s.finishTimeout); err != nil {
			return err
		}
		if err := s.mutateMessages(ctx, tx, in); err != nil {
			return err
		}
		if err := s.runs.InsertRun(ctx, tx, run); err != nil {
			return err
		}
		id, err := jobs.EnqueueOutbox(ctx, tx, model.StreamChatRuns, model.ChatRunJob{RunID: run.ID, ConversationID: in.ConversationID, UserID: user.ID})
		outboxID = id
		return err
	})
	if err != nil {
		return model.Run{}, "", err
	}
	return run, outboxID, nil
}

func validateBeginRun(in model.BeginRunInput) error {
	if in.UserMessage == nil && in.RegenerateMessageID == "" {
		return errors.FieldError(fieldMessage, errors.FieldCodeRequired)
	}
	if in.AssistantMessageID == "" {
		return errors.FieldError(fieldAssistant, errors.FieldCodeRequired)
	}
	if in.Locale != "" && in.Locale != model.LocaleVI && in.Locale != model.LocaleEN {
		return errors.FieldError(fieldLocale, errors.FieldCodeInvalid)
	}
	return nil
}

// mutateMessages applies the message half of BeginRun. Regenerate takes the last assistant message (dropped, R6) or
// the last user message when no answer was saved (kept, W7); any other id is NOT_FOUND.
func (s *ChatStoreImpl) mutateMessages(ctx context.Context, tx pgx.Tx, in model.BeginRunInput) error {
	if in.UserMessage != nil {
		msg := *in.UserMessage
		msg.Role = model.RoleUser
		if err := s.messages.InsertMessage(ctx, tx, in.ConversationID, msg); err != nil {
			return err
		}
		return s.conversations.Touch(ctx, tx, in.ConversationID)
	}
	last, ok, err := s.messages.Last(ctx, tx, in.ConversationID)
	if err != nil {
		return err
	}
	if !ok || last.ID != in.RegenerateMessageID {
		return errors.ErrNotFound
	}
	if last.Role == model.RoleAssistant {
		return s.messages.DeleteMessage(ctx, tx, in.ConversationID, last.ID)
	}
	return nil
}

// ClaimRun makes one executor of a run. A refused claim is explained by one read (amendment W5): the caller acks and
// drops it, or marks the run interrupted or past its deadline.
func (s *ChatStoreImpl) ClaimRun(ctx context.Context, runID uuid.UUID, owner string, lease time.Duration) (model.Run, model.Lease, model.ClaimResult, error) {
	attempt := model.Lease{RunID: runID, Token: uuid.New(), Owner: owner}
	run, claimed, err := s.runs.ClaimRun(ctx, s.pool, runID, attempt, lease)
	if err != nil {
		return model.Run{}, model.Lease{}, "", errors.Normalize(err)
	}
	if claimed {
		return run, attempt, model.ClaimResultClaimed, nil
	}
	state, err := s.runs.ClaimState(ctx, s.pool, runID)
	if err != nil {
		return model.Run{}, model.Lease{}, "", errors.Normalize(err)
	}
	// The refused run carries what the caller needs to finish the story (its id and deadline for the stream expiry).
	return model.Run{ID: runID, Status: state.Status, DeadlineAt: state.DeadlineAt}, model.Lease{}, classifyRefusedClaim(state), nil
}

func classifyRefusedClaim(state repository.ClaimState) model.ClaimResult {
	switch {
	case !state.Found || state.Status != model.RunStatusRunning:
		return model.ClaimResultNotRunning
	case !state.LeaseHeld && state.PastDeadline:
		return model.ClaimResultQueuedPastDeadline
	case state.LeaseHeld && state.LeaseLapsed:
		return model.ClaimResultExpired
	default:
		return model.ClaimResultHeldElsewhere
	}
}

func (s *ChatStoreImpl) RenewLease(ctx context.Context, lease model.Lease, d time.Duration) error {
	renewed, err := s.runs.RenewLease(ctx, s.pool, lease, d)
	if err != nil {
		return errors.Normalize(err)
	}
	if !renewed {
		return errors.ErrRunLeaseLost
	}
	return nil
}

// RecordStep writes only while the lease is live; the statement holds the run row, so it never races the expiry.
func (s *ChatStoreImpl) RecordStep(ctx context.Context, lease model.Lease, step model.RunStep) error {
	written, err := s.runs.InsertFencedStep(ctx, s.pool, lease, step)
	if err != nil {
		return errors.Normalize(err)
	}
	if !written {
		return errors.ErrRunLeaseLost
	}
	return nil
}

// FinishRun locks the conversation first (for seq), then the fenced run update is the first run statement: a lost
// lease rolls the whole transaction back and nothing is stored.
func (s *ChatStoreImpl) FinishRun(ctx context.Context, lease model.Lease, run model.Run, assistant model.Message) error {
	if run.Status == model.RunStatusRunning {
		return errors.FieldError(fieldStatus, errors.FieldCodeInvalid)
	}
	return s.locked(ctx, run.ConversationID, func(tx pgx.Tx, _ model.Conversation) error {
		finished, err := s.runs.FinishFencedRun(ctx, tx, lease, run)
		if err != nil {
			return err
		}
		if !finished {
			return errors.ErrRunLeaseLost
		}
		if len(assistant.Parts) > 0 {
			assistant.Role = model.RoleAssistant
			if err := s.messages.InsertMessage(ctx, tx, run.ConversationID, assistant); err != nil {
				return err
			}
		}
		return s.conversations.Touch(ctx, tx, run.ConversationID)
	})
}

// FailRun is the interrupted and queued-past-deadline transition; false means the row was not the caller's to change.
func (s *ChatStoreImpl) FailRun(ctx context.Context, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error) {
	flipped, err := s.runs.FailRun(ctx, s.pool, runID, code, onlyIfExpired)
	return flipped, errors.Normalize(err)
}

// RunStatus answers NOT_FOUND for a run the user cannot see, whether it is missing, another org's or another user's.
func (s *ChatStoreImpl) RunStatus(ctx context.Context, runID uuid.UUID) (model.Run, error) {
	run, err := s.runs.GetRun(ctx, s.pool, runID)
	if err != nil {
		return model.Run{}, notFoundOrFailure(err, errors.CodeRunNotActive)
	}
	if _, err := s.conversations.GetConversation(ctx, s.pool, run.ConversationID); err != nil {
		return model.Run{}, notFoundOrFailure(err, errors.CodeConversationNotFound)
	}
	return run, nil
}

// notFoundOrFailure hides what the user cannot see behind NOT_FOUND, but an outage stays an outage (never a 404).
func notFoundOrFailure(err error, hidden errors.Code) error {
	if errors.CodeOf(errors.Normalize(err)) == hidden {
		return errors.ErrNotFound
	}
	return errors.Normalize(err)
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

// locked runs fn in one transaction that holds the conversation row, so seq, runs and regenerate never race. The
// lock order is always conversation, then run.
func (s *ChatStoreImpl) locked(ctx context.Context, conversationID uuid.UUID, fn func(tx pgx.Tx, conversation model.Conversation) error) error {
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		conversation, err := s.conversations.GetConversationForUpdate(ctx, tx, conversationID)
		if err != nil {
			return err
		}
		return fn(tx, conversation)
	})
}
