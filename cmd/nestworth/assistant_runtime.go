package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	nestworthapp "github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/builtinai"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/providers/einoopenai"
)

// All bindings and paths originate in the desktop host. No mobile principal or
// client-provided household/ledger/path/mode is accepted. Provider activation is
// disabled by default and requires explicit desktop configuration.
func openAssistantRuntime(app *nestworthapp.Service, databasePath, settingsPath string, database *sqlite.DB) (*assistant.Runtime, error) {
	if app == nil {
		return nil, assistant.ErrUnavailable
	}
	b, err := app.Bootstrap(context.Background())
	if err != nil || b.Household == nil {
		return nil, assistant.ErrUnavailable
	}
	path, err := filepath.EvalSymlinks(databasePath)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, assistant.ErrUnavailable
	}
	sum := sha256.Sum256([]byte(path + "\x00" + b.Household.ID.String()))
	ledger := hex.EncodeToString(sum[:])
	directory := filepath.Join(filepath.Dir(settingsPath), "assistant-private", ledger)
	state, err := aistate.Open(directory, ledger)
	if err != nil {
		return nil, assistant.ErrStorage
	}
	incarnation := app.MobileWorkspace()
	pool := builtinai.NewPool(app, filepath.Join(directory, "capabilities"))
	factory := func(ctx context.Context, cfg assistant.ModelConfig, key []byte, conversationID string) (assistant.Engine, error) {
		session, err := pool.Get(ctx, conversationID)
		if err != nil {
			return nil, assistant.ErrUnavailable
		}
		engine, err := einoopenai.NewEngine(ctx, cfg, key, builtinai.New(session, assistant.SelectedPage(ctx), assistant.ModelNavigationEnabled(ctx)), nil)
		if err != nil {
			return nil, assistant.ErrUnavailable
		}
		return engine, nil
	}
	r, err := assistant.NewRuntime(assistant.Options{Capabilities: pool, Ledger: ledger, Store: state, Credentials: aicredentials.New(sqlite.NewConfigurationRepository(database)), Factory: factory,
		NavigationValidator: func(ctx context.Context, target assistant.ModelNavigationTarget) bool {
			if !target.Valid() || ctx.Err() != nil || app.MobileWorkspace() != incarnation {
				return false
			}
			if target.Kind != "account" {
				return true
			}
			accounts, err := app.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{domain.AccountID(target.AccountID)}, IncludeArchived: true})
			return err == nil && len(accounts) == 1 && accounts[0].Account.ID.String() == target.AccountID
		},
		Validate: func(ctx context.Context) bool { return ctx.Err() == nil && app.MobileWorkspace() == incarnation },
	})
	if err != nil {
		pool.Close()
		_ = state.Close()
		return nil, err
	}
	return r, nil
}
