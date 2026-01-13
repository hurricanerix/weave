# Story: Settings panel with SD 3.5 parameters

## Status
Complete

## Problem
Generation settings (steps, CFG, seed) currently exist as basic number inputs below the prompt field. They work functionally but lack context, explanation, and visual polish. Users see three inputs with no indication of what they control or how they relate to the generated image. Power users who want to fine-tune generation have no visibility into what prompt the agent generated or what other parameters might be available.

The settings are always visible in the main workspace, taking up valuable space even when users do not need them. Novice users ignore these controls but cannot hide them. Power users want more detailed settings and better organization.

The mockup shows a collapsible right panel dedicated to generation settings. This panel provides space for the resolved prompt (what the agent actually sent to the model), negative prompt, and all SD 3.5 generation parameters in a well-organized form. Settings can be hidden when not needed, and expanded when users want control.

## User/Actor
Two types of users:
- **Power users**: Want detailed control over generation parameters. They understand what steps, CFG, seed, sampler, and dimensions do. They want to see the resolved prompt, adjust settings, and regenerate with modifications.
- **Novice users**: Do not understand generation parameters. They rely on the agent to make good choices. They can collapse the settings panel and focus on the conversation and resulting image.

## Desired Outcome
Users can toggle the settings panel to show or hide generation parameters. When open, the panel displays the resolved prompt (read-only, with copy button), negative prompt (editable), and all SD 3.5 parameters (model, sampler, CFG, steps, seed, dimensions). Power users can read the resolved prompt to understand what the agent generated, adjust parameters, and regenerate. Novice users can close the panel and ignore it entirely. The settings are organized clearly with appropriate labels and grouped logically.

## Acceptance Criteria
- [ ] Settings panel is collapsible via toggle button in app header
- [ ] Panel is collapsed by default on initial page load
- [ ] Panel slides in from the right when opened
- [ ] Panel slides out to the right when closed
- [ ] Panel header shows "Image Details" title
- [ ] Resolved prompt is displayed in a read-only textarea
- [ ] Resolved prompt textarea uses monospace font for readability
- [ ] Copy button appears in resolved prompt textarea for easy copying
- [ ] Clicking copy button copies resolved prompt to clipboard
- [ ] Parameters section displays SD 3.5 generation settings
- [ ] Model dropdown shows single option "Stable Diffusion 3.5" (display only)
- [ ] CFG scale slider with numeric value display (range 0-20, step 0.1)
- [ ] Steps slider with numeric value display (range 1-100, step 1)
- [ ] Seed input field (number input, allows -1 for random)
- [ ] Width input (number input)
- [ ] Height input (number input)
- [ ] Regenerate button at bottom of settings
- [ ] Copy Settings button to copy all parameters as text
- [ ] All form controls use consistent styling from design system
- [ ] Settings panel is scrollable when content overflows
- [ ] When agent sends settings-update SSE event, form controls update
- [ ] When user manually changes settings, values persist until next SSE update
- [ ] Form controls have appropriate hints (e.g., "Use -1 for random" on seed)

## Out of Scope
- Advanced parameters (scheduler, attention, LoRA) - future story
- Preset configurations or saved settings - future enhancement
- Settings history or undo - not planned for MVP
- Negative prompt suggestions or templates - future enhancement
- Parameter tooltips or help modals - future story
- Regenerate with variations - future enhancement
- Batch generation settings - separate major feature
- Image-to-image settings - separate major feature
- Resizable settings panel width - nice to have, not MVP
- Collapsible sections within settings panel - keep flat for MVP

## Dependencies
- Story 013: App shell layout (provides settings panel structure and toggle)
- Story 014: CSS component library (provides form styling)
- Story 011: Generation settings UI controls (existing settings logic)
- Story 012: Agent-triggered generation (settings-update SSE event)

## Open Questions

RESOLVED:

1. Model dropdown: Include single option "Stable Diffusion 3.5" for display clarity. Not for selection, just to show users what model is being used.

2. Sampler dropdown: Omit entirely. Not needed for MVP.

3. Dimensions: Use number inputs for width and height. Backend validates range (64-2048, multiple of 64). No frontend validation beyond number input type.

4. Negative prompt: Deferred to later story. Not included in this panel.

## Notes
This story moves the existing generation settings from below the prompt into the new settings panel. The backend logic for handling these settings already exists from story 011. This story reorganizes the UI and adds new display elements (resolved prompt, negative prompt, model/sampler selectors).

Current settings (from story 011):
- Steps (1-100)
- CFG (0-20)
- Seed (-1 for random, or specific integer)

New settings to add:
- Model selector (dropdown with SD 3.5 variants)
- Sampler selector (dropdown with compatible samplers)
- Width (integer input or dropdown)
- Height (integer input or dropdown)
- Negative prompt (textarea)

Resolved prompt:
The agent generates a detailed prompt from the user's request (e.g., "a dragon" becomes "a majestic dragon with detailed scales, dramatic lighting, 8k"). This resolved prompt is what actually gets sent to the image model. Users need to see this to understand what the model received and to learn prompt engineering.

The resolved prompt is read-only because it represents what was used for the current image. If users want to modify it, they should chat with Ara or manually adjust the negative prompt and parameters.

Copy buttons use the Clipboard API:
```javascript
async function copyText(elementId) {
  const element = document.getElementById(elementId);
  await navigator.clipboard.writeText(element.value);
  // Optional: Show brief "Copied!" feedback
}
```

Copy Settings button formats all parameters as text:
```
Model: Stable Diffusion 3.5
CFG: 7.5
Steps: 28
Seed: 3847291056
Dimensions: 768x768
```

The settings panel width is fixed at 360px (from design system) with min-width 280px. On smaller screens (<1024px), the panel overlays the content as a slide-out drawer instead of pushing content.

Regenerate button functionality:
When clicked, read all current form values, send them to /generate endpoint (existing from story 011), and trigger new generation using current settings. This allows power users to tweak parameters and regenerate without chatting with Ara.

The backend needs to expose the resolved prompt via SSE events. This may require extending the existing SSE event types or adding fields to the settings-update or prompt-update event.

## Tasks

### 001: Add settings panel HTML structure to template
**Domain:** weave
**Status:** done
**Depends on:** none

Add settings panel HTML to `/internal/web/templates/index.html` using structure from `tmp/demo.html` lines 1578-1670. Panel should include: header with "Image Details" title, scrollable content area with resolved prompt section (read-only textarea with copy button), Parameters section (Model dropdown with single option "Stable Diffusion 3.5", CFG slider 0-20 step 0.1, Steps slider 1-100 step 1, Seed number input with hint "Use -1 for random", Width and Height number inputs), and buttons (Regenerate primary button, Copy Settings secondary button). Use existing CSS classes from `tmp/index.html`. Panel starts with `open` class removed (collapsed by default).

---

### 002: Add settings toggle button to app header
**Domain:** weave
**Status:** done
**Depends on:** 001

Add icon button to app header in `/internal/web/templates/index.html` for toggling settings panel visibility. Button should use `icon-btn` class and contain SVG icon matching the structure from `tmp/demo.html` line 1326-1331. Button calls `toggleSettings()` JavaScript function on click. Position after title, before any right-aligned elements.

---

### 003: Implement settings panel toggle JavaScript
**Domain:** weave
**Status:** done
**Depends on:** 001, 002

Add `toggleSettings()` function to `/internal/web/templates/index.html` script section. Function toggles `open` class on settings panel element with id `settings-panel`. When opening, add `open` class to slide panel in from right. When closing, remove `open` class to slide panel out. Reference implementation in `tmp/demo.html` lines 1759-1773.

---

### 004: Wire up resolved prompt copy button
**Domain:** weave
**Status:** done
**Depends on:** 001

Add `copyResolvedPrompt()` JavaScript function to copy resolved prompt textarea content to clipboard using `navigator.clipboard.writeText()`. Wire up copy button in resolved prompt section to call this function on click. Reference implementation in `tmp/demo.html` lines 1776-1779. Function should handle cases where clipboard API is unavailable.

---

### 005: Wire up CFG and Steps sliders to display value
**Domain:** weave
**Status:** done
**Depends on:** 001

Add JavaScript to synchronize CFG and Steps range sliders with their displayed numeric values. When slider moves, update corresponding `form-range-value` span. Reference existing implementation in `/internal/web/templates/index.html` lines 1826-1836. Apply same pattern to new sliders in settings panel.

---

### 006: Add backend support for width and height parameters
**Domain:** weave
**Status:** done
**Depends on:** none

Modify `/internal/web/server.go` to parse width and height from form data in `handleGenerate`. Add `parseWidth()` and `parseHeight()` helper functions similar to existing `parseSteps()`, `parseCFG()`, `parseSeed()`. Valid range is 64-2048, must be multiple of 64. Default to 768x768 (current hardcoded value in `generateImage()`). Pass parsed values to `generateImage()` call instead of hardcoded `width, height := uint32(768), uint32(768)` at line 915.

---

### 007: Extend generateImage to accept width and height parameters
**Domain:** weave
**Status:** done
**Depends on:** 006

Modify `generateImage()` function signature in `/internal/web/server.go` to accept width and height parameters. Replace hardcoded `width, height := uint32(768), uint32(768)` at line 915 with function parameters. Update all callers (`handleGenerate` and agent-triggered generation in `handleChat`) to pass dimensions. Default to 768x768 when called from agent without explicit dimensions.

---

### 008: Add resolved-prompt SSE event type
**Domain:** weave
**Status:** done
**Depends on:** none

Add `EventResolvedPrompt` constant to `/internal/web/sse.go` event type list. Backend will send this event when agent provides a prompt, separate from `EventPromptUpdate` which updates the editable prompt field. Resolved prompt is read-only and shows what was actually sent to the model.

---

### 009: Send resolved prompt in SSE events
**Domain:** weave
**Status:** done
**Depends on:** 008

Modify `/internal/web/server.go` `handleChat()` to send `EventResolvedPrompt` when prompt is extracted from agent metadata (around line 402). Send event with map containing `prompt` key. This populates the read-only resolved prompt textarea in settings panel. Send event before or alongside `EventPromptUpdate`.

---

### 010: Add SSE handler for resolved-prompt event
**Domain:** weave
**Status:** done
**Depends on:** 001, 008, 009

Add `handleResolvedPrompt()` JavaScript function to `/internal/web/templates/index.html` to handle `resolved-prompt` SSE event. Function should update resolved prompt textarea value with received prompt string. Add case to SSE message handler switch statement (around line 561) to call `handleResolvedPrompt(data)` when `eventType === 'resolved-prompt'`. Add hidden SSE swap target for resolved-prompt event similar to existing targets (around line 337-357).

---

### 011: Implement Regenerate button functionality
**Domain:** weave
**Status:** done
**Depends on:** 001, 005, 006, 007

Wire up Regenerate button in settings panel to trigger image generation with current form values. Add `hx-post="/generate"` attribute and `hx-include` for all form inputs (resolved-prompt, cfg, steps, seed, width, height). Button should use existing generate logic but with settings panel values instead of separate prompt field. Reference existing generate button implementation in `/internal/web/templates/index.html` lines 436-440.

---

### 012: Implement Copy Settings button functionality
**Domain:** weave
**Status:** done
**Depends on:** 001

Add `copySettings()` JavaScript function to format current parameter values as text and copy to clipboard. Format: "Model: Stable Diffusion 3.5\nCFG: {value}\nSteps: {value}\nSeed: {value}\nDimensions: {width}x{height}". Read values from form inputs and construct string. Use `navigator.clipboard.writeText()` to copy. Wire button to call function on click. Handle clipboard API unavailable case.

---

### 013: Extend settings-update SSE to include width and height
**Domain:** weave
**Status:** done
**Depends on:** 006, 007

Modify `/internal/web/server.go` `handleChat()` to include width and height in `EventSettingsUpdate` payload (around line 420). Backend currently sends steps, cfg, seed. Add width and height fields to map. Default to 768x768 when agent doesn't specify. This allows agent to suggest dimensions via chat.

---

### 014: Update SSE handler for width and height settings
**Domain:** weave
**Status:** done
**Depends on:** 001, 013

Extend `handleSettingsUpdate()` JavaScript function in `/internal/web/templates/index.html` (around line 647) to update width and height inputs when SSE event includes those fields. Add checks for `data.width` and `data.height` and update corresponding input values if present. Follow existing pattern for steps, cfg, seed updates.

---

### 015: Update handleGenerate to include width and height in form
**Domain:** weave
**Status:** done
**Depends on:** 001, 006, 007

Modify existing generate button `hx-include` in `/internal/web/templates/index.html` (line 438) to include width and height inputs from settings panel. Currently includes `#prompt-field, #steps-input, #cfg-input, #seed-input`. Add `#width-input, #height-input` to include list. This ensures manual generation uses dimensions from settings panel.

---
