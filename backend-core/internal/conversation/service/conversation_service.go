package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	agentservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

type ConversationService interface {
	List(ctx context.Context, page model.Page) ([]model.ConversationSummary, model.PageMeta, error)
	Get(ctx context.Context, id uuid.UUID) (model.ConversationDetail, error)
	Rename(ctx context.Context, id uuid.UUID, title string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

const (
	logKBLookupFailed = "conversation kb lookup failed"
	fieldKBID         = "kb_id"
)

type ConversationServiceImpl struct {
	pool          *pgxpool.Pool
	conversations repository.ConversationRepo
	messages      repository.MessageRepo
	agents        agentrepository.AgentRepo
	kbs           agentservice.KnowledgeBaseLookup
	cfg           config.ConversationConfig
	log           *slog.Logger
}

func NewConversationService(pool *pgxpool.Pool, conversations repository.ConversationRepo, messages repository.MessageRepo,
	agents agentrepository.AgentRepo, kbs agentservice.KnowledgeBaseLookup, cfg config.ConversationConfig, log *slog.Logger) ConversationService {
	return &ConversationServiceImpl{pool: pool, conversations: conversations, messages: messages, agents: agents, kbs: kbs, cfg: cfg, log: log}
}

// ParseLimit turns the query value into a limit (ruling A2-11): empty is the default, anything else 1..max.
func ParseLimit(raw string, cfg config.ConversationConfig) (int, error) {
	if raw == "" {
		return cfg.PageDefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > cfg.PageMaxLimit {
		return 0, errors.FieldError(model.FieldLimit, errors.FieldCodeInvalid)
	}
	return limit, nil
}

func (s *ConversationServiceImpl) List(ctx context.Context, page model.Page) ([]model.ConversationSummary, model.PageMeta, error) {
	if page.Limit < 1 || page.Limit > s.cfg.PageMaxLimit {
		return nil, model.PageMeta{}, errors.FieldError(model.FieldLimit, errors.FieldCodeInvalid)
	}
	var after *time.Time
	var afterID uuid.UUID
	if page.Cursor != "" {
		at, id, err := decodeCursor(page.Cursor)
		if err != nil {
			return nil, model.PageMeta{}, err
		}
		after, afterID = &at, id
	}
	rows, err := s.conversations.ListPage(ctx, s.pool, after, afterID, page.Limit)
	if err != nil {
		return nil, model.PageMeta{}, errors.Normalize(err)
	}
	meta := model.PageMeta{Limit: page.Limit}
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		last := rows[len(rows)-1]
		next := encodeCursor(last.UpdatedAt, last.ID)
		meta.Cursor = &next
	}
	summaries := make([]model.ConversationSummary, 0, len(rows))
	for _, c := range rows {
		summaries = append(summaries, model.ConversationSummary{ID: c.ID, Title: c.Title, AgentID: c.AgentID,
			Lines: []string{model.LineKB}, UpdatedAt: c.UpdatedAt})
	}
	return summaries, meta, nil
}

func (s *ConversationServiceImpl) Get(ctx context.Context, id uuid.UUID) (model.ConversationDetail, error) {
	conversation, err := s.conversations.GetConversation(ctx, s.pool, id)
	if err != nil {
		return model.ConversationDetail{}, errors.Normalize(err)
	}
	messages, err := s.messages.ListAll(ctx, s.pool, id)
	if err != nil {
		return model.ConversationDetail{}, errors.Normalize(err)
	}
	agent, err := s.agents.GetAgent(ctx, s.pool, conversation.AgentID)
	if err != nil {
		return model.ConversationDetail{}, errors.Normalize(err)
	}
	kb := s.lookupKB(ctx, agent.KBID)
	scope := model.Scope{Agent: model.ScopeAgent{ID: agent.ID, Name: agent.Name}, KB: kb, ToolSources: []model.ToolSource{}}
	if !kb.Missing {
		scope.ToolSources = append(scope.ToolSources, model.ToolSource{Line: model.LineKB, Name: kb.Name})
	}
	return model.ConversationDetail{ID: conversation.ID, Title: conversation.Title, AgentID: conversation.AgentID, Scope: scope, Messages: messages}, nil
}

// lookupKB never fails a read. Index amendment 18: only KB_NOT_FOUND marks the KB missing; an outage leaves the name
// empty and the KB present, so the run keeps its KB tools and reports the outage per tool call.
func (s *ConversationServiceImpl) lookupKB(ctx context.Context, kbID uuid.UUID) model.ScopeKB {
	found, err := s.kbs.GetKnowledgeBase(ctx, kbID)
	if err == nil {
		return model.ScopeKB{ID: kbID, Name: found.Name}
	}
	if errors.CodeOf(err) == errors.CodeKbNotFound {
		return model.ScopeKB{ID: kbID, Missing: true}
	}
	s.log.WarnContext(ctx, logKBLookupFailed, slog.String(fieldKBID, kbID.String()), slog.String(logging.FieldErrorCode, string(errors.CodeOf(err))))
	return model.ScopeKB{ID: kbID}
}

func (s *ConversationServiceImpl) Rename(ctx context.Context, id uuid.UUID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.FieldError(model.FieldTitle, errors.FieldCodeRequired)
	}
	if utf8.RuneCountInString(title) > model.TitleMaxRunes {
		return errors.FieldError(model.FieldTitle, errors.FieldCodeInvalid)
	}
	return errors.Normalize(s.conversations.SetTitle(ctx, s.pool, id, title))
}

func (s *ConversationServiceImpl) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.Normalize(s.conversations.DeleteConversation(ctx, s.pool, id))
}
