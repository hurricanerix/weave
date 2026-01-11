# Weave

> Thread by thread, the image emerges — not from your hand alone, nor the machine's, but from the space between where meaning is made.

Diffusion-based image generation through conversation rather than configuration.

:warning: This project is an early prototype :warning:

There are no gurantees that it works correctly.  See the [docs](./docs) for more details about the project.

## Design Principles

**Simplicity. Minimalism. Functional.** Every element earns its place. If it doesn't serve the user's creative intent, it doesn't belong.

**Make it work, make it right, make it fast.** Get the core experience right before optimizing. No premature complexity.

## Philosophy

**Simplicity through intelligence.** Weave's agent--Ara--handles prompt engineering, model selection, extensions, and parameter tuning so users can focus on their creative intent. You describe what you want; Ara figures out how to make it happen.

**Local-first.** Your hardware, your generations, no API costs, no quotas, no privacy concerns. Your prompts and images never leave your machine. Running locally means Ara can generate as many preview iterations as needed without worrying about cost.

**Conversational over configurational.** Chat is the primary interface, not forms and sliders. "Make it more blue" works because the agent understands context. The conversation *is* the workflow.

**Conversation is primary for everything.** Setup, configuration, model management, error handling--all happen through conversation. Standard OS patterns (window controls, file selection, conversation history) stay conventional. Everything Weave-specific flows through chat.

**Transparency on demand.** Users can always access what's happening--the resolved prompt, the model, the parameters--but this information stays out of the way until wanted. Transparency is available, not imposed.

**Ara observes and adapts.** Ara notices how you work. She reads tone, not just content. If you're iterating quickly with short responses, she keeps commentary brief. If you dismiss suggestions, she dials back. If you manually tweak settings, she might ask why, offer suggestions, or simply learn your preferences. You're always in a conversation, even when adjusting parameters directly.

## Who This Is For

Weave is for people who want to generate images without mastering the tooling. Artists, designers, creators--people who care about the output and don't want to learn about CFG scales, samplers, or ControlNet configurations.

You don't need to understand Stable Diffusion to use Weave. You don't need to be fluent in LLM interaction patterns. You describe what you want; Ara handles the complexity.

Power users aren't excluded--they can access everything under the hood--but they're not the default audience. Weave is simple first, powerful when needed.

## The Target Experience

### The Interface

Weave presents a focused, minimal interface. The left side holds the conversation--a narrow column where you chat with Ara and see generated images as thumbnails. The right side displays the most recent image as large as the window allows. The image is the primary output; it gets the primary real estate.

Click any thumbnail in the chat to display it on the right. You can then reference it directly: "Ara, let's go back to this one--I think we went down the wrong path." Ara understands which image you mean because it's visually selected, and she can restart from it because every image carries its own settings.

When you scroll up in the conversation, a "jump to end" button appears--a reminder that you're in history and a quick way back to the present.

On smaller screens, the layout adapts: the large image moves above the chat and shrinks to fit.

Settings and parameters exist but stay hidden by default. You can surface them when you want to see or adjust what Ara is doing, but you're never required to engage with them.

Each generated image carries its settings with it. Click to inspect what was used. This ties transparency to the artifact, not the interface--you see what happened when you want to, not constantly.

The conversation always shows what's happening. When Ara is generating, a progress bar appears in the chat. If something fails, an error appears inline--a red bubble with context about what went wrong. No silent failures, no mystery states.

### First Run

On first launch, Weave downloads the models it needs--the LLM that powers Ara and a default image generation model. These download in parallel. Once Ara is ready, she introduces herself and starts the onboarding conversation while the image model finishes downloading in the background.

Ara asks a few simple questions to understand how you want to work:

- "When you're iterating quickly, should I offer suggestions or stay quiet unless you ask?"
- "Do you have a default style you usually work in--photography, illustration, something else?"
- "How much do you want me to explain what I'm doing?"

Your answers shape Ara's default behavior. This isn't a form--it's a conversation that happens to configure the product. Power users can later edit the underlying configuration directly; most users never need to.

If the image model is still downloading when onboarding finishes, Ara keeps the user company--offering tips, answering questions about what Weave can do, or just chatting until everything is ready.

### Generating Images

```
You: a dragon perched on a castle tower

Ara: [generates two quick previews]
     Here are two directions--which feels closer?
     1. Western dragon, dark and imposing
     2. Eastern dragon, serpentine and elegant

You: 1, but make it more colorful

Ara: [generates two more previews]
     Added more color. Which palette?
     1. Jewel tones--emerald scales, ruby accents
     2. Sunset colors--orange, gold, deep purple

You: 2 is great. can you make it breathing fire?

Ara: [generates preview]
     Added fire breath. Want me to render a final high-quality 
     version, or keep iterating?

You: final version

Ara: [switches to quality model, generates]
     Here's the full-quality render. I used the slower model
     for finer detail.
```

Ara generates fast previews for iteration and switches to quality models for final output. Running locally means there's no cost concern--she can show you options freely.

When a request is ambiguous, Ara may generate multiple options to help narrow down what you want. If you'd rather she just pick, say so--she might still show a few thumbnails to confirm she understood, but she won't over-ask.

You can specify aspect ratio or dimensions naturally: "make it widescreen," "portrait orientation," "square." If you don't, Ara picks something reasonable based on the subject.

### Working with Reference Images

Drag images directly into the conversation. Ara can use them as a starting point for generation.

```
You: [drags in a rough sketch]
     clean this up, make it look professional

Ara: [generates preview]
     Used your sketch as a starting point and refined it.
     Let me know if this captures what you were going for.

You: [drags in a photo]
     make this look like a watercolor painting

Ara: [generates preview]
     Applied a watercolor style to your photo. Want me 
     to push the effect further or dial it back?
```

### Power User Mode

A button in the corner or a keyboard shortcut reveals the settings panel. It shows what Ara is working with: the resolved prompt, selected model, and generation parameters like guidance scale, steps, and sampler. Everything Ara decided on your behalf is visible here.

When you adjust parameters directly, Ara shifts from leading the conversation to coaching:

```
You: [opens settings panel, adjusts CFG scale from 5.0 to 7.5]

Ara: I see you bumped up the guidance scale--going for 
     stronger prompt adherence? I'll regenerate. I'm also 
     going to try a more dramatic lighting description to 
     take advantage of the higher guidance.
     [generates with adjusted settings]

You: [adjusts negative prompt directly]

Ara: Adding "blurry" to the negative prompt is good. You 
     might also try "out of focus" if you're seeing softness 
     issues. Tell me what you're trying to fix and I can 
     suggest more.
```

Ara becomes a coach--observing your changes, offering context, suggesting refinements. You're still in a conversation; you've just shifted who's leading.

If you want Ara to be quiet while you work, say so. She'll respect that until you invite her back in.

### Conversation History

Ara maintains full conversation history. "Make it more blue" works because she knows what "it" refers to. "Go back to the one with the red background" works because she can reference any prior state.

Select any previous point in the conversation and start typing to branch from there. Full context is preserved for the branch.

A sidebar lists previous conversations. Click to return to any past session. This is one of the few non-conversational UI elements--managing conversations follows the standard pattern every chat app uses.

### Model Management

Weave knows what models are available and what each is good for. When you need something that isn't downloaded yet, Ara handles it conversationally:

```
You: a picture of a cat eating ramen, anime style

Ara: I can generate this with the base model, but for really 
     good anime results we should grab the Anime-Style LoRA. 
     It's about 200MB. Want me to download it?

You: sure

Ara: Downloading now. This might take a minute--want me to 
     generate a preview with the base model while we wait?

You: yes

Ara: [generates preview]
     Here's what we get with the base model. The anime LoRA 
     will give you cleaner linework and more stylized proportions.
     
     [download completes]
     
     Got it. Regenerating with the LoRA now.
     [generates with anime LoRA]
     This should give you cleaner linework and more stylized 
     proportions.
```

Ara only recommends models she knows about. She won't hallucinate download links or suggest unsupported configurations.

### Saving and Sharing

Generated images appear in the conversation with a subtle save icon overlaid. Click the icon to save directly. Click the image itself to expand it full-size without the overlay--save is still available there, just not obscuring the image.

To share settings with someone else, ask: "give me the settings for this image." Ara provides a clean, copyable text block with everything needed to reproduce the result.

## Non-Goals

**Not a node-based workflow tool.** This isn't ComfyUI. You won't be connecting nodes to build pipelines. Conversation is the interface--Ara builds the pipeline for you.

**Not cloud-dependent.** Runs entirely on local hardware. No API keys required for core functionality. No usage quotas. No subscription.

**Not a settings-first UI.** You shouldn't need to understand CFG scale, sampler selection, or scheduler types to generate an image. Ara handles optimization.

**Not maximally configurable at the expense of usability.** Every feature must earn its complexity. If it makes the product harder to use without proportional benefit, it doesn't belong.

**Not a passive tool.** Ara actively helps. She generates previews, asks questions, notices your adjustments, and adapts. If you wanted a dumb parameter form, you'd use something else.
