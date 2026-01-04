# Weave

Diffusion based Gen AI through conversation rather than explicit prompting and knob turning.

## Philosophy

**Simplicity through intelligence.** The agent handles prompt engineering, model selection, extensions, and parameter tuning so users can focus on their creative intent. You describe what you want; Weave figures out how to make it happen.

**Local-first.** Your hardware, your generations, no API costs, no quotas, no privacy concerns. Your prompts and images never leave your machine. Running locally means the agent can generate as many preview iterations as needed without worrying about cost.

**Conversational over configurational.** Chat is the primary interface, not forms and sliders. "Make it more blue" works because the agent understands context. The conversation *is* the workflow.

**Transparency without burden.** Users can always see what's happening—the resolved prompt, the model, the parameters—but they don't have to. The settings pane is always visible, never hidden, but you're not required to understand it.

**Native performance.** Go handles orchestration and UX; C handles GPU compute. No Python runtime, no pip, no conda, no cgo. Two clean processes communicating over Unix sockets.

**The agent observes and adapts.** When you manually tweak a setting, the agent notices. It might ask why, offer suggestions, or simply learn your preferences. Even in "advanced mode," you're still in a conversation.

## The Application

### User Experience

#### The Split-Screen Interface

Weave presents a split-screen interface:

- **Left pane**: Conversational chat with the Weave agent. Generated images appear inline as thumbnails. You describe what you want, ask for changes, upload reference images, and iterate through conversation.

- **Right pane**: Live view of the "resolved" settings—the exact prompt, model, parameters, and extensions that would be sent to compute. As you converse, you watch the right pane update in real-time as the agent interprets your intent.

Each generated image in the chat history links to its exact settings snapshot. Click any previous message to see the state at that moment. Select any point in the conversation and start typing to branch from there—full conversation context is preserved for the branch.

#### Example Conversation

```
You: a dragon perched on a castle tower

Weave: [generates two quick previews]
       Here are two directions—which feels closer?
       1. Western dragon, dark and imposing
       2. Eastern dragon, serpentine and elegant

You: 1, but make it more colorful

Weave: [generates two more previews]
       Added more color. Which palette?
       1. Jewel tones—emerald scales, ruby accents
       2. Sunset colors—orange, gold, deep purple

You: 2 is great. can you make it breathing fire?

Weave: [generates preview]
       Added fire breath. The flames pick up those sunset 
       colors. Want me to render a final high-quality version,
       or keep iterating?

You: final version

Weave: [switches to quality model, generates]
       Here's the full-quality render. I used the slower model
       for better detail on the scales and flame effects.       
```

The agent generates fast previews for iteration and switches to quality models for final output. Running locally means there's no cost concern—the agent can show you options freely. This behavior is configurable via agent profiles.

#### Agent Profiles

Users create and switch between agent configuration profiles—human-readable markdown files that seed the agent with preferences, default styles, and behavioral hints:

```
~/.weave/profiles/
├── agent.md              # Base defaults
├── anime-art.md          # "Always use anime-style LoRA, soft shading"
├── product-photography.md # "Clean backgrounds, studio lighting"
└── company-brand.md      # "Use brand colors, approved fonts"
```

Profiles extend the base `agent.md`. The agent can suggest profile modifications during conversation but never changes them without explicit user consent. Both direct file editing and conversational modification are supported.

#### Working with Reference Images

```
You: [uploads photo of a person in a dynamic pose]
     a robot in this pose, industrial setting

Weave: [generates preview using pose from photo]
       Matched the pose from your reference. The robot's 
       proportions follow the human figure. Industrial 
       background with machinery and steam.
       
You: [uploads artwork with a specific visual style]
     can you make it look more like this art style?

Weave: [generates with style transfer]
       Applied the color palette and rendering style from 
       that artwork—the heavy shadows and muted tones.

You: the lighting is too flat
     [manually adjusts CFG scale from 5.0 to 7.5 in settings pane]

Weave: I see you bumped up the guidance scale. I'll regenerate
       with stronger prompt adherence. I'm also going to try
       a more dramatic lighting description.
       [generates with adjusted settings and enhanced prompt]
```

The right pane showing resolved settings is always visible—you're never locked out. When you manually tweak settings, the agent notices and may comment, ask questions, or incorporate your preference. The conversation continues even when you're adjusting parameters directly.

#### Conversation as Context

The agent maintains full conversation history. "Make it more blue" works because the agent knows what "it" refers to. "Go back to the one with the red background" works because the agent can reference any prior state. Select any previous message, start typing, and you're branching from that point.

### Architecture

```
┌─────────────────────────────────────┐
│  Weave Application (Go)             │
│  - Web UI (split-screen chat)       │
│  - CLI interface                    │
│  - Agent/LLM orchestration          │
│  - Conversation state management    │
│  - Settings resolution              │
│  - Image analysis (CLIP, OpenPose)  │
└──────────────┬──────────────────────┘
               │ Unix Socket
               │ Binary Protocol
               │ /run/weave/compute.sock
┌──────────────▼──────────────────────┐
│  weave-compute (C Daemon)           │
│  ┌────────────────────────────────┐ │
│  │  Core Registry                 │ │
│  │  ┌────────┬───────┬─────────┐  │ │
│  │  │ Vulkan │  CPU  │  CUDA   │  │ │
│  │  │  Core  │ Core  │  Core   │  │ │
│  │  └────────┴───────┴─────────┘  │ │
│  │  Model Loading (SafeTensors)   │ │
│  │  Inference Execution           │ │
│  └────────────────────────────────┘ │
└─────────────────────────────────────┘
```

**Why this split?**

- **Go excels at**: Networking, concurrency, web serving, LLM integration, conversation management
- **C excels at**: GPU compute, memory control, hardware interfaces, inference execution
- **Unix sockets**: Clean process boundary, no cgo complexity, language-agnostic binary protocol
- **Portability**: The C daemon could be used by any language, not just Go

The compute daemon stays simple and generic. It receives generation requests and returns images. All complexity—chat logic, LLM reasoning, image analysis, extension selection—lives in the Go application.

### Compute Cores

**Vulkan Core (Primary)**: Cross-platform GPU support for NVIDIA, AMD, and Intel. Works on both RTX 4070 desktop and Intel Arc laptop. This is the default and covers the majority of users.

**CUDA Core (Optimization)**: Better performance on NVIDIA hardware. Optional, for users who want maximum speed on supported GPUs.

**CPU Core (Fallback)**: Enables running models too large for available VRAM. SD 3.5 Large FP16 (~24GB) can run on a 32-core Threadripper with 128GB RAM. Slower but functional—useful for quality-focused generation when time isn't critical.

### The Agent

The agent is the intelligence layer between you and the compute daemon. Its responsibilities:

- **Interpret natural language** into concrete generation parameters
- **Decide when to preview** versus when to ask clarifying questions
- **Select appropriate models**: fast models for iteration, quality models for final output
- **Manage extensions** (LoRA, ControlNet, IP-Adapter) transparently based on user intent
- **Optimize prompts** for the selected model's strengths
- **Track context**: remember what "it" refers to, what you've tried, what you prefer
- **Observe and respond** to manual setting changes

The agent uses a local LLM for privacy. Your conversations never leave your machine.

### Extensions & Capabilities

The agent abstracts these so users don't need to understand them:

- **LoRA**: Style and concept modifications. "Anime style," "pixel art," "photorealistic skin textures"—the agent selects and applies the right LoRA at the right strength.

- **ControlNet**: Structural guidance from reference images. "Pose like this," "follow this composition," "use the edges from this sketch." The agent detects what you want (pose, depth, canny edges) and configures ControlNet appropriately.

- **IP-Adapter**: Style transfer from reference images. "In the style of this artwork" triggers IP-Adapter to extract and apply stylistic elements.

- **img2img**: Iterative refinement. "Make the background more dramatic" applies img2img to the previous result with appropriate denoising strength.

When a user uploads an image or describes a constraint, the agent decides which extension to use. Advanced users can override these decisions via the settings pane.

### Models

**Primary Target**: Stable Diffusion 3.5 family (modern architecture, best quality/speed tradeoff)

| Model | Steps | Time | Use Case |
|-------|-------|------|----------|
| SD 3.5 Large Turbo | 4 | ~1-2s | Fast iteration, quick previews |
| SD 3.5 Medium | 28 | ~5s | Balanced quality and speed |
| SD 3.5 Large | 28 | ~8s (GPU) | Maximum quality, final output |

The architecture supports additional models as they emerge. The agent handles model-specific prompt optimization and parameter defaults, keeping the user experience consistent regardless of which model is active.

## Non-Goals

- **Not a Python wrapper.** Native Go + C implementation. No Python runtime, no pip, no conda, no virtual environments. The compute daemon is pure C; the application is pure Go.

- **Not a node-based workflow tool.** This isn't ComfyUI. You won't be connecting nodes to build pipelines. Conversation is the interface—the agent builds the pipeline for you.

- **Not cloud-dependent.** Runs entirely on local hardware. No API keys required for core functionality. No usage quotas. No subscription.

- **Not a settings-first UI.** You shouldn't need to understand CFG scale, sampler selection, or scheduler types to generate an image. Sensible defaults; the agent handles optimization.

- **Not maximally configurable at the expense of usability.** Every feature must earn its complexity. If it makes the product harder to use without proportional benefit, it doesn't belong.

- **Not cgo.** The Go and C components are separate processes communicating over Unix domain sockets. No cgo bindings, no shared memory complexity, no build system nightmares.

- **Not a passive tool.** The agent actively helps. It generates previews, asks questions, notices your adjustments, and adapts. If you wanted a dumb parameter form, you'd use something else.