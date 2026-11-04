//go:build !darwin || !cgo

package aicredentials

import "github.com/waltwang/nestworth-go/internal/assistant"

func New(scope string) assistant.Credentials { return unavailable{} }
