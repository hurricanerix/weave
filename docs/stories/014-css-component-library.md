# Story: CSS component library and design system

## Status
Complete

## Problem
The current web UI has minimal, developer-focused styling. It works functionally but looks unprofessional and generic. Users see browser default fonts, basic colors, no visual polish. The interface does not convey quality or craftsmanship. There is no consistent design language, no dark mode support, and no attention to typography or spacing.

Professional creative tools have cohesive visual design that makes the software feel intentional and trustworthy. Users judge software quality by its appearance. A prototype aesthetic signals "not ready for real work" even when functionality is solid.

The mockup templates in tmp/demo.html show a complete design system with warm colors, proper typography, dark mode support, and polished form controls. This styling needs to be imported into the actual application.

## User/Actor
All users of the Weave web interface who interact with the visual design. This includes novice users forming first impressions and experienced users who spend extended time in the interface and benefit from dark mode and visual consistency.

## Desired Outcome
Users see a polished, professional interface with warm color palette, beautiful typography, consistent spacing, and full dark mode support. Form controls look intentional and well-designed. The interface feels like a finished creative tool, not a prototype. Users working at night or in low-light conditions can enable dark mode and see a comfortable warm color scheme. All interactive elements have appropriate hover states, transitions, and visual feedback.

## Acceptance Criteria
- [ ] CSS custom properties are defined for the complete design system
- [ ] Color palette uses warm, inviting tones (beige/tan base, brown text, gold accents)
- [ ] Dark mode color palette is defined using prefers-color-scheme media query
- [ ] Dark mode uses warm dark tones (not pure black), maintaining color harmony
- [ ] Typography uses system font stack with proper fallbacks
- [ ] MarckScript display font is loaded for the "Weave" branding
- [ ] Font sizes, line heights, and spacing use consistent scale via custom properties
- [ ] Form inputs (text, textarea, select) have consistent styling
- [ ] Range sliders have custom styling matching the design system
- [ ] Checkboxes have custom styling
- [ ] Buttons have variants: default, primary, ghost, danger, sm (small)
- [ ] Icon buttons are styled appropriately
- [ ] All interactive elements have hover states
- [ ] Transitions use consistent timing (150ms fast, 200ms base)
- [ ] Scrollbars are styled to match the design (thin, warm colors)
- [ ] Focus states are visible and accessible
- [ ] Utility classes are available for common patterns
- [ ] All existing UI elements automatically receive new styling
- [ ] Dark mode activates automatically based on system preference
- [ ] No layout breaks when switching between light and dark mode

## Out of Scope
- Custom theme picker (system preference only for MVP)
- Multiple color scheme options beyond light/dark
- CSS animations beyond basic transitions
- Advanced form validation styling
- Tooltip styling (no tooltips in MVP)
- Loading spinner custom styling
- Badge or notification styling
- Card component styling beyond what is needed for messages
- Grid system or layout utilities (flexbox only)

## Dependencies
- Story 013: App shell layout (provides HTML structure to style)

## Open Questions
None.

## Notes
The CSS in tmp/demo.html is complete and production-ready. This story involves importing that CSS into the project, with minimal modifications:

1. Extract all CSS from tmp/demo.html
2. Place in internal/web/templates/index.html style block (or separate CSS file if preferred)
3. Ensure MarckScript font file (MarckScript-Regular.ttf) is copied to web assets
4. Verify all selectors match the actual HTML structure from story 013
5. Test in both light and dark mode

The CSS includes:
- CSS custom properties (root variables)
- Dark mode overrides using prefers-color-scheme
- Reset and base styles
- Form element styling
- Button variants
- Layout utilities
- Scrollbar styling

Color palette (light mode):
- Background: warm beige/cream tones
- Text: brown/tan tones
- Accent: gold/tan
- Borders: light brown

Color palette (dark mode):
- Background: warm dark brown/black tones
- Text: light beige/cream
- Accent: lighter gold
- Borders: dark brown

The design system is intentionally warm and inviting, avoiding stark whites or pure blacks. This creates a comfortable environment for creative work.

All form controls use the custom properties, so changing color scheme in the future only requires updating the root variables.

## Tasks

### 001: Copy MarckScript font to static directory
**Domain:** weave
**Status:** done
**Depends on:** none

Copy MarckScript-Regular.ttf from /home/hurricanerix/workspace/projects/weave/tmp/ to /home/hurricanerix/workspace/projects/weave/internal/web/static/fonts/. The font is already available and ready to use. Verify the file is copied correctly (81816 bytes). The web server already serves files from internal/web/static/ via the /static/ route using Go embed.

---

### 002: Extract and integrate CSS into template
**Domain:** weave
**Status:** pending
**Depends on:** 001

Extract the complete CSS from /home/hurricanerix/workspace/projects/weave/tmp/index.html (lines 8-1426, inside the style tag). Update the @font-face url path from 'MarckScript-Regular.ttf' to '/static/fonts/MarckScript-Regular.ttf'. Place the CSS in the style block of /home/hurricanerix/workspace/projects/weave/internal/web/templates/index.html, replacing the existing basic styles. The CSS is production-ready and requires no modifications beyond the font path change.

---

### 003: Verify CSS selectors match HTML structure
**Domain:** weave
**Status:** pending
**Depends on:** 002

Compare the CSS class names in the imported stylesheet against the actual HTML structure in /home/hurricanerix/workspace/projects/weave/internal/web/templates/index.html from story 013. Check that layout classes (.app, .app-header, .app-body, .main, .sidebar, .chat-panel, .image-panel, .settings-panel) match the HTML structure. Check that form classes (.form-group, .form-label, .form-input, .form-select, .form-range, .form-check) match existing form elements. Verify button classes (.btn, .btn--primary, .icon-btn) are applied correctly. If mismatches are found, either update HTML class names to match CSS, or update CSS selectors to match HTML. Document any changes needed.

---

### 004: Test visual appearance in light mode
**Domain:** weave
**Status:** pending
**Depends on:** 003

Run the web server and verify the interface displays correctly in light mode. Check that colors are warm beige/tan tones (not stark white). Verify typography uses system font stack. Verify MarckScript font loads for "Weave" branding in header. Check that all form elements (inputs, selects, ranges, checkboxes) are styled consistently. Verify button variants render correctly. Check that hover states work on interactive elements. Verify scrollbars are styled (thin, warm colors). Check that focus states are visible. Test in both Firefox and Chrome.

---

### 005: Test visual appearance in dark mode
**Domain:** weave
**Status:** pending
**Depends on:** 004

Change system preference to dark mode and verify the interface switches automatically. Check that dark mode uses warm dark brown/black tones (not pure black). Verify text is readable with sufficient contrast. Check that all interactive elements remain visible and have appropriate hover states. Verify accent colors remain distinguishable. Test that no layout shifts occur when switching between light and dark modes. Verify scrollbars adjust to dark theme. Test in both Firefox and Chrome with dark mode system preference enabled.

---

### 006: Verify existing functionality unchanged
**Domain:** weave
**Status:** pending
**Depends on:** 005

Test all existing features to ensure no regressions. Verify chat input and message display work correctly. Test prompt field editing and auto-save on blur. Verify generation settings (steps, cfg, seed) inputs work. Test "Generate Image" button functionality. Verify SSE events (agent-token, agent-done, prompt-update, image-ready, error) display correctly with new styling. Check that sidebar toggle and settings panel toggle animations work smoothly. Verify image display in the image panel. Confirm no JavaScript errors in browser console.
