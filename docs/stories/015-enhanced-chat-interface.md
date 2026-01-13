# Story: Enhanced chat interface with message bubbles

## Status
Complete

## Problem
The current chat interface displays messages in a basic list format without visual distinction between user and assistant messages. Messages lack hierarchy, personality, and polish. Users see plain text in a scrolling div with no visual cues about who said what or when.

Users expect chat interfaces to have message bubbles that clearly distinguish between their messages and the assistant's responses. Modern chat applications use bubbles, avatars, sender names, and thoughtful spacing to create a conversational feel. The current implementation feels like a log file, not a conversation.

The assistant's name "Ara" is not displayed anywhere. Users do not know who they are talking to. The interface lacks personality and feels anonymous.

When images are generated, they appear in a separate panel but are not visually connected to the conversation. Users cannot see which message resulted in which image. The mockup shows thumbnail grids in messages, but for MVP we just need improved message styling and structure.

## User/Actor
Users having conversations with Ara to generate images. They send messages describing what they want, and Ara responds with guidance and prompts. Users need to easily distinguish their messages from Ara's responses and understand the flow of conversation.

## Desired Outcome
Users see a polished chat interface with distinct message bubbles. Their messages appear in one style (aligned right, user color), and Ara's messages appear in another style (aligned left, assistant color, with "Ara" name displayed). The conversation feels natural and easy to follow. Users can quickly scan the history and understand who said what. The interface has the polish and personality of a professional chat application.

## Acceptance Criteria
- [ ] User messages are displayed as right-aligned bubbles
- [ ] User message bubbles have distinct background color (from design system)
- [ ] Assistant messages are displayed as left-aligned bubbles
- [ ] Assistant message bubbles have distinct background color (from design system)
- [ ] Assistant messages display "Ara" as the sender name above the bubble
- [ ] User messages do not display a sender name (implicit "you")
- [ ] Message bubbles have appropriate padding, border radius, and spacing
- [ ] Messages have proper vertical spacing between them (not cramped)
- [ ] Consecutive messages from the same sender are grouped visually
- [ ] Long messages wrap properly within bubbles without breaking layout
- [ ] Code or technical content in messages is readable
- [ ] Chat area scrolls smoothly when new messages arrive
- [ ] Chat auto-scrolls to newest message when assistant responds
- [ ] Existing SSE message handling continues to work
- [ ] Agent streaming (token-by-token) continues to work in new bubble layout
- [ ] System messages (like clamping feedback) are displayed in centered, muted style
- [ ] Empty state shows helpful hint text when no messages exist yet

## Out of Scope
- Message thumbnails (clickable image grids) - nice to have, not MVP
- Progress indicators (determinate/indeterminate) - separate story
- Jump to end button - nice to have, not MVP
- Message timestamps - not needed for MVP
- Message actions (copy, regenerate) - future enhancement
- Auto-resize textarea input - nice to have, not MVP
- "Ara is typing" indicator - future enhancement
- Message reactions or threading - not planned
- File upload or drag-drop for reference images - future story

## Dependencies
- Story 013: App shell layout (provides chat panel structure)
- Story 014: CSS component library (provides bubble styling)
- Story 007: Chat prompt panes (existing chat message handling)
- Story 012: Agent-triggered generation (SSE event handling)

## Open Questions
None.

## Tasks

### 001: Update message HTML structure in index.html to use bubble classes
**Domain:** weave
**Status:** done
**Depends on:** none

Replace the current message structure (`.message.user`, `.message.agent` with `.message-role` and `.message-content`) with the demo structure (`.message.message-user`, `.message.message-assistant` with `.message-bubble`, `.message-sender`). Update the empty state div to use `.empty-state` class. Verify HTML structure matches demo exactly so CSS from story 014 will work.

Files to modify: `internal/web/templates/index.html` (HTML structure only, no JavaScript changes yet)

---

### 002: Update addUserMessage function to create bubble structure
**Domain:** weave
**Status:** done
**Depends on:** 001

Update the `addUserMessage()` JavaScript function to create user messages with the new bubble structure: `.message.message-user > .message-bubble`. User messages do not have a sender name (implicit "you"). Remove the `.message-role` div creation. Verify messages display correctly with existing SSE flow.

Files to modify: `internal/web/templates/index.html` (JavaScript section)

---

### 003: Update handleAgentToken to create assistant bubble structure
**Domain:** weave
**Status:** done
**Depends on:** 001

Update the `handleAgentToken()` function to create agent messages with the new structure: `.message.message-assistant > .message-sender` ("Ara") + `.message-bubble`. The streaming class should remain for token-by-token display. Remove `.message-role` div creation. Verify streaming continues to work.

Files to modify: `internal/web/templates/index.html` (JavaScript section)

---

### 004: Update handleAgentDone to remove correct streaming class
**Domain:** weave
**Status:** done
**Depends on:** 003

Update `handleAgentDone()` to query for `.message.message-assistant.streaming` instead of `.message.agent.streaming`. Verify the streaming indicator is correctly removed when agent finishes responding.

Files to modify: `internal/web/templates/index.html` (JavaScript section)

---

### 005: Add system message display function
**Domain:** weave
**Status:** done
**Depends on:** 001

Create a new `addSystemMessage(text)` function that displays centered, muted system messages using `.message.message-system > .message-text`. This will be used for settings clamping feedback and other system notifications. Add it after the `addUserMessage` function.

Files to modify: `internal/web/templates/index.html` (JavaScript section)

---

### 006: Implement smart auto-scroll behavior
**Domain:** weave
**Status:** done
**Depends on:** 002, 003

Update `scrollChatToBottom()` to only auto-scroll when user is within 100px of the bottom. Add a check: `if (chatMessages.scrollHeight - chatMessages.scrollTop - chatMessages.clientHeight < 100)` before scrolling. This preserves scroll position when users read history. Test that new messages auto-scroll when at bottom, but don't jump when scrolled up.

Files to modify: `internal/web/templates/index.html` (JavaScript section)

---

### 007: Update empty state to show helpful hint text
**Domain:** weave
**Status:** done
**Depends on:** 001

Change the empty state div content from "No messages yet. Start chatting below." to a more helpful hint like "Start a conversation to generate images" or similar guidance. Update the `.empty-state` class usage to match demo structure. Consider adding a secondary hint about what to type.

Files to modify: `internal/web/templates/index.html` (HTML section)

---

### 008: Test message display with existing SSE events
**Domain:** weave
**Status:** done
**Depends on:** 002, 003, 004, 005, 006, 007

Manually test the full message flow: send chat message, verify user bubble appears right-aligned, verify agent bubble appears left-aligned with "Ara" sender, verify streaming tokens append correctly, verify agent-done removes streaming indicator, verify auto-scroll works when at bottom and doesn't jump when scrolled up. Test with long messages to verify wrapping. Test system messages if available.

No files modified (testing only)

---

### 009: Verify message grouping visual spacing
**Domain:** weave
**Status:** done
**Depends on:** 008

Test consecutive messages from the same sender to verify they are grouped visually with appropriate spacing. The CSS from story 014 should handle this, but verify it works correctly with the message structure created by JavaScript. If spacing is wrong, investigate whether CSS selectors need adjustment or if message structure is incorrect.

No files modified (visual verification, may require CSS tweaks if story 014 CSS doesn't match)

---

## Notes
This story focuses on visual presentation of messages. The underlying message handling (SSE events, streaming tokens, adding messages to the DOM) already works from stories 007 and 012. This story restructures the message HTML to use bubbles instead of plain divs.

Current message structure (from story 007):
```html
<div id="chat-messages">
  <div>User: Hello</div>
  <div>Assistant: Hi there</div>
</div>
```

New message structure (this story):
```html
<div id="chat-messages">
  <div class="message message-user">
    <div class="message-bubble">Hello</div>
  </div>
  <div class="message message-assistant">
    <div class="message-sender">Ara</div>
    <div class="message-bubble">Hi there</div>
  </div>
  <div class="message message-system">
    <div class="message-text">Settings adjusted: steps 150→100</div>
  </div>
</div>
```

The CSS for these message styles comes from story 014. This story implements the HTML structure and ensures JavaScript correctly creates message elements with the new classes.

System messages (like settings clamping feedback from story 011) need special styling to appear centered and muted, distinct from conversation messages.

The chat input remains at the bottom of the chat panel, below the message scroll area. The input structure from story 007 is preserved but may receive updated styling from story 014.

Auto-scroll behavior: When a new message arrives via SSE and the user is already scrolled near the bottom (within 100px), automatically scroll to show the new message. If the user has scrolled up to read history, do not auto-scroll (preserve their position).
