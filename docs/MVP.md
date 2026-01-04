# Weave MVP

> Prove that a conversational agent crafting prompts in real-time makes image generation dramatically easier than fiddling with settings.

## What This Proves

**Core thesis:** A conversational interface where an LLM agent handles prompt engineering makes image generation dramatically easier than traditional settings-heavy UIs.

The MVP validates this by demonstrating:
- You can describe what you want in plain language
- The agent translates intent into a prompt
- You watch the prompt evolve as you converse
- You generate images without touching a single setting
- Manual edits are respected, not clobbered

If users can go from "a cat in a wizard hat" to a generated image through conversation alone—and feel like the agent *understood* them—the thesis is proven. Prompt quality doesn't matter yet; the interface does.

## Scope

### Included

**User Experience**
- Web UI only (browser-based, no installation beyond running the binary)
- Split-screen layout: chat on left, live prompt view on right
- Generated images appear inline in chat
- Right pane shows prompt updating in real-time as agent responds
- User can manually edit prompt in right pane
- On prompt field blur, system injects "[user edited prompt]" into chat history
- Agent sees the edit notification and incorporates current prompt state
- Linear conversation (no branching in MVP)
- "Generate" button produces image inline in chat

**The Agent**
- Llama 3.2 **1B** via ollama (HTTP API) — fast iteration over quality
- No custom system prompt engineering knowledge — uses model's existing capabilities
- Agent crafts and refines prompt based on conversation
- Agent may suggest other settings as text (not auto-applied for MVP)
- Multi-turn context awareness ("make it more blue" works)

**Image Generation**
- SD 3.5 Large Turbo only (fastest iteration, fits 12GB VRAM)
- Vulkan compute core only (cross-platform: NVIDIA, AMD, Intel)
- Generation triggered by user request
- Positive prompt only (no negative prompt for MVP)

**Architecture**
- Go application serves web UI via HTMX
- SSE for live streaming of agent responses and prompt updates
- Go ↔ ollama: HTTP
- Go ↔ weave-compute: Unix socket with binary protocol
- weave-compute: C daemon with Vulkan core

### Excluded

- CLI interface
- Desktop application
- Agent profiles / `agent.md` configuration files
- Multiple model support (SD 3.5 Medium, Large, etc.)
- CUDA compute core
- CPU compute core for large models
- LoRA, ControlNet, IP-Adapter
- Agent controlling settings beyond prompt
- Conversation branching / history navigation
- Settings pane (only prompt field visible)
- Image upload for reference/pose/style
- Batch generation
- Model selection in UI
- Fine-tuned LLM
- Autonomous preview generation (agent asks, user confirms)
- Click-to-restore previous generation settings
- **Negative prompts** (positive prompt only for MVP)
- Custom system prompt engineering

### Hardcoded/Stubbed

| Setting | MVP Value |
|---------|-----------|
| Model | SD 3.5 Large Turbo |
| Steps | Hardcoded (4) or CLI arg |
| CFG Scale | Hardcoded or CLI arg |
| Resolution | 1024×1024 (or CLI arg) |
| Seed | Random (or CLI arg for reproducibility) |
| Compute Core | Vulkan only |
| LLM | Llama 3.2 **1B** via ollama |
| Negative Prompt | None (deferred) |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Weave App (Go + HTMX)                                  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Web UI                                           │  │
│  │  ┌─────────────────┐  ┌─────────────────────────┐ │  │
│  │  │  Chat Pane      │  │  Prompt Pane            │ │  │
│  │  │  - User msgs    │  │  - Prompt (editable)    │ │  │
│  │  │  - Agent msgs   │  │  - Live updates via     │ │  │
│  │  │  - Images       │  │    SSE or websocket     │ │  │
│  │  └─────────────────┘  └─────────────────────────┘ │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Conversation Manager                             │  │
│  │  - Chat history for LLM context                   │  │
│  │  - Current prompt state                           │  │
│  │  - Injects "[user edited prompt]" on blur         │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─────────────────────┐  ┌─────────────────────────┐  │
│  │  LLM Client         │  │  Compute Client         │  │
│  │  HTTP → ollama      │  │  Unix socket → C daemon │  │
│  └─────────────────────┘  └─────────────────────────┘  │
└───────────┬───────────────────────────┬─────────────────┘
            │                           │
            │ HTTP :11434               │ /run/weave/compute.sock
            ▼                           ▼
      ┌───────────┐              ┌─────────────────┐
      │  ollama   │              │  weave-compute  │
      │ Llama 3.2 │              │  C daemon       │
      │  (CPU)    │              │  Vulkan core    │
      └───────────┘              │  SD 3.5 Turbo   │
                                 └─────────────────┘
```

### Binary Protocol (Go ↔ weave-compute)

The protocol is fully specified in `protocol/RULES.md`. Key details for MVP:

**Design Philosophy: GPU-Friendly Wire Format**

Inspired by glTF, the protocol is designed for minimal CPU-side processing. The binary payload should be structured so the C daemon can pass buffer regions directly to the GPU without parsing or copying.

```
┌─────────────────────────────────────────────────────────────┐
│ Header (fixed size, CPU reads this)                         │
│   - magic, version, msg_type                                │
│   - request_id, model_id                                    │
│   - generation params (width, height, steps, cfg, seed)     │
│   - buffer_len (total size of what follows)                 │
│   - offset table (where each text segment lives)            │
├─────────────────────────────────────────────────────────────┤
│ Buffer (GPU-friendly, minimal touching)                     │
│   - text data, properly aligned                             │
│   - future: latents, control images, etc.                   │
└─────────────────────────────────────────────────────────────┘
```

**Prompt Offset Table:**

SD 3.5 has three text encoders (CLIP-L, CLIP-G, T5-XXL). Rather than separate length-prefixed strings, use a single contiguous buffer with an offset table:

```c
// MVP: positive prompt only
struct prompt_table {
    uint32_t buffer_len;          // Total size of text buffer
    
    // Positive prompts (offsets into buffer)
    uint16_t clip_l_offset;
    uint16_t clip_l_len;
    uint16_t clip_g_offset;
    uint16_t clip_g_len;
    uint16_t t5_offset;
    uint16_t t5_len;
    
    // Future: negative prompts (same structure)
};
// Followed by: uint8_t text_buffer[buffer_len]
```

**MVP simplification:** All three encoders receive the same prompt. Go writes the text once and points all offsets to the same location—no wire duplication, and C doesn't care whether they're aliased or distinct. Negative prompts deferred to post-MVP.

**Wire Format:**
```
┌────────────────────────────────────────┐
│ Magic Number (4 bytes): 0x57455645     │  "WEVE"
│ Protocol Version (2 bytes): 0x0001     │
│ Message Type (2 bytes)                 │
│ Payload Length (4 bytes)               │
└────────────────────────────────────────┘
```

**MVP Message Types:**
- `MSG_GENERATE_REQUEST` (0x0001) — Send prompt, get image
- `MSG_GENERATE_RESPONSE` (0x0002) — Image data back
- `MSG_ERROR` (0x00FF) — Error with code and message

**Auth:**
- `SO_PEERCRED` check immediately after `accept()` (kernel-verified UID/GID)
- Userland mode: only same-UID connections allowed
- Silent close on rejection (no response to unauthorized clients)

**Socket:** `$XDG_RUNTIME_DIR/weave/weave.sock` (0600 permissions)

**Why this architecture matters:**

1. **Two-process design** (Go + C) is load-bearing—no shortcuts. This enables the full vision where the C daemon could serve any frontend.

2. **Unix socket with binary protocol** is designed for future expansion (multiple models, streaming progress, cancelation).

3. **ollama for LLM** avoids custom inference code—focus on the product, not infrastructure.

4. **HTMX + SSE** keeps the frontend simple while enabling real-time prompt updates.

## User Flow

**1. Launch**
```
User starts weave (Go binary)
  → ollama must be running with Llama 3.2 1B
  → weave-compute must be running with SD 3.5 Large Turbo
  → Browser opens to localhost:8080

User sees:
  Left pane: Empty chat, input field at bottom
  Right pane: Empty prompt field
```

**2. Initial Request**
```
User types: "I want a cat wearing a tiny wizard hat"

Chat shows:
  [User] I want a cat wearing a tiny wizard hat

Request sent to ollama with user message

Agent responds (streamed):
  "A cat in a wizard hat! I'm thinking something magical 
   and whimsical. Here's what I have:"

Right pane updates LIVE as agent responds:
  Prompt: "a fluffy cat wearing a small wizard hat, magical 
           atmosphere, detailed fur, enchanting, fantasy art"

Agent continues:
  "Want me to generate a preview?"
```

**3. Generation**
```
User: "yes"

Go sends request to weave-compute via Unix socket:
  - prompt (from current state)
  - hardcoded: steps=4, cfg=1.0, size=1024x1024

Chat shows progress indicator

Image returns, appears inline:
  [Image: wizard cat]

Agent: "Here's your wizard cat! What do you think?"
```

**4. Refinement**
```
User: "make the hat more sparkly"

Agent: "Adding some magical sparkle to that hat!"

Right pane updates:
  Prompt: "a fluffy cat wearing a small wizard hat with 
           magical sparkles and stars, magical atmosphere, 
           detailed fur, enchanting, fantasy art, glitter"

Agent: "Generate another preview?"
```

**5. User Edit**
```
User clicks into prompt field in right pane
User adds "purple background" to prompt
User clicks elsewhere (blur)

System injects into chat history:
  [System] user edited prompt

Agent sees this on next interaction
```

**6. Continued Conversation**
```
User: "also make it a black cat"

Agent sees:
  - "[user edited prompt]" in history
  - Current prompt includes "purple background" (user edit)
  - User wants a black cat

Agent updates prompt, PRESERVING user edit:
  Prompt: "a fluffy black cat wearing a small wizard hat with 
           magical sparkles and stars, purple background, 
           magical atmosphere, detailed fur, enchanting, 
           fantasy art, glitter"

Agent: "Black cat with purple background—nice combo! Preview?"
```

**7. Final Generation**
```
User: "generate it"

Final image generated and shown inline
Agent: "Here you go! Anything else you'd like to change?"
```

## Success Criteria

MVP is **done** when all of the following work end-to-end:

| Criterion | Validation |
|-----------|------------|
| **Architecture works** | Go app successfully communicates with ollama (HTTP) AND weave-compute (Unix socket) |
| **Conversation works** | Multi-turn chat where agent maintains context ("make it more blue" references previous image) |
| **Live updates work** | Right pane shows prompt updating in real-time as agent streams response |
| **User edits respected** | Edit prompt field, blur → "[user edited prompt]" injected → agent incorporates edit on next message |
| **Generation works** | Clicking generate sends request to weave-compute, receives image, displays it |
| **Images inline** | Generated images appear in chat history at correct position |
| **Cross-platform GPU** | Vulkan core works on RTX 4070 Super (desktop) AND Intel Arc (laptop) |

**Not success criteria for MVP:**
- Performance optimization
- Beautiful UI
- Error recovery
- Multiple sessions
- Prompt quality (we're proving the interface, not the prompting)

## Open Questions

### Resolved

1. ~~**Binary protocol format**~~ → Defined in `protocol/RULES.md`. Updated with glTF-inspired design: single contiguous buffer with offset table for prompt segments, GPU-friendly alignment, zero-copy path to encoders.

2. ~~**Llama 3.2 variant: 1B vs 3B?**~~ → **1B for MVP.** Speed of iteration matters more than quality. The goal is proving the concept works, not perfecting metaprompting. Future options: move to 3B, fine-tune 1B, fine-tune 3B, or something else entirely.

3. ~~**How to detect "user edited prompt"?**~~ → **On blur, inject a message into chat.** When the prompt field loses focus, fire an event that injects "[user edited prompt]" (or similar) into the conversation history. Agent sees it and knows to incorporate the current prompt state. Future: use an LLM to summarize what changed.

4. ~~**HTMX streaming pattern**~~ → **Go streams via SSE or websockets.** SSE is simpler (one-way, server → client, native HTMX support). Websockets also fine. Implementation detail—either enables real-time DOM updates as agent responds.

5. ~~**System prompt / prompt engineering knowledge**~~ → **No embedded knowledge for MVP. No negative prompts.** The agent uses whatever Llama 3.2 1B already knows about image generation. We're proving the conversational interface works, not optimizing prompt quality. Negative prompts deferred—MVP focuses on positive prompt only.

### Can Defer

- Exact error messages and recovery flows
- Conversation persistence between sessions
- Keyboard shortcuts
- Mobile responsiveness
- Loading states and animations
- Negative prompt support

## What's NOT Deferred (Architecture Decisions)

These MVP choices are intentionally designed to support the full vision:

| MVP Choice | Why It Matters for Vision |
|------------|---------------------------|
| Unix socket to C daemon | Any future frontend (CLI, desktop) uses same protocol |
| Binary protocol (see `protocol/RULES.md`) | Versioned, extensible for streaming, progress, model switching |
| Conversation state in Go | Enables branching, history navigation later |
| Prompt state tracked separately | Enables "click to restore" on previous generations |
| Agent via ollama | Can swap to fine-tuned model without architecture change |
| HTMX + SSE | Can add WebSocket for bidirectional later if needed |

The MVP is minimal, but the architecture is not compromised.
