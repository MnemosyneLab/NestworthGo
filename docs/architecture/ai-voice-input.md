# AI voice input

The desktop assistant can record a question and append its transcript to the
existing editable draft. It never submits the question or calls financial tools.
Existing draft text (including edits during transcription) is preserved. If the
combined draft exceeds the existing 16 KiB UTF-8 limit, the shared input displays
its validation error and sending stays disabled until the user edits it.

## Provider and privacy

The host uses the existing enabled `openai-responses` provider and its credential
port. There is no second API key, frontend credential, endpoint selector or
credential migration. The current provider uses the fixed OpenAI API origin.
`POST https://api.openai.com/v1/audio/transcriptions` receives a multipart `file`
and `model=gpt-transcribe`; JSON `text` becomes the draft. No page, ledger,
conversation history or prompt is included. Account/model entitlement is only
known when a request is made; configuration presence does not establish access.
Provider failures do not trigger retries or a fallback to a different model.

Official documentation checked 2026-11-06:
- [File transcription](https://developers.openai.com/api/docs/guides/speech-to-text)
- [Current transcription models](https://developers.openai.com/api/docs/guides/transcription)

The current guide recommends `gpt-transcribe` for completed recordings. The old
Whisper/GPT-4o transcription models are deprecated; this feature does not select
them or reuse the conversation model ID.

Before recording, the control explains that stopping sends audio to OpenAI using
existing AI configuration. Audio is not transcribed entirely locally. Nestworth
keeps recordings in memory only, excludes them from conversation persistence,
never logs audio/key/provider response bodies, and releases buffers on completion
or cancellation. Provider processing and retention follow the user's OpenAI
account terms; this is not a promise of provider-side deletion. Cancel aborts an
in-flight request and discards late results; it cannot retract audio already sent.

## Bounds and lifetime

- Record 1–120 seconds; automatic stop at 120 seconds. Maximum audio size is
  8 MiB (below the API's 25 MB limit), checked during collection and by the host.
- Prefer WebM/Opus, then MP4 (WebKit), then WebM. Unsupported clients show an
  explanation. Only audio is requested; camera access is never requested.
- Permission wait is bounded to 30 seconds. The provider call has a 60-second
  timeout and 64 KiB response bound; the UI has a 65-second completion watchdog.
- Stop releases media tracks before upload. Cancel, hidden document, page unload,
  closed sidebar (including a retained hidden desktop dock), conversation switch,
  epoch/configuration replacement, disabled/offline state and unmount dispose the
  recording. Late permission grants release their tracks without uploading.
- A single transcription occupies an in-memory runtime cancellation slot. Wails
  cancellation, shutdown, restore, provider disable and credential removal cancel
  it. Credential replacement remains blocked during active calls. Epoch/revision
  are checked before admission and before returning text. No recording is replayed
  after restart or a failure.

Duration metadata is a UI guard, not a media decoder assertion. The backend's
hard byte/response/time bounds apply even to malformed IPC media. The provider
validates the audio container. Empty/invalid/oversized transcripts fail closed.

## Platform permissions

macOS production and development Info.plists include
`NSMicrophoneUsageDescription`; the dev identity stays `com.nestworth.dev`.
Wails 3.1.0's `requestMediaCapturePermissionForOrigin` implementation maps default
microphone permission to WebKit's prompt. Nestworth does not pre-grant it or alter
system settings. Existing packaging is ad-hoc signed, not App Sandbox packaging;
a future sandbox/hardened-runtime distribution must separately validate the
appropriate audio-input entitlement and its signing flow.

Windows WebView2 and Linux WebKit desktops use the same capability-detected
MediaRecorder path and their existing default Wails permission handling. Their
OS/browser microphone settings must permit recording. macOS/Windows/Linux native
acceptance remains separate from jsdom tests. Mobile and remote HTTP clients
currently have no model runtime binding, so they gain no voice route or broader
ledger permission. If model support later reaches a web client, HTTPS/secure
context and server authorization must be established first.

## Acceptance

Use only synthetic audio, keys, databases and settings in automated tests.
Provider tests use a fake RoundTripper; recording tests use fake media devices.
Native manual gates, using the established isolated development identity:

1. Allow/deny/revoke microphone access; test WebKit MP4 recording and short speech.
2. Stop, cancel during permission/recording/transcription, close the dock/sheet,
   switch conversations and disable AI; verify the microphone indicator goes out
   and no stale text appears.
3. Test 1-second and 120-second bounds, unplugged mic, offline/timeout, missing
   model entitlement, empty speech, Unicode and mixed Chinese/English text.
4. Keep an existing draft, edit it while transcription runs, then verify the
   transcript is appended, focused, editable and never sent automatically.
5. Verify the dev and release package contain the microphone usage declaration.

Do not treat Linux tests as native macOS acceptance or use real ledger/key
material to satisfy this checklist.
