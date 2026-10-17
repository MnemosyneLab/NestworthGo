# Investment review browser evidence

`nested-condition-editor.chromium.png` is a 1500×1500 Chromium capture of the
real `ReviewRuleEditor` component with a synthetic nested all/any rule. The
temporary Vite harness mocked only instrument, settings, and mutation hooks; it
used no ledger or Wails backend. It confirms the nested group controls, type
multi-select, metric inputs, timezone, trigger selector, preview area, and save
controls render together.

This browser capture does not establish native macOS Wails layout, VoiceOver,
Chinese IME, theme behavior, or zoom acceptance.
