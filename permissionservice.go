package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type permissionDecision struct {
	optionID  string
	cancelled bool
}

type pendingPermission struct {
	request  PermissionRequest
	response chan permissionDecision
}

type PermissionService struct {
	mu      sync.Mutex
	pending map[string]*pendingPermission
}

func NewPermissionService() *PermissionService {
	return &PermissionService{
		pending: map[string]*pendingPermission{},
	}
}

func (p *PermissionService) ServiceStartup(context.Context, application.ServiceOptions) error {
	return nil
}

func (p *PermissionService) ServiceShutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, pending := range p.pending {
		select {
		case pending.response <- permissionDecision{cancelled: true}:
		default:
		}
		close(pending.response)
		delete(p.pending, id)
	}

	return nil
}

func (p *PermissionService) ListPendingPermissions() []PermissionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]PermissionRequest, 0, len(p.pending))
	for _, pending := range p.pending {
		result = append(result, pending.request)
	}
	return result
}

func (p *PermissionService) ResolvePermission(requestID, selectedOptionID string, cancelled bool) error {
	p.mu.Lock()
	pending, ok := p.pending[requestID]
	if ok {
		delete(p.pending, requestID)
	}
	p.mu.Unlock()

	if !ok {
		return errors.New("permission request not found")
	}

	decision := permissionDecision{
		optionID:  selectedOptionID,
		cancelled: cancelled,
	}

	pending.response <- decision
	close(pending.response)
	emitSessionEvent(SessionEvent{
		SessionID: pending.request.SessionID,
		Kind:      "permission_resolved",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Permission: &PermissionRequest{
			RequestID: pending.request.RequestID,
			SessionID: pending.request.SessionID,
		},
	})
	return nil
}

func (p *PermissionService) requestPermission(ctx context.Context, sessionID, toolCallID, title, description string, options []PermissionOption) (permissionDecision, error) {
	request := PermissionRequest{
		RequestID:   uuid.NewString(),
		SessionID:   sessionID,
		ToolCallID:  toolCallID,
		Title:       title,
		Description: description,
		Options:     options,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	pending := &pendingPermission{
		request:  request,
		response: make(chan permissionDecision, 1),
	}

	p.mu.Lock()
	p.pending[request.RequestID] = pending
	p.mu.Unlock()

	emitSessionEvent(SessionEvent{
		SessionID:  sessionID,
		Kind:       "permission_request",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Permission: &request,
	})

	select {
	case decision := <-pending.response:
		return decision, nil
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.pending, request.RequestID)
		p.mu.Unlock()
		return permissionDecision{cancelled: true}, ctx.Err()
	}
}

func (p *PermissionService) cancelSession(sessionID string) {
	p.mu.Lock()
	ids := make([]string, 0)
	for id, pending := range p.pending {
		if pending.request.SessionID == sessionID {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		pending := p.pending[id]
		delete(p.pending, id)
		select {
		case pending.response <- permissionDecision{cancelled: true}:
		default:
		}
		close(pending.response)
	}
	p.mu.Unlock()
}
