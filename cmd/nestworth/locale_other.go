//go:build !darwin || !cgo || server

package main

func systemLocale() string {
	return environmentLocale()
}
