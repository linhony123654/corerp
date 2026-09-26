package httpapi

import (
	"context"
	"net/http"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

type rpInformationSendReceipt struct {
	MessageID            string `json:"message_id"`
	DeliveryDueWorldTime string `json:"delivery_due_world_time"`
	EventSequence        int64  `json:"event_sequence"`
	Replayed             bool   `json:"replayed"`
}

type rpInformationStanceReceipt struct {
	MessageID     string `json:"message_id"`
	Stance        string `json:"stance"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	Replayed      bool   `json:"replayed"`
}

type rpInformationNoticeReceipt struct {
	MessageID     string `json:"message_id"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	Replayed      bool   `json:"replayed"`
}

// The command receipt is deliberately narrower than the private source fact:
// recipients, content, canonical sender IDs and evidence Event IDs stay inside
// the authenticated world service rather than crossing the client boundary.
func (s *Server) handleRPInformationSend(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPInformationSendRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPInformationSendRequest) (rpInformationSendReceipt, error) {
			record, err := s.service.SendRPInformation(ctx, r)
			if err != nil {
				return rpInformationSendReceipt{}, err
			}
			return rpInformationSendReceipt{MessageID: record.Fact.MessageID,
				DeliveryDueWorldTime: record.Fact.DeliverWorldTime,
				EventSequence:        record.EventSequence, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPInformationStance(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPInformationStanceRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPInformationStanceRequest) (rpInformationStanceReceipt, error) {
			record, err := s.service.RecordRPInformationStance(ctx, r)
			if err != nil {
				return rpInformationStanceReceipt{}, err
			}
			return rpInformationStanceReceipt{MessageID: record.Fact.MessageID,
				Stance: record.Fact.Stance, EventSequence: record.EventSequence,
				WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPInformationRelay(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPInformationRelayRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPInformationRelayRequest) (rpInformationSendReceipt, error) {
			record, err := s.service.RelayRPInformation(ctx, r)
			if err != nil {
				return rpInformationSendReceipt{}, err
			}
			return rpInformationSendReceipt{MessageID: record.Fact.MessageID,
				DeliveryDueWorldTime: record.Fact.DeliverWorldTime,
				EventSequence:        record.EventSequence, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPOrganizationNoticePublish(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPOrganizationNoticePublishRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPOrganizationNoticePublishRequest) (rpInformationNoticeReceipt, error) {
			record, err := s.service.PublishRPOrganizationNotice(ctx, r)
			if err != nil {
				return rpInformationNoticeReceipt{}, err
			}
			return rpInformationNoticeReceipt{MessageID: record.Fact.MessageID, EventSequence: record.EventSequence,
				WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPOrganizationNoticeAccess(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPOrganizationNoticeAccessRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPOrganizationNoticeAccessRequest) (rpInformationNoticeReceipt, error) {
			record, err := s.service.AccessRPOrganizationNotice(ctx, r)
			if err != nil {
				return rpInformationNoticeReceipt{}, err
			}
			return rpInformationNoticeReceipt{MessageID: record.Fact.MessageID, EventSequence: record.EventSequence,
				WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPOrganizationNoticeList(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSessionReadRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadRPOrganizationNotices(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPPublicNoticePublish(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPPublicNoticePublishRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPPublicNoticePublishRequest) (rpInformationNoticeReceipt, error) {
			record, err := s.service.PublishRPPublicNotice(ctx, r)
			if err != nil {
				return rpInformationNoticeReceipt{}, err
			}
			return rpInformationNoticeReceipt{MessageID: record.Fact.MessageID, EventSequence: record.EventSequence,
				WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPPublicNoticeAccess(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	handleBoundCommand(s, response, request, requestID, principalID,
		func(r *storage.RPPublicNoticeAccessRequest) *string { return &r.Binding.PrincipalID },
		func(ctx context.Context, r storage.RPPublicNoticeAccessRequest) (rpInformationNoticeReceipt, error) {
			record, err := s.service.AccessRPPublicNotice(ctx, r)
			if err != nil {
				return rpInformationNoticeReceipt{}, err
			}
			return rpInformationNoticeReceipt{MessageID: record.Fact.MessageID, EventSequence: record.EventSequence,
				WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
		})
}

func (s *Server) handleRPPublicNoticeList(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSessionReadRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadRPPublicNotices(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}
