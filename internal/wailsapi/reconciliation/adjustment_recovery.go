package reconciliation

import (
	"encoding/json"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

// Persist canonical decimal text, since domain decimal wrappers are deliberately
// opaque to encoding/json. Never rebuild these commands from current balances.
type savedAdjustmentCommand struct {
	Kind                                 string
	HouseholdID                          domain.HouseholdID
	AccountID                            domain.AccountID
	HoldingID                            domain.HoldingID
	Amount, Currency, Quantity, UnitCost string
	Added                                bool
	At                                   time.Time
	Note                                 *string
}
type savedAdjustmentPlan struct {
	ID, Hash, Token string
	Link            domain.ReconciliationLink
	Expires         time.Time
	Commands        []savedAdjustmentCommand
}

func (p adjustmentPlan) MarshalJSON() ([]byte, error) {
	v := savedAdjustmentPlan{ID: p.id, Hash: p.hash, Token: p.token, Link: p.link, Expires: p.expires}
	for _, c := range p.commands {
		var x savedAdjustmentCommand
		switch d := c.(type) {
		case domain.ValueUpdateInput:
			x = savedAdjustmentCommand{Kind: "value", HouseholdID: d.HouseholdID, AccountID: d.AccountID, Amount: d.NewValue.CanonicalAmount(), Currency: d.NewValue.Currency().String(), At: d.EffectiveAt, Note: d.Note}
		case domain.MoneyAddedInput:
			x = savedAdjustmentCommand{Kind: "add", HouseholdID: d.HouseholdID, AccountID: d.AccountID, Amount: d.Amount.CanonicalAmount(), Currency: d.Amount.Currency().String(), At: d.EffectiveAt, Note: d.Note}
		case domain.MoneyRemovedInput:
			x = savedAdjustmentCommand{Kind: "remove", HouseholdID: d.HouseholdID, AccountID: d.AccountID, Amount: d.Amount.CanonicalAmount(), Currency: d.Amount.Currency().String(), At: d.EffectiveAt, Note: d.Note}
		case domain.PositionAdjustmentInput:
			x = savedAdjustmentCommand{Kind: "quantity", HouseholdID: d.HouseholdID, HoldingID: d.HoldingID, Quantity: d.Quantity.Canonical(), Added: d.Added, At: d.EffectiveAt, Note: d.Note}
			if d.UnitCost != nil {
				x.UnitCost = d.UnitCost.Canonical()
			}
		case domain.PositionCostAdjustmentInput:
			x = savedAdjustmentCommand{Kind: "cost", HouseholdID: d.HouseholdID, HoldingID: d.HoldingID, UnitCost: d.UnitCost.Canonical(), At: d.EffectiveAt, Note: d.Note}
		default:
			return nil, fmt.Errorf("unsupported reconciliation command %T", c)
		}
		v.Commands = append(v.Commands, x)
	}
	return json.Marshal(v)
}
func (p *adjustmentPlan) UnmarshalJSON(raw []byte) error {
	var v savedAdjustmentPlan
	if e := json.Unmarshal(raw, &v); e != nil {
		return e
	}
	*p = adjustmentPlan{id: v.ID, hash: v.Hash, token: v.Token, link: v.Link, expires: v.Expires}
	for _, c := range v.Commands {
		var command any
		switch c.Kind {
		case "value", "add", "remove":
			currency, e := domain.ParseSupportedCurrency(c.Currency)
			if e != nil {
				return e
			}
			m, e := domain.ParseMoney(c.Amount, currency)
			if e != nil {
				return e
			}
			switch c.Kind {
			case "value":
				command = domain.ValueUpdateInput{HouseholdID: c.HouseholdID, AccountID: c.AccountID, NewValue: m, Reason: domain.ReasonReconciliation, EffectiveAt: c.At, Note: c.Note}
			case "add":
				command = domain.MoneyAddedInput{HouseholdID: c.HouseholdID, AccountID: c.AccountID, Amount: m, Reason: domain.ReasonReconciliation, EffectiveAt: c.At, Note: c.Note}
			case "remove":
				command = domain.MoneyRemovedInput{HouseholdID: c.HouseholdID, AccountID: c.AccountID, Amount: m, Reason: domain.ReasonReconciliation, EffectiveAt: c.At, Note: c.Note}
			}
		case "quantity":
			q, e := domain.ParseQuantity(c.Quantity)
			if e != nil {
				return e
			}
			var unit *domain.UnitPrice
			if c.UnitCost != "" {
				u, e := domain.ParseUnitPrice(c.UnitCost)
				if e != nil {
					return e
				}
				unit = &u
			}
			command = domain.PositionAdjustmentInput{HouseholdID: c.HouseholdID, HoldingID: c.HoldingID, Quantity: q, Added: c.Added, UnitCost: unit, EffectiveAt: c.At, Note: c.Note}
		case "cost":
			unit, e := domain.ParseUnitPrice(c.UnitCost)
			if e != nil {
				return e
			}
			command = domain.PositionCostAdjustmentInput{HouseholdID: c.HouseholdID, HoldingID: c.HoldingID, UnitCost: unit, EffectiveAt: c.At, Note: c.Note}
		default:
			return fmt.Errorf("invalid saved command")
		}
		p.commands = append(p.commands, command)
	}
	return nil
}
