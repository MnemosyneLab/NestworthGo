// Package media adapts internal/application.Service's image normalization
// and MediaAsset persistence for the Wails IPC boundary, and implements the
// native file-picker flow.
package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// Dialog is the minimal surface this service needs from Wails's native
// file-open dialog. It is a local interface (not github.com/wailsapp/
// wails/v3) so this package stays importable and unit-testable without the
// Wails runtime; the real application.App.Dialog.OpenFile()-based adapter
// is wired in cmd/nestworth's main.go.
type Dialog interface {
	// OpenFile prompts for a single image file and returns its path, or an
	// empty string if the user cancelled (not an error).
	OpenFile(title string) (string, error)
}

type Service struct {
	app    *application.Service
	dialog Dialog
}

func NewService(app *application.Service, dialog Dialog) *Service {
	return &Service{app: app, dialog: dialog}
}

// MediaAssetDTO mirrors domain.MediaAsset. Data crosses the wire as a
// base64 string because Go's encoding/json already base64-encodes a []byte
// field by default; unlike domain.Money/Quantity,
// no additional fix is needed for this type.
type MediaAssetDTO struct {
	ID          string `json:"id"`
	HouseholdID string `json:"householdId"`
	MimeType    string `json:"mimeType"`
	Data        string `json:"data"`
	CreatedAt   string `json:"createdAt"`
}

func fromMediaAsset(value domain.MediaAsset) MediaAssetDTO {
	return MediaAssetDTO{
		ID: value.ID.String(), HouseholdID: value.HouseholdID.String(), MimeType: value.MimeType,
		Data: base64.StdEncoding.EncodeToString(value.Data), CreatedAt: wire.FormatTime(value.CreatedAt),
	}
}

func (s *Service) CreateMediaAsset(ctx context.Context, mimeType, dataBase64 string) (MediaAssetDTO, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return MediaAssetDTO{}, apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Field: "data", Message: "must be base64-encoded"})
	}
	asset, err := s.app.CreateMediaAsset(ctx, mimeType, data)
	if err != nil {
		return MediaAssetDTO{}, apierror.Wrap(err)
	}
	return fromMediaAsset(asset), nil
}

func (s *Service) MediaAsset(ctx context.Context, id string) (MediaAssetDTO, error) {
	assetID, err := domain.ParseMediaAssetID(id)
	if err != nil {
		return MediaAssetDTO{}, apierror.Wrap(err)
	}
	asset, err := s.app.MediaAsset(ctx, assetID)
	if err != nil {
		return MediaAssetDTO{}, apierror.Wrap(err)
	}
	return fromMediaAsset(asset), nil
}

// NormalizeImage normalizes already-picked image bytes to PNG without
// persisting anything, mirroring application.Service.NormalizeImage's
// pick-time/confirm-time split.
func (s *Service) NormalizeImage(dataBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Field: "data", Message: "must be base64-encoded"})
	}
	normalized, err := s.app.NormalizeImage(bytes.NewReader(data))
	if err != nil {
		return "", apierror.Wrap(err)
	}
	return base64.StdEncoding.EncodeToString(normalized), nil
}

// PickImage opens the native file dialog, reads and normalizes the
// selected file, and returns the normalized PNG bytes as base64 — ready
// for the caller to pass to CreateMediaAsset once the surrounding form is
// confirmed. A nil, nil result (no error, empty string) means the user
// cancelled; this is not an error condition.
func (s *Service) PickImage(title string) (string, error) {
	path, err := s.dialog.OpenFile(title)
	if err != nil {
		return "", apierror.Wrap(err)
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", apierror.Wrap(&domain.Error{Code: domain.ErrValidation, Field: "data", Message: "selected file could not be read"})
	}
	normalized, err := s.app.NormalizeImage(bytes.NewReader(data))
	if err != nil {
		return "", apierror.Wrap(err)
	}
	return base64.StdEncoding.EncodeToString(normalized), nil
}
