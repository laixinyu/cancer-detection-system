package service

import (
	"context"
	"encoding/json"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"
)

type AuditService struct {
	repo repository.AuditRepository
}

func NewAuditService(repo repository.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

type ListAuditsInput struct {
	Action     string
	EntityType string
	EntityID   string
	Result     string
	Limit      int
}

type AuditActorUser struct {
	ID    string
	Name  string
	Email string
	Role  string
}

type AuditLogItem struct {
	ID          string
	ActorUserID *string
	ActorRole   *string
	Action      string
	EntityType  string
	EntityID    string
	Result      string
	Metadata    map[string]any
	CreatedAt   time.Time
	ActorUser   *AuditActorUser
}

type ListAuditsOutput struct {
	Logs       []AuditLogItem
	NextCursor *string
}

type WriteAuditInput struct {
	ActorUserID *string
	ActorRole   *string
	Action      string
	EntityType  string
	EntityID    string
	Result      string
	Metadata    map[string]any
}

func (s *AuditService) ListAudits(ctx context.Context, in ListAuditsInput) (*ListAuditsOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.repo.List(ctx, repository.AuditListFilter{
		Action:     in.Action,
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		Result:     in.Result,
		Limit:      limit + 1,
	})
	if err != nil {
		return nil, err
	}

	items := make([]AuditLogItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAuditLogItem(row))
	}

	var nextCursor *string
	if len(items) > limit {
		lastID := items[len(items)-1].ID
		nextCursor = &lastID
		items = items[:len(items)-1]
	}

	return &ListAuditsOutput{
		Logs:       items,
		NextCursor: nextCursor,
	}, nil
}

func (s *AuditService) WriteAudit(ctx context.Context, in WriteAuditInput) error {
	payload, _ := json.Marshal(in.Metadata)
	row := &domain.AuditLog{
		ActorUserID: in.ActorUserID,
		ActorRole:   in.ActorRole,
		Action:      in.Action,
		EntityType:  in.EntityType,
		EntityID:    in.EntityID,
		Result:      in.Result,
		Metadata:    payload,
		CreatedAt:   time.Now().UTC(),
	}
	return s.repo.Create(ctx, row)
}

func toAuditLogItem(row domain.AuditLog) AuditLogItem {
	item := AuditLogItem{
		ID:          row.ID,
		ActorUserID: row.ActorUserID,
		ActorRole:   row.ActorRole,
		Action:      row.Action,
		EntityType:  row.EntityType,
		EntityID:    row.EntityID,
		Result:      row.Result,
		Metadata:    decodeMetadata(row.Metadata),
		CreatedAt:   row.CreatedAt,
	}
	if row.ActorUser != nil {
		item.ActorUser = &AuditActorUser{
			ID:    row.ActorUser.ID,
			Name:  row.ActorUser.Name,
			Email: row.ActorUser.Email,
			Role:  row.ActorUser.Role,
		}
	}
	return item
}

func decodeMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}
