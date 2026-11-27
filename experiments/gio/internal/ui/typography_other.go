//go:build !darwin

package ui

import "gioui.org/font"

func systemTypeface(base font.Typeface) font.Typeface { return base }

func supplementalSystemFaces() []font.FontFace { return nil }
