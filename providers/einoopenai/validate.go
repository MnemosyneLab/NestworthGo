package einoopenai

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Decode through the repository codec after rejecting ambiguous duplicate names,
// invalid UTF-8, excessive nesting and trailing data. Never log provider text.
func object(raw []byte) (map[string]json.RawMessage, error) {
	if strictJSON(raw) != nil {
		return nil, errProtocol
	}
	var obj map[string]json.RawMessage
	if jsoncodec.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, errProtocol
	}
	return obj, nil
}
func str(obj map[string]json.RawMessage, key string) string {
	var s string
	if jsoncodec.Unmarshal(obj[key], &s) != nil {
		return ""
	}
	return s
}
func textField(obj map[string]json.RawMessage, key string) (string, error) {
	raw := bytes.TrimSpace(obj[key])
	var text string
	if len(raw) == 0 || raw[0] != '"' || jsoncodec.Unmarshal(raw, &text) != nil {
		return "", errProtocol
	}
	return text, nil
}
func index(obj map[string]json.RawMessage, key string) (int, error) {
	n, err := strconv.Atoi(string(obj[key]))
	if err != nil || n < 0 {
		return 0, errProtocol
	}
	return n, nil
}
func same(a, b []byte) bool {
	var av, bv any
	da, db := jsoncodec.NewDecoder(bytes.NewReader(a)), jsoncodec.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	db.UseNumber()
	return da.Decode(&av) == nil && db.Decode(&bv) == nil && reflect.DeepEqual(av, bv)
}
func nullOrAbsent(v json.RawMessage) bool {
	return len(v) == 0 || bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}
func validateResponse(raw []byte, limits Limits) ([]json.RawMessage, error) {
	obj, err := object(raw)
	if err != nil || str(obj, "object") != "response" || str(obj, "id") == "" || str(obj, "status") != "completed" || !nullOrAbsent(obj["error"]) || !nullOrAbsent(obj["incomplete_details"]) {
		return nil, errProtocol
	}
	var output []json.RawMessage
	if jsoncodec.Unmarshal(obj["output"], &output) != nil || output == nil {
		return nil, errProtocol
	}
	if len(output) > limits.Items {
		return nil, errLimit
	}
	ids, calls := map[string]bool{}, map[string]bool{}
	for _, raw := range output {
		item, err := validateItem(raw)
		if err != nil {
			return nil, err
		}
		id := str(item, "id")
		if ids[id] {
			return nil, errProtocol
		}
		ids[id] = true
		if str(item, "type") == "function_call" {
			id = str(item, "call_id")
			if calls[id] {
				return nil, errProtocol
			}
			calls[id] = true
		}
	}
	return output, nil
}
func validateItem(raw []byte) (map[string]json.RawMessage, error) {
	item, err := object(raw)
	if err != nil || str(item, "id") == "" {
		return nil, errProtocol
	}
	status := str(item, "status")
	switch str(item, "type") {
	case "function_call":
		args := str(item, "arguments")
		if status != "completed" || str(item, "name") == "" || str(item, "call_id") == "" || len(args) > 64<<10 {
			return nil, errProtocol
		}
		if _, err := object([]byte(args)); err != nil {
			return nil, errProtocol
		}
	case "message":
		if status != "completed" || str(item, "role") != "assistant" {
			return nil, errProtocol
		}
		var parts []map[string]json.RawMessage
		if jsoncodec.Unmarshal(item["content"], &parts) != nil || parts == nil {
			return nil, errProtocol
		}
		for _, part := range parts {
			switch str(part, "type") {
			case "output_text":
				var annotations []json.RawMessage
				if _, err := textField(part, "text"); err != nil {
					return nil, errProtocol
				}
				if jsoncodec.Unmarshal(part["annotations"], &annotations) != nil || annotations == nil || len(annotations) != 0 {
					return nil, errProtocol
				}
			case "refusal":
				if _, err := textField(part, "refusal"); err != nil {
					return nil, errProtocol
				}
			default:
				return nil, errProtocol
			}
		}
	case "reasoning":
		if (len(item["status"]) != 0 && status != "completed") || str(item, "encrypted_content") == "" {
			return nil, errProtocol
		}
		var summary []map[string]json.RawMessage
		if jsoncodec.Unmarshal(item["summary"], &summary) != nil || summary == nil {
			return nil, errProtocol
		}
		for _, part := range summary {
			_, err := textField(part, "text")
			if str(part, "type") != "summary_text" || err != nil {
				return nil, errProtocol
			}
		}
		if content, ok := item["content"]; ok {
			var parts []map[string]json.RawMessage
			if jsoncodec.Unmarshal(content, &parts) != nil || parts == nil {
				return nil, errProtocol
			}
			for _, part := range parts {
				_, err := textField(part, "text")
				if str(part, "type") != "reasoning_text" || err != nil {
					return nil, errProtocol
				}
			}
		}
	default:
		return nil, errProtocol
	}
	return item, nil
}

type streamItem struct {
	added              map[string]json.RawMessage
	done               json.RawMessage
	args               strings.Builder
	argsSeen, argsDone bool
	parts              map[string]*streamPart
}

func validateStream(raw []byte, limits Limits) ([]byte, []json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, nil, errProtocol
	}
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return nil, nil, errProtocol
	}
	var items []*streamItem
	var final []byte
	responseID := ""
	sequence := -1
	frames := 0
	doneSent := false
	eventName := ""
	var data []string
	process := func() error {
		if len(data) == 0 {
			if eventName != "" {
				return errProtocol
			}
			eventName = ""
			return nil
		}
		frames++
		if frames > limits.Events {
			return errLimit
		}
		payload := []byte(strings.Join(data, "\n"))
		data = nil
		event := eventName
		eventName = ""
		if bytes.Equal(payload, []byte("[DONE]")) {
			if final == nil || doneSent || event != "" {
				return errProtocol
			}
			doneSent = true
			return nil
		}
		if final != nil || doneSent {
			return errProtocol
		}
		obj, err := object(payload)
		if err != nil {
			return err
		}
		typ := str(obj, "type")
		if event != "" && event != typ {
			return errProtocol
		}
		seq, err := index(obj, "sequence_number")
		if err != nil || seq <= sequence {
			return errProtocol
		}
		sequence = seq
		switch typ {
		case "response.created":
			resp, err := object(obj["response"])
			if err != nil || responseID != "" || len(items) != 0 || str(resp, "object") != "response" || str(resp, "status") != "in_progress" || str(resp, "id") == "" || !nullOrAbsent(resp["error"]) || !nullOrAbsent(resp["incomplete_details"]) {
				return errProtocol
			}
			responseID = str(resp, "id")
		case "response.in_progress":
			resp, err := object(obj["response"])
			if err != nil || responseID == "" || str(resp, "id") != responseID || str(resp, "status") != "in_progress" || !nullOrAbsent(resp["error"]) || !nullOrAbsent(resp["incomplete_details"]) {
				return errProtocol
			}
		case "response.output_item.added":
			idx, err := index(obj, "output_index")
			if err != nil || responseID == "" || idx != len(items) {
				return errProtocol
			}
			if len(items) >= limits.Items {
				return errLimit
			}
			item, err := object(obj["item"])
			if err != nil || str(item, "id") == "" {
				return errProtocol
			}
			for _, prior := range items {
				if str(prior.added, "id") == str(item, "id") {
					return errProtocol
				}
			}
			switch str(item, "type") {
			case "function_call", "message", "reasoning":
			default:
				return errProtocol
			}
			if err := validateAdded(item); err != nil {
				return err
			}
			items = append(items, &streamItem{added: item, parts: map[string]*streamPart{}})
		case "response.output_item.done":
			idx, err := index(obj, "output_index")
			if err != nil || idx >= len(items) || items[idx].done != nil {
				return errProtocol
			}
			item, err := validateItem(obj["item"])
			if err != nil {
				return err
			}
			prior := items[idx]
			for _, key := range []string{"id", "type", "name", "call_id", "role"} {
				if str(prior.added, key) != str(item, key) {
					return errProtocol
				}
			}
			if old := str(prior.added, "encrypted_content"); old != "" && old != str(item, "encrypted_content") {
				return errProtocol
			}
			if prior.argsSeen && prior.args.String() != str(item, "arguments") {
				return errProtocol
			}
			if err := finishParts(prior, item); err != nil {
				return err
			}
			prior.done = obj["item"]
		case "response.completed":
			output, err := validateResponse(obj["response"], limits)
			if err != nil {
				return err
			}
			resp, _ := object(obj["response"])
			if responseID == "" || str(resp, "id") != responseID || len(output) != len(items) {
				return errProtocol
			}
			for i, item := range items {
				if item.done == nil || !same(item.done, output[i]) {
					return errProtocol
				}
			}
			final = append([]byte(nil), obj["response"]...)
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			item, err := liveItem(obj, items, "function_call")
			if err != nil || item.argsDone {
				return errProtocol
			}
			if typ == "response.function_call_arguments.delta" {
				delta, err := textField(obj, "delta")
				if err != nil {
					return errProtocol
				}
				if item.args.Len()+len(delta) > 64<<10 {
					return errLimit
				}
				item.argsSeen = true
				item.args.WriteString(delta)
			} else {
				if !item.argsSeen || str(obj, "arguments") != item.args.String() {
					return errProtocol
				}
				item.argsDone = true
			}
		case "response.content_part.added", "response.content_part.done", "response.output_text.delta", "response.output_text.done", "response.refusal.delta", "response.refusal.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_text.delta", "response.reasoning_text.done":
			item, err := liveItem(obj, items, "")
			if err != nil {
				return err
			}
			if err := consumePart(item, obj, typ); err != nil {
				return err
			}

		default:
			return errProtocol // failed/incomplete/cancelled/error and unsupported features
		}
		return nil
	}
	lines := strings.Split(normalized, "\n")
	if strings.HasSuffix(normalized, "\n") {
		lines = lines[:len(lines)-1] // EOF is not an SSE blank-line delimiter.
	}
	for _, line := range lines {
		if line == "" {
			if err := process(); err != nil {
				return nil, nil, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, nil, errProtocol
		}
		value = strings.TrimPrefix(value, " ")
		switch key {
		case "event":
			if eventName != "" {
				return nil, nil, errProtocol
			}
			eventName = value
		case "data":
			data = append(data, value)
		default:
			return nil, nil, errProtocol
		}
	}
	if len(data) != 0 || eventName != "" || final == nil {
		return nil, nil, errProtocol
	}
	output, err := validateResponse(final, limits)
	return final, output, err
}
func liveItem(obj map[string]json.RawMessage, items []*streamItem, typ string) (*streamItem, error) {
	idx, err := index(obj, "output_index")
	if err != nil || idx >= len(items) {
		return nil, errProtocol
	}
	item := items[idx]
	if item.done != nil || str(item.added, "id") != str(obj, "item_id") || (typ != "" && str(item.added, "type") != typ) {
		return nil, errProtocol
	}
	return item, nil
}

// Only duplicate-name/nesting checks use this scanner, after codec validation.
// Actual JSON encoding and decoding always go through internal/jsoncodec.
func strictJSON(raw []byte) error {
	if !utf8.Valid(raw) || !jsoncodec.Valid(raw) {
		return errProtocol
	}
	i := 0
	space := func() {
		for i < len(raw) && strings.ContainsRune(" \t\r\n", rune(raw[i])) {
			i++
		}
	}
	stringEnd := func() int {
		start := i
		i++
		for i < len(raw) {
			c := raw[i]
			i++
			if c == '\\' {
				i++
			} else if c == '"' {
				break
			}
		}
		return start
	}
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return errLimit
		}
		space()
		switch raw[i] {
		case '{':
			i++
			space()
			names := map[string]bool{}
			if raw[i] == '}' {
				i++
				return nil
			}
			for {
				start := stringEnd()
				var key string
				if jsoncodec.Unmarshal(raw[start:i], &key) != nil || names[key] {
					return errProtocol
				}
				names[key] = true
				space()
				i++
				if err := walk(depth + 1); err != nil {
					return err
				}
				space()
				c := raw[i]
				i++
				if c == '}' {
					return nil
				}
				space()
			}
		case '[':
			i++
			space()
			if raw[i] == ']' {
				i++
				return nil
			}
			for {
				if err := walk(depth + 1); err != nil {
					return err
				}
				space()
				c := raw[i]
				i++
				if c == ']' {
					return nil
				}
			}
		case '"':
			stringEnd()
		default:
			for i < len(raw) && !strings.ContainsRune(",]} \t\n\r", rune(raw[i])) {
				i++
			}
		}
		return nil
	}
	return walk(0)
}

func validateAdded(item map[string]json.RawMessage) error {
	typ, status := str(item, "type"), str(item, "status")
	if typ == "reasoning" {
		if len(item["status"]) != 0 && status != "in_progress" {
			return errProtocol
		}
		var summary []json.RawMessage
		if jsoncodec.Unmarshal(item["summary"], &summary) != nil || summary == nil {
			return errProtocol
		}
		for _, part := range summary {
			if _, err := partText(part, "summary_text"); err != nil {
				return err
			}
		}
		if _, ok := item["encrypted_content"]; ok {
			if _, err := textField(item, "encrypted_content"); err != nil {
				return err
			}
		}
		return nil
	}
	if status != "in_progress" {
		return errProtocol
	}
	if typ == "function_call" {
		args, err := textField(item, "arguments")
		if str(item, "name") == "" || str(item, "call_id") == "" || err != nil || args != "" {
			return errProtocol
		}
	} else {
		var parts []json.RawMessage
		if str(item, "role") != "assistant" || jsoncodec.Unmarshal(item["content"], &parts) != nil || parts == nil || len(parts) != 0 {
			return errProtocol
		}
	}
	return nil
}

type streamPart struct {
	group, kind    string
	index          int
	text           strings.Builder
	textDone, done bool
}

func partText(raw []byte, expected string) (string, error) {
	part, err := object(raw)
	if err != nil || str(part, "type") != expected {
		return "", errProtocol
	}
	key := "text"
	if expected == "refusal" {
		key = "refusal"
	}
	text, err := textField(part, key)
	if err != nil {
		return "", errProtocol
	}
	if expected == "output_text" {
		var annotations []json.RawMessage
		if jsoncodec.Unmarshal(part["annotations"], &annotations) != nil || annotations == nil || len(annotations) != 0 {
			return "", errProtocol
		}
	}
	return text, nil
}
func consumePart(item *streamItem, obj map[string]json.RawMessage, event string) error {
	group, key, kind := "content", "content_index", ""
	if strings.HasPrefix(event, "response.reasoning_summary_") {
		group, key, kind = "summary", "summary_index", "summary_text"
	}
	typ := str(item.added, "type")
	if typ == "function_call" || (group == "summary" && typ != "reasoning") {
		return errProtocol
	}
	if kind == "" {
		if strings.Contains(event, "refusal.") {
			kind = "refusal"
		} else if strings.Contains(event, "reasoning_text.") {
			kind = "reasoning_text"
		} else if strings.Contains(event, "output_text.") {
			kind = "output_text"
		} else {
			part, err := object(obj["part"])
			if err != nil {
				return err
			}
			kind = str(part, "type")
		}
	}
	if (typ == "message" && kind != "output_text" && kind != "refusal") || (typ == "reasoning" && kind != "summary_text" && kind != "reasoning_text") {
		return errProtocol
	}
	idx, err := index(obj, key)
	if err != nil || idx >= 128 {
		return errProtocol
	}
	identity := group + ":" + strconv.Itoa(idx)
	part := item.parts[identity]
	if strings.HasSuffix(event, "part.added") {
		if part != nil {
			return errProtocol
		}
		text, err := partText(obj["part"], kind)
		if err != nil {
			return err
		}
		part = &streamPart{group: group, kind: kind, index: idx}
		part.text.WriteString(text)
		item.parts[identity] = part
		return nil
	}
	if part == nil || part.done || part.kind != kind {
		return errProtocol
	}
	if strings.HasSuffix(event, "part.done") {
		text, err := partText(obj["part"], kind)
		if err != nil || text != part.text.String() {
			return errProtocol
		}
		part.done = true
		return nil
	}
	if part.textDone {
		return errProtocol
	}
	if strings.HasSuffix(event, ".delta") {
		delta, err := textField(obj, "delta")
		if err != nil {
			return errProtocol
		}
		part.text.WriteString(delta)
	} else {
		key := "text"
		if kind == "refusal" {
			key = "refusal"
		}
		text, err := textField(obj, key)
		if err != nil || text != part.text.String() {
			return errProtocol
		}
		part.textDone = true
	}
	return nil
}
func finishParts(item *streamItem, final map[string]json.RawMessage) error {
	for _, part := range item.parts {
		if !part.done {
			return errProtocol
		}
		var native []json.RawMessage
		if jsoncodec.Unmarshal(final[part.group], &native) != nil || part.index >= len(native) {
			return errProtocol
		}
		text, err := partText(native[part.index], part.kind)
		if err != nil || text != part.text.String() {
			return errProtocol
		}
	}
	return nil
}
