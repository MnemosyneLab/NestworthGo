package einoopenai

import (
	"sort"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const loadToolGroups = "load_tool_groups"

var groupOrder = [...]agentcapability.ToolGroup{
	agentcapability.AccountsTools, agentcapability.HistoryTools, agentcapability.InvestmentsTools,
	agentcapability.MarketDataTools, agentcapability.ReconciliationTools, agentcapability.MaintenanceTools,
	agentcapability.ChangesTools,
}

// turnCatalog is local to Complete. A successful selection appends schemas to
// the current prefix in a stable order. Nothing persisted can load a group or
// turn historical metadata into execution authority.
type turnCatalog struct {
	infos  []*schema.ToolInfo
	active map[string]bool
	groups map[agentcapability.ToolGroup][]*schema.ToolInfo
	loaded map[agentcapability.ToolGroup]bool
}

func (e *Engine) newTurnCatalog(full bool) (*turnCatalog, error) {
	out := &turnCatalog{active: map[string]bool{}, groups: map[agentcapability.ToolGroup][]*schema.ToolInfo{}, loaded: map[agentcapability.ToolGroup]bool{}}
	if e.catalog == nil {
		out.infos = e.infos
		for name := range e.tools {
			out.active[name] = true
		}
		return out, nil
	}
	definitions, err := e.catalog(full)
	if err != nil {
		return nil, errEngine
	}
	core := make([]agentcapability.Definition, 0, len(definitions))
	names := make(map[string]bool, len(definitions))
	for _, d := range definitions {
		if d.Name == loadToolGroups || names[d.Name] {
			return nil, errEngine
		}
		names[d.Name] = true
		if d.ToolGroup() == agentcapability.CoreTools {
			core = append(core, d)
		}
	}
	for _, info := range e.infos {
		if !names[info.Name] {
			return nil, errEngine
		}
		if group := e.groups[info.Name]; group != agentcapability.CoreTools {
			out.groups[group] = append(out.groups[group], info)
		} else {
			out.active[info.Name] = true
		}
	}
	for _, infos := range out.groups {
		sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	}
	out.infos, err = catalogInfos(core)
	if err != nil {
		return nil, errEngine
	}
	if len(out.groups) > 0 {
		loader, err := catalogInfos([]agentcapability.Definition{groupLoaderDefinition()})
		if err != nil {
			return nil, errEngine
		}
		out.infos = append(out.infos, loader...)
		out.active[loadToolGroups] = true
	}
	return out, nil
}

func groupLoaderDefinition() agentcapability.Definition {
	groups := groupOrder[:]
	return agentcapability.Definition{
		Name:        loadToolGroups,
		Description: "Load extra tool schemas for this turn. Choose only groups needed: accounts (directory, holdings, liquidity); history (activity, historical overview, period analysis and contributions); investments (instruments, products and investment review); market_data (quotes, FX and imports comparison); reconciliation (sessions and evidence); maintenance (data health and repair status); changes (available previews and mutations). Use the loaded tools in a later response. Loading does not grant permissions or dispatch a financial operation.",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"groups"},
			"properties": map[string]any{"groups": map[string]any{
				"type": "array", "minItems": 1, "maxItems": len(groups), "uniqueItems": true,
				"items": map[string]any{"type": "string", "enum": groups},
			}},
		},
	}
}

func (c *turnCatalog) load(arguments string) (string, error) {
	var q struct {
		Groups []agentcapability.ToolGroup `json:"groups"`
	}
	var fields map[string]any
	if strictJSON([]byte(arguments)) != nil || jsoncodec.Unmarshal([]byte(arguments), &fields) != nil ||
		len(fields) != 1 || fields["groups"] == nil || jsoncodec.Unmarshal([]byte(arguments), &q) != nil ||
		len(q.Groups) == 0 || len(q.Groups) > len(groupOrder) {
		return "", newReadFailure("validation", "groups")
	}
	requested := map[agentcapability.ToolGroup]bool{}
	for _, group := range q.Groups {
		known := false
		for _, candidate := range groupOrder {
			known = known || group == candidate
		}
		if !known || requested[group] {
			return "", newReadFailure("validation", "groups")
		}
		requested[group] = true
	}
	result := struct {
		Available   []agentcapability.ToolGroup `json:"availableGroups"`
		Loaded      []agentcapability.ToolGroup `json:"loadedGroups"`
		Unavailable []agentcapability.ToolGroup `json:"unavailableGroups,omitempty"`
	}{Available: []agentcapability.ToolGroup{}, Loaded: []agentcapability.ToolGroup{}}
	for _, group := range groupOrder {
		if len(c.groups[group]) == 0 {
			if requested[group] {
				result.Unavailable = append(result.Unavailable, group)
			}
			continue
		}
		result.Available = append(result.Available, group)
		if requested[group] && !c.loaded[group] {
			c.loaded[group] = true
			c.infos = append(c.infos, c.groups[group]...)
			for _, info := range c.groups[group] {
				c.active[info.Name] = true
			}
		}
		if c.loaded[group] {
			result.Loaded = append(result.Loaded, group)
		}
	}
	raw, err := jsoncodec.Marshal(result)
	return string(raw), err
}

func (c *turnCatalog) choice(tools map[string]tool.InvokableTool) *schema.AgenticToolChoice {
	allowed := make([]string, 0, len(c.active))
	for name, active := range c.active {
		if active && (name == loadToolGroups || tools[name] != nil) {
			allowed = append(allowed, name)
		}
	}
	if len(allowed) == 0 {
		return &schema.AgenticToolChoice{Type: schema.ToolChoiceForbidden}
	}
	sort.Strings(allowed)
	list := make([]*schema.AllowedTool, 0, len(allowed))
	for _, name := range allowed {
		list = append(list, &schema.AllowedTool{FunctionName: name})
	}
	return &schema.AgenticToolChoice{Type: schema.ToolChoiceAllowed, Allowed: &schema.AgenticAllowedToolChoice{Tools: list}}
}
