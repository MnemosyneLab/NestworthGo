//go:build !darwin && !linux

package mobileapi

// Do not bind remembered authorization to a replaceable path on unverified OSes.
func databaseFileIdentity(string) (string, error) { return "", errSessionRegistry }

// Preserve the existing memory-only configuration path; durable remembering is disabled.
func syncDirectory(string) error { return nil }
