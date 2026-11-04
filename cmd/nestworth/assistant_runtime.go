package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	nestworthapp "github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aistate"
	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/providers/einoopenai"
)

// All bindings and paths originate in the desktop host. No mobile principal or
// client-provided household/ledger/path/mode is accepted. Provider activation is
// disabled by default; credential/configuration mutation is not bound to IPC.
func openAssistantRuntime(app *nestworthapp.Service, databasePath, settingsPath string) (*assistant.Runtime, error) {
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
	factory := func(ctx context.Context, cfg assistant.ModelConfig, key []byte, conversationID string) (assistant.Engine, error) {
		session, err := mcpserver.NewInProcessSession(ctx, app, filepath.Join(directory, "capabilities"), conversationID, mcpserver.ReadOnly, nil)
		if err != nil {
			return nil, assistant.ErrUnavailable
		}
		engine, err := einoopenai.NewEngine(ctx, cfg, key, session, session.Close)
		if err != nil {
			session.Close()
			return nil, assistant.ErrUnavailable
		}
		return engine, nil
	}
	r, err := assistant.NewRuntime(assistant.Options{Ledger: ledger, Store: state, Credentials: aicredentials.New(ledger), Factory: factory,
		Validate: func(ctx context.Context) bool { return ctx.Err() == nil && app.MobileWorkspace() == incarnation },
	})
	if err != nil {
		_ = state.Close()
		return nil, err
	}
	return r, nil
}
