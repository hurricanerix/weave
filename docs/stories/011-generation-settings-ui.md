# Story: Generation settings UI controls

## Status
Complete

## Problem
Generation settings (steps, CFG, seed) are hardcoded in the application. Users cannot experiment with different settings without modifying code and restarting the server. Power users who understand these parameters have no way to adjust them during a session. This makes iteration slow and prevents users from fine-tuning their results.

## User/Actor
Two types of users:
- **Novice users**: Do not understand technical generation parameters. They will ignore these settings and rely on the agent to make appropriate adjustments based on conversation.
- **Power users**: Understand what steps, CFG, and seed do. They want manual control to experiment and fine-tune results without restarting the application.

## Desired Outcome
Users can adjust generation settings from the web interface. Settings are visible but non-intrusive. Power users can tweak values between generations. Novice users can ignore them and continue using the chat interface normally. The application uses the values from the UI controls instead of hardcoded defaults.

## Acceptance Criteria
- [ ] Three number input controls are visible in the web interface: steps, CFG, and seed
- [ ] Controls are positioned below the prompt input box and above the generate image button
- [ ] Controls are always visible (not hidden or collapsed)
- [ ] Steps input accepts values from 1-100
- [ ] CFG input accepts values from 0-20 (decimals allowed)
- [ ] Seed input accepts any integer, where -1 means "use random seed"
- [ ] When user submits a generation request, the current UI values for steps, CFG, and seed are sent to the backend
- [ ] Backend uses the values from the request instead of hardcoded values
- [ ] On page load, controls default to values specified by CLI flags (--steps, --cfg, --seed)
- [ ] Changing a value in the UI applies to the next generation (no save/apply button needed)
- [ ] Values do not persist across page refreshes (reset to CLI flag defaults)
- [ ] Agent always outputs steps, CFG, and seed values in JSON response
- [ ] Agent sees current setting values in system prompt
- [ ] When agent provides setting values, UI controls update via SSE
- [ ] When agent provides invalid values, they are clamped to valid ranges
- [ ] When values are clamped, feedback message is injected into chat explaining what was clamped

## Out of Scope
- Width and height controls (future story)
- Persisting user preferences across sessions
- Input validation UI feedback (browsers provide native validation for number inputs)
- Tooltips or help text explaining what each setting does
- Advanced controls (samplers, schedulers, etc.)
- Preset configurations or saved settings
- Injecting manual setting changes into chat context (future enhancement)

## Dependencies
None. This can be implemented independently.

## Open Questions
None.

## Notes
This story includes both manual controls and agent integration:
- Tasks 001-008: UI controls and manual adjustment
- Tasks 009-013: Agent control of settings (009=metadata, 010=prompt, 011a/b/c=state+context, 012=SSE, 013=UI handler)

Current implementation has steps and CFG hardcoded in `internal/web/server.go`. The CLI flags exist but are not wired through to the web server handler. Tasks 001-006 fix that wiring. Tasks 009-013 extend the agent's JSON metadata format to include generation settings and wire the full flow from agent response to UI update.

Default values (from CLI flags):
- Steps: 4
- CFG: 1.0
- Seed: -1 (random)

Agent JSON format (after tasks 009-013):
```json
{
  "prompt": "detailed prompt text",
  "ready": true,
  "steps": 20,
  "cfg": 5.0,
  "seed": -1
}
```

## Tasks

### 001: Pass config to web server
**Domain:** weave
**Status:** done
**Depends on:** none

The Config struct already contains Steps, CFG, and Seed fields parsed from CLI flags. However, CreateWebServer in internal/startup/init.go does not pass the config to the web server. The Server struct in internal/web/server.go needs to store these default values.

Add a Config field to the Server struct. Update NewServerWithDeps to accept a config parameter and store Steps, CFG, and Seed. Update CreateWebServer in internal/startup/init.go to pass cfg to NewServerWithDeps. Update all tests that call NewServerWithDeps to pass a test config.

Verification: Run `go test ./internal/web/...` and `go test ./internal/startup/...` to ensure no regressions.

---

### 002: Add generation settings struct for template data
**Domain:** weave
**Status:** done
**Depends on:** 001

The index.html template currently receives `nil` as template data in handleIndex. To populate the input controls with CLI flag defaults, the template needs access to Steps, CFG, and Seed values.

Create a struct type in server.go to hold template data with fields for Steps, CFG, and Seed. Update handleIndex to populate this struct from s.config and pass it to ExecuteTemplate instead of nil.

Verification: Start weave with custom flags like `--steps 10 --cfg 2.5 --seed 42`. Inspect server.go:handleIndex to confirm the struct is populated. No visual changes yet since template does not use the data.

---

### 003: Add input controls to HTML template
**Domain:** weave
**Status:** done
**Depends on:** 002

The template needs three number input controls for steps, CFG, and seed. These should be positioned below the prompt textarea and above the generate button, styled to match the existing UI.

Add a container div with three labeled number inputs in index.html between the prompt-field textarea and the generate-button. Steps input: type=number, min=1, max=100, step=1. CFG input: type=number, min=0, max=20, step=0.1. Seed input: type=number, step=1. Use template variables to set default values from the template data struct. Add CSS styling for the controls to match existing form elements.

Verification: Load the page. Verify three controls are visible below prompt textarea. Verify default values match CLI flags. Verify browser validation prevents out-of-range values.

---

### 004: Send generation settings to backend
**Domain:** weave
**Status:** done
**Depends on:** 003

The generate button uses HTMX to POST to /generate. Currently it only includes the prompt field. It needs to also include the three generation setting inputs.

Update the hx-include attribute on the generate-button in index.html to include the three new input elements by ID or class selector. Give each input a name attribute that matches expected form field names: steps, cfg, seed.

Verification: Open browser dev tools network tab. Click generate. Inspect the POST request to /generate. Verify form data includes steps, cfg, and seed with the values from the inputs.

---

### 005: Parse generation settings in backend handler
**Domain:** weave
**Status:** done
**Depends on:** 001, 004

The handleGenerate function in server.go currently hardcodes steps=20, cfgScale=1.0, and seed=0. It needs to read these values from the form data and validate them.

In handleGenerate, after r.ParseForm, read steps, cfg, and seed from r.FormValue. Parse steps as uint32 (validate 1-100). Parse cfg as float32 (validate 0-20). Parse seed as int64. If seed is -1, convert to 0 (random) for the protocol. If any values are missing or invalid, fall back to server defaults from s.config. Use the parsed values instead of hardcoded values when calling protocol.NewSD35GenerateRequest.

Verification: Run weave. Set steps=50, cfg=7.5, seed=12345 in UI. Generate image. Check server logs to confirm values are parsed. Set steps=999 (out of range). Generate image. Verify it falls back to default or returns validation error.

---

### 006: Handle seed=-1 for random generation
**Domain:** weave
**Status:** done
**Depends on:** 005

The acceptance criteria specify seed=-1 means "use random seed". The protocol expects seed=0 for random. The UI needs to display -1 as the default and convert it to 0 when sending to the daemon.

Update handleGenerate to check if the parsed seed value is -1. If so, convert it to 0 before passing to protocol.NewSD35GenerateRequest. Update the template default value for seed input to use -1 instead of 0 if that matches CLI flag convention.

Verification: Set seed=-1 in UI. Generate image. Check that different images are produced each time (random seed working). Set seed=42. Generate twice. Verify identical images (deterministic seed working).

---

### 007: Update CLI flag default for seed to -1
**Domain:** weave
**Status:** done
**Depends on:** none

The config package currently defaults seed to 0. The story specifies seed=-1 should mean random. For consistency with the UI, the CLI flag default should be -1.

Update defaultSeed constant in internal/config/config.go from 0 to -1. Update validation logic to allow -1 as a valid seed value (currently minSeed is 0). Update help text to clarify -1 means random.

Verification: Run `weave --help`. Verify seed default shown as -1. Run `weave --seed -1`. Verify no validation error. Check that seed=-1 is properly stored in config.

---

### 008: Update config validation to allow seed=-1
**Domain:** weave
**Status:** done
**Depends on:** 007

The config validation currently rejects negative seed values. It needs to allow -1 as a special value meaning random.

Update validate() in internal/config/config.go. Change seed validation from `c.Seed < minSeed` to `c.Seed < -1`. Update ErrInvalidSeed message to indicate -1 is allowed for random.

Verification: Run config package tests. Verify seed=-1 passes validation. Verify seed=-2 fails validation. Run `go test ./internal/config/...`.

---

### 009: Add steps, cfg, seed fields to LLMMetadata
**Domain:** weave
**Status:** done
**Depends on:** none

The LLMMetadata struct in internal/ollama/types.go currently only contains prompt and ready fields. The agent needs to specify generation settings in every response so they can be validated and applied.

Add three new fields to LLMMetadata: Steps (int), CFG (float64), and Seed (int64). These fields are **required** in every agent response - add them to the field presence check in parseResponse alongside "prompt" and "ready". If any field is missing, return ErrMissingFields. The fields may have any numeric value (clamping happens in the web layer). Zero values are valid, so we must validate presence to distinguish "field missing" from "agent wants zero value".

Update existing tests to include all five fields. Add new test cases:
- Response missing steps field returns ErrMissingFields
- Response missing cfg field returns ErrMissingFields
- Response missing seed field returns ErrMissingFields
- Response with all fields but invalid values (e.g., steps=-999) parses successfully (clamping is web layer's job)

Verification: Run `go test ./internal/ollama/...`. Verify responses without the new fields fail validation. Verify responses with all fields (even extreme values) parse successfully.

---

### 010: Update agent system prompt with generation settings
**Domain:** weave
**Status:** done
**Depends on:** 009

The SystemPrompt constant in internal/ollama/types.go needs to explain the new fields and their valid ranges. The agent must understand what these settings do and what values are acceptable.

Update SystemPrompt to add documentation for steps, cfg, and seed fields in the JSON metadata section:
- steps (1-100): Controls quality/speed tradeoff. Higher values produce more detailed images but take longer. Default is 4 for fast iteration. Use 20-30 for quality previews.
- cfg (0-20): Controls how strictly the image follows the prompt. Higher values = stricter adherence but potentially less creative. Default is 1.0. Use 3-7 for balanced results.
- seed (-1 for random, 0+ for deterministic): Use -1 when exploring variations. Use a specific seed when the user wants to reproduce or iterate on a specific result.

Add guidance on when to change values:
- Be conservative by default - only change settings when the user explicitly asks or clearly implies they want different quality/speed tradeoffs
- If user says "more detailed" or "higher quality", increase steps
- If user says "faster" or "quick preview", decrease steps
- If results don't match the prompt well, suggest increasing cfg
- Keep seed at -1 unless user wants reproducibility

Provide examples showing the correct format with all five fields. Add a note that invalid values will be clamped and the agent will receive a feedback message explaining what was adjusted.

Verification: Manually review the updated prompt. Verify it clearly explains valid ranges and when to change values. Verify examples include all five fields. Run existing tests to ensure no regressions.

---

### 011a: Add generation settings to Session state
**Domain:** weave
**Status:** done
**Depends on:** none

The conversation Manager is for conversation history, not session-wide state. Generation settings should be tracked by the Session struct instead.

Add a GenerationSettings struct to internal/conversation/types.go with fields: Steps (int), CFG (float64), and Seed (int64). Update Session in internal/conversation/session.go to store current settings as a pointer field (nil means "not yet set", use server defaults). Add methods to Session:
- SetGenerationSettings(steps int, cfg float64, seed int64)
- GetGenerationSettings() (steps int, cfg float64, seed int64, hasSettings bool)

When hasSettings is false, the caller should use server defaults.

Verification: Add unit tests for Session.SetGenerationSettings and GetGenerationSettings. Verify settings can be stored and retrieved per session. Verify new sessions return hasSettings=false.

---

### 011b: Inject current settings into LLM context
**Domain:** weave
**Status:** done
**Depends on:** 010, 011a

The agent needs to see current UI values when generating responses. Modify BuildLLMContext to accept and inject current generation settings.

Update BuildLLMContext signature to accept optional settings: `func (m *Manager) BuildLLMContext(systemPrompt string, currentSteps int, currentCFG float64, currentSeed int64) []Message`. After the system prompt message, inject an additional system message: `[Current generation settings: steps=X, cfg=Y, seed=Z]`. This appears before conversation history so the agent sees it as context.

If all settings are zero (not yet set), skip injecting the settings message.

Verification: Add unit tests for BuildLLMContext with and without settings. Verify the settings message appears after system prompt but before conversation history. Verify zero values skip injection.

---

### 011c: Wire settings from UI through /chat handler
**Domain:** weave
**Status:** done
**Depends on:** 011a, 011b

The /chat endpoint currently doesn't receive settings values from the UI, so handleChat doesn't know current settings. Extend the UI to send current settings with chat messages.

Update index.html: Change the chat form's hx-include attribute to also include the settings inputs: `hx-include="#chat-input, #steps-input, #cfg-input, #seed-input"`. The chat form already has hx-include for the chat input.

Update handleChat in internal/web/server.go:
1. Parse steps, cfg, seed from form data using existing parseSteps, parseCFG, parseSeed functions
2. Get the session from sessionManager
3. Call session.SetGenerationSettings with parsed values
4. When calling manager.BuildLLMContext, pass current settings from the session

Verification: Start weave. Open browser dev tools network tab. Send a chat message. Verify POST to /chat includes steps, cfg, seed form fields. Check server logs to confirm settings are parsed and available for LLM context injection.

---

### 012: Add settings-update SSE event type
**Domain:** weave
**Status:** done
**Depends on:** 009, 011c

The UI needs to receive agent-provided setting values via SSE and update the input controls. A new SSE event type is required to push settings changes from backend to frontend.

Add EventSettingsUpdate constant to internal/web/sse.go (where other event constants are defined). Update handleChat in internal/web/server.go to extract steps, cfg, and seed from result.Metadata after successful agent response.

Implement clamping with feedback:
1. Clamp values to valid ranges: steps 1-100, cfg 0-20, seed >= -1
2. Track which values were clamped and by how much
3. If any values were clamped, construct a feedback message

Feedback message format examples:
- "Settings adjusted: steps 150→100 (maximum is 100)"
- "Settings adjusted: cfg -2.0→0.0 (minimum is 0)"
- "Settings adjusted: seed -5→-1 (minimum is -1)"
- "Settings adjusted: steps 150→100 (max 100), cfg 25.0→20.0 (max 20)"

Send settings-update event with JSON payload: `{"steps": X, "cfg": Y, "seed": Z}`. If values were clamped, send an additional agent-token event with the feedback message so it appears in the chat stream.

Note: Using agent-token event (not error event) for clamping feedback so it appears as informational text in the chat rather than a red error message.

Verification: Run `go test ./internal/web/...` to ensure no regressions. Add unit tests for clamping logic. Manually test by having agent return out-of-range values and verify clamping occurs and feedback appears in chat.

---

### 013: Handle settings-update events in UI
**Domain:** weave
**Status:** done
**Depends on:** 012

The frontend needs to handle settings-update SSE events and update the three input controls accordingly. This allows the agent to control generation settings conversationally.

Update index.html JavaScript:

1. Add focus tracking state for each input:
```javascript
let isStepsFocused = false;
let isCfgFocused = false;
let isSeedFocused = false;
```

2. Add focus/blur event listeners for each settings input:
```javascript
document.getElementById('steps-input').addEventListener('focus', () => isStepsFocused = true);
document.getElementById('steps-input').addEventListener('blur', () => isStepsFocused = false);
// Similar for cfg-input and seed-input
```

3. Add handleSettingsUpdate function:
```javascript
function handleSettingsUpdate(data) {
    if (data.steps !== undefined && !isStepsFocused) {
        document.getElementById('steps-input').value = data.steps;
    }
    if (data.cfg !== undefined && !isCfgFocused) {
        document.getElementById('cfg-input').value = data.cfg;
    }
    if (data.seed !== undefined && !isSeedFocused) {
        document.getElementById('seed-input').value = data.seed;
    }
}
```

4. In the htmx:sseMessage event listener, add case for "settings-update" event type that calls handleSettingsUpdate with parsed JSON data.

Defensive handling: Check that data fields exist before updating. If event data is malformed, log warning and skip update.

Verification: Start weave. Chat with agent and have it suggest settings changes. Verify the input controls update when the agent responds. Click into steps input and hold focus while agent responds - verify steps field is NOT updated while focused. Release focus and verify next update works normally.

---
