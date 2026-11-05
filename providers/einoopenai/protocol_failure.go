package einoopenai

import (
	"errors"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

func protocolFailure(rule string, err error) error {
	if err == nil || errors.Is(err, errLimit) {
		return err
	}
	if assistant.IsResponseFailureCode(assistant.FailureCode(err)) {
		return err
	}
	return assistant.ProviderResponseFailure(rule)
}

func streamRule(event string) string {
	switch event {
	case "response.created", "response.in_progress":
		return "stream_lifecycle"
	case "response.output_item.added":
		return "stream_item_added"
	case "response.output_item.done":
		return "stream_item_done"
	case "response.completed":
		return "stream_terminal"
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		return "stream_arguments"
	case "response.content_part.added", "response.content_part.done", "response.output_text.delta", "response.output_text.done", "response.refusal.delta", "response.refusal.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_text.delta", "response.reasoning_text.done":
		return "stream_part"
	default:
		return "stream_event"
	}
}
