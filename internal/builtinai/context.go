package builtinai

import (
	"encoding/json"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
)

// Project only authority metadata. Household/history facts and exact values
// retain the shared response; the storage session's mode is not model authority.
func (s *Session) effectiveContext(raw json.RawMessage) (json.RawMessage, error) {
	definitions, err := s.Definitions()
	if err != nil {
		return nil, err
	}
	var response, data map[string]json.RawMessage
	if jsoncodec.Unmarshal(raw, &response) != nil || response == nil || jsoncodec.Unmarshal(response["data"], &data) != nil || data == nil {
		return nil, assistant.ErrUnavailable
	}
	mode := mcpserver.ReadOnly
	if s.FullAccessEnabled() {
		mode = mcpserver.ReadWrite
	}
	data["mode"], err = jsoncodec.Marshal(mode)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	data["capabilities"], err = jsoncodec.Marshal(agentcapability.Features(definitions))
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	response["data"], err = jsoncodec.Marshal(data)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	return jsoncodec.Marshal(response)
}
