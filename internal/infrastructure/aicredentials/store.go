// Package aicredentials owns system secure-store access. There is no plaintext
// fallback, environment lookup, external command, logging or backup serialization.
package aicredentials

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/assistant"
)

var errUnavailable = errors.New("system secure credentials unavailable")

type unavailable struct{}

func (unavailable) Available() bool                                { return false }
func (unavailable) Put(context.Context, []byte) error              { return errUnavailable }
func (unavailable) With(context.Context, func([]byte) error) error { return errUnavailable }
func (unavailable) Delete(context.Context) error                   { return errUnavailable }

var _ assistant.Credentials = unavailable{}
