//go:build darwin

package secrets

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const keychainService = "com.nestworth.app.provider"

// keychainStore uses macOS's `security` command so the desktop binary does
// not need to carry a second keychain implementation or persist credentials in
// the application database. Arguments are fixed/validated; the secret is
// supplied on stdin for writes rather than placed in the process argument
// list.
type keychainStore struct{}

func newKeychainStore() application.SecretStore { return keychainStore{} }

func (keychainStore) Status(ctx context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "security", "find-generic-password", "-a", keychainAccount, "-s", keychainItem(ref))
	output, err := command.CombinedOutput()
	if err == nil {
		return application.SecretStatusAvailable, nil
	}
	if keychainNotFound(output) {
		return application.SecretStatusMissing, nil
	}
	return application.SecretStatusUnavailable, keychainError(nil, err)
}

func (keychainStore) Get(ctx context.Context, ref application.SecretRef) ([]byte, application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return nil, "", err
	}
	return keychainGet(ctx, ref)
}

func (keychainStore) Put(ctx context.Context, ref application.SecretRef, value []byte) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	if len(bytesTrim(value)) == 0 {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "secret", Message: "secret value is required"}
	}
	// Keep -w last: security treats it as an interactive/stdin prompt when
	// no password argument follows. The secret must not appear in argv.
	command := exec.CommandContext(ctx, "security", "add-generic-password", "-a", keychainAccount, "-s", keychainItem(ref), "-U", "-w")
	command.Stdin = bytes.NewReader(value)
	if output, err := command.CombinedOutput(); err != nil {
		return application.SecretStatusUnavailable, keychainError(output, err)
	}
	return application.SecretStatusAvailable, nil
}

func (keychainStore) Delete(ctx context.Context, ref application.SecretRef) (application.SecretStatus, error) {
	if err := validateSecretRef(ref); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "security", "delete-generic-password", "-a", keychainAccount, "-s", keychainItem(ref))
	output, err := command.CombinedOutput()
	if err == nil || keychainNotFound(output) {
		return application.SecretStatusMissing, nil
	}
	return application.SecretStatusUnavailable, keychainError(output, err)
}

const keychainAccount = "Nestworth"

func keychainItem(ref application.SecretRef) string {
	return keychainService + "." + strings.TrimSpace(ref.Name)
}

func keychainGet(ctx context.Context, ref application.SecretRef) ([]byte, application.SecretStatus, error) {
	command := exec.CommandContext(ctx, "security", "find-generic-password", "-a", keychainAccount, "-s", keychainItem(ref), "-w")
	output, err := command.Output()
	if err == nil {
		value := bytes.TrimSpace(output)
		if len(value) == 0 {
			return nil, application.SecretStatusMissing, nil
		}
		return append([]byte(nil), value...), application.SecretStatusAvailable, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && keychainNotFound(exit.Stderr) {
		return nil, application.SecretStatusMissing, nil
	}
	return nil, application.SecretStatusUnavailable, keychainError(nil, err)
}

func keychainNotFound(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "could not be found") || strings.Contains(message, "item not found") || strings.Contains(message, "seckeychainsearchcopynext")
}

func keychainError(output []byte, err error) error {
	if err == nil {
		return nil
	}
	// Do not return security's output: it can contain account metadata and is
	// not a stable application error contract.
	if len(output) > 0 {
		return &domain.Error{Code: domain.ErrUnavailable, Field: "secret", Message: "macOS Keychain is unavailable"}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &domain.Error{Code: domain.ErrUnavailable, Field: "secret", Message: "macOS Keychain is unavailable"}
}
