package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Operation struct {
	ID        string              `json:"id"`
	Tool      string              `json:"tool"`
	Hash      string              `json:"hash"`
	Status    string              `json:"status"`
	CreatedAt string              `json:"createdAt"`
	Result    json.RawMessage     `json:"result,omitempty"`
	Error     *apierror.WireError `json:"error,omitempty"`
}

func safeError(err error) *apierror.WireError {
	var wire *apierror.WireError
	if errors.As(err, &wire) {
		return wire
	}
	wrapped := apierror.Wrap(err)
	if errors.As(wrapped, &wire) {
		return wire
	}
	return &apierror.WireError{Code: "internal", Message: "operation failed"}
}
func fail(code, message string) error { return &apierror.WireError{Code: code, Message: message} }
func (s *Service) operationPath(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", fail("validation", "operationId must be a UUID; reuse it only for identical retries")
	}
	return filepath.Join(s.dir, "operations", parsed.String()+".json"), nil
}
func (s *Service) GetOperation(id string) (Operation, error) {
	path, err := s.operationPath(id)
	if err != nil {
		return Operation{}, err
	}
	data, err := s.readPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		return Operation{}, fail("not_found", "operation not found")
	}
	if err != nil {
		return Operation{}, err
	}
	var op Operation
	if err = json.Unmarshal(data, &op); err != nil {
		return op, err
	}
	// A persisted pending receipt is deliberately never retried automatically.
	// It may represent a process interruption after the business write committed.
	if op.Status == "pending" {
		op.Status = "unknown"
	}
	return op, nil
}

type OperationSummary struct {
	ID        string `json:"id"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

func (s *Service) RecentOperations() ([]OperationSummary, error) {
	entries, err := s.operationFiles()
	if errors.Is(err, os.ErrNotExist) {
		return []OperationSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	ops := make([]OperationSummary, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5]
		op, err := s.GetOperation(id)
		if err != nil {
			return nil, err
		}
		// The settings activity list contains metadata, never arguments or receipts.
		ops = append(ops, OperationSummary{ID: op.ID, Tool: op.Tool, Status: op.Status, CreatedAt: op.CreatedAt})
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].CreatedAt > ops[j].CreatedAt })
	if len(ops) > 20 {
		ops = ops[:20]
	}
	return ops, nil
}
func (s *Service) execute(ctx context.Context, id, tool string, input any, fn func(context.Context) (any, error)) (any, error) {
	path, err := s.operationPath(id)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte(tool+"\n"), payload...))
	hash := hex.EncodeToString(digest[:])
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if prior, readErr := s.GetOperation(id); readErr == nil {
		if prior.Tool != tool || prior.Hash != hash {
			return nil, fail("conflict", "operationId was already used with different arguments")
		}
		if prior.Status == "succeeded" {
			return prior, nil
		}
		if prior.Status == "failed" {
			return nil, prior.Error
		}
		return nil, fail("operation_outcome_unknown", "Previous execution may have committed. Inspect current data and the operation receipt before creating a new operation.")
	} else {
		var wire *apierror.WireError
		if !errors.As(readErr, &wire) || wire.Code != "not_found" {
			return nil, readErr
		}
	}
	op := Operation{ID: id, Tool: tool, Hash: hash, Status: "pending", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = s.writePrivate(path, op); err != nil {
		return nil, err
	}
	value, callErr := fn(ctx)
	if callErr != nil {
		op.Error = safeError(callErr)
		op.Status = "failed"
		if op.Error.Code == "internal" {
			op.Status = "unknown"
		}
	} else {
		op.Result, err = json.Marshal(value)
		if err != nil {
			return nil, fail("operation_outcome_unknown", "Write completed but its result could not be saved; inspect current data")
		}
		op.Status = "succeeded"
	}
	if callErr == nil && s.changed != nil {
		s.changed()
	}
	if err = s.writePrivate(path, op); err != nil {
		return nil, fail("operation_outcome_unknown", "Write outcome could not be saved; inspect current data before retrying")
	}
	if callErr != nil {
		return nil, op.Error
	}
	return op, nil
}
