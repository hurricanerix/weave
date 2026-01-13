# Story: Image panel with overlay actions

## Status
Complete

## Problem
The current image display is basic and lacks polish. Generated images appear in the UI but without any controls or context. Users cannot easily save the image or open it in a new tab. The image display does not feel like a focused, intentional presentation. There is no visual hierarchy or emphasis on the generated artwork.

Professional creative tools treat the generated image as the primary artifact. The image should be displayed prominently, with appropriate spacing, shadows, and presentation. Users should have immediate access to common actions like saving or viewing the image full-screen.

When no image has been generated yet, the panel is empty without any indication of what will appear there. Users starting a new session see a blank space with no context.

## User/Actor
Users viewing and managing generated images. After requesting an image, they want to see it displayed beautifully and access it easily. Power users want quick access to save or open in new tab without navigating menus.

## Desired Outcome
Users see their generated image displayed prominently in the center panel with professional presentation. The image has appropriate spacing, shadows, and visual polish. When they hover over the image, action buttons appear as an overlay (open in tab, save). They can click these buttons to perform quick actions without interrupting their workflow. When no image exists yet, they see a helpful empty state that explains what will appear in this space.

## Acceptance Criteria
- [ ] Generated image is displayed centered in the image panel
- [ ] Image is scaled to fit panel while maintaining aspect ratio
- [ ] Image has maximum dimensions to prevent excessive scaling on large screens
- [ ] Image has subtle shadow for depth and visual separation
- [ ] Image background color matches design system (warm image-bg color)
- [ ] When user hovers over the image, action buttons appear as overlay
- [ ] Overlay contains two icon buttons: "Open in new tab" and "Save"
- [ ] Overlay buttons are positioned in the top-right corner of the image
- [ ] Overlay buttons have semi-transparent background for visibility
- [ ] Clicking "Open in new tab" button opens the image URL in a new browser tab
- [ ] Clicking "Save" button triggers browser download of the image
- [ ] Overlay disappears when mouse leaves the image area
- [ ] Overlay transitions smoothly (fade in/out)
- [ ] When no image exists, empty state is displayed
- [ ] Empty state shows centered icon and helpful text
- [ ] Empty state text explains that images will appear here after generation
- [ ] Empty state styling matches design system (muted, non-intrusive)
- [ ] Existing image generation via SSE continues to work
- [ ] Image updates when new image-ready SSE event is received
- [ ] Image URL from SSE event is correctly set as img src
- [ ] Alt text is set appropriately on the image element

## Out of Scope
- Image zoom or pan controls - future enhancement
- Image comparison slider - not planned for MVP
- Image editing tools - separate major feature
- Thumbnail gallery of previous images - future story
- Image metadata display (dimensions, file size) - separate story (settings panel)
- Drag-and-drop to save - nice to have enhancement
- Keyboard shortcuts for actions - future enhancement
- Image loading spinner - basic browser loading only
- Progressive image loading - future optimization

## Dependencies
- Story 013: App shell layout (provides image panel structure)
- Story 014: CSS component library (provides image styling)
- Story 009: Image encoding (backend sends images)
- Story 012: Agent-triggered generation (image-ready SSE event)

## Open Questions
None.

## Notes
This story enhances the visual presentation and interaction with the generated image. The underlying image generation and delivery via SSE already works from stories 009 and 012.

Current image handling (from story 009/012):
- Backend generates image and serves it at a URL
- Backend sends image-ready SSE event with image URL
- Frontend receives event and updates img src
- Image displays in a basic img tag

This story adds:
1. Image container with proper styling and centering
2. Overlay actions that appear on hover
3. Click handlers for open/save actions
4. Empty state for initial page load

Empty state structure:
```html
<div class="empty-state">
  <svg class="empty-state-icon">...</svg>
  <div class="empty-state-text">No image yet</div>
  <div class="empty-state-hint">Generated images will appear here</div>
</div>
```

Image with overlay structure:
```html
<div class="image-container">
  <img class="image-main" src="/path/to/image.png" alt="Generated image">
  <div class="image-actions-overlay">
    <button class="image-action-btn" aria-label="Open in new tab">
      <svg>...</svg>
    </button>
    <button class="image-action-btn" aria-label="Save">
      <svg>...</svg>
    </button>
  </div>
</div>
```

The CSS for these elements comes from story 014. This story implements the HTML structure and JavaScript for overlay visibility and button actions.

"Open in new tab" implementation:
```javascript
function openImageInNewTab() {
  const img = document.querySelector('.image-main');
  if (img && img.src) {
    window.open(img.src, '_blank');
  }
}
```

"Save" implementation:
```javascript
function saveImage() {
  const img = document.querySelector('.image-main');
  if (img && img.src) {
    const a = document.createElement('a');
    a.href = img.src;
    a.download = 'weave-image.png'; // or extract filename from URL
    a.click();
  }
}
```

The image panel background uses var(--color-image-bg) from the design system to create appropriate contrast in both light and dark modes.

## Tasks

### 001: Replace image display with empty state structure
**Domain:** weave
**Status:** done
**Depends on:** none

Replace the current image display in index.html (current-image div) with the empty state structure. The empty state should be the default view when no image has been generated. Use the structure from demo.html: .empty-state container with SVG icon, text, and hint. The SVG should be a simple image icon (mountain/sun graphic). Keep the current image-container wrapper but change its contents to show the empty state initially.

---

### 002: Add JavaScript function to toggle between empty state and image display
**Domain:** weave
**Status:** pending
**Depends on:** 001

Create JavaScript helper function showImageWithOverlay(url, alt) that replaces the empty state with the image and overlay structure. This function should create the .image-container structure with .image-main img element and .image-actions-overlay div containing the two action buttons. The existing handleImageReady SSE handler should call this function instead of directly manipulating the DOM.

---

### 003: Implement openImageInNewTab action
**Domain:** weave
**Status:** pending
**Depends on:** 002

Add openImageInNewTab() JavaScript function that finds the .image-main img element and opens its src in a new browser tab using window.open(img.src, '_blank'). Wire this function to the "Open in new tab" button's onclick handler. Include null checks for safety. The button should have the external link SVG icon from demo.html.

---

### 004: Implement saveImage download action
**Domain:** weave
**Status:** pending
**Depends on:** 002

Add saveImage() JavaScript function that creates a temporary anchor element, sets its href to the image src, sets download attribute to 'weave-image.png', and programmatically clicks it to trigger download. Wire this function to the "Save" button's onclick handler. Include null checks for safety. The button should have the download arrow SVG icon from demo.html.

---

### 005: Update handleImageReady to use new display functions
**Domain:** weave
**Status:** pending
**Depends on:** 002, 003, 004

Modify the existing handleImageReady SSE event handler to call showImageWithOverlay() instead of directly setting innerHTML. Remove the current image display logic that creates a simple img tag. The handler should pass the image URL from the SSE event data and use "Generated image" as the alt text. This ensures the overlay actions are wired up when images are displayed.

---
