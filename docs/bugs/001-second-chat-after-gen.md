# Bug 001: Second image generation fails with Ollama chat error

## Status

Fixed

## Summary

After successfully generating an image from a prompt, attempting to continue the conversation with the agent fails. The user sees a generic error message while the logs reveal an Ollama-specific issue with system message ordering.

## Symptoms

**User-facing error:**
```
An error occurred while processing your message. Please try again.
```

**Log output:**
```
2026/01/06 07:39:41 Ollama chat error for session 77206647584152ee9775e3ccb34a5a72: system message must be first in conversation
```

## Steps to reproduce

1. Start the weave application
2. Begin a conversation with the agent
3. Submit a prompt that triggers image generation
4. Wait for the image to generate successfully
5. Continue the conversation (send any follow-up message)
6. Observe the error

## Expected behavior

The conversation should continue normally after image generation. Follow-up messages should be processed without error.

## Actual behavior

The second message (and any subsequent messages) fail with a generic error. The agent cannot continue the conversation after generating an image.

## Root cause analysis

The error message "system message must be first in conversation" indicates that when the conversation history is sent to Ollama after image generation, the system message is not positioned at the beginning of the message array.

Possible causes:
- Image generation response is being inserted before the system message
- Conversation history reconstruction does not preserve system message position
- System message is being omitted entirely on follow-up requests

## Affected components

- `weave` (Go) - Likely in the Ollama client or chat session handling
- Session management logic

## Investigation notes

- Session ID in log: `77206647584152ee9775e3ccb34a5a72`
- First request succeeds, indicating initial session setup is correct
- Issue manifests only after image generation completes

## Priority

High - This prevents multi-turn conversations, a core feature of the chat interface.

## Resolution

**Root cause**: In `internal/conversation/manager.go`, the `BuildLLMContext()` method was appending a trailing context message with `RoleSystem` instead of `RoleUser`. This violated Ollama's requirement that system messages must be first in the conversation.

**Fix**: Changed the trailing context message role from `RoleSystem` to `RoleUser` in `BuildLLMContext()`. This is consistent with how `NotifyPromptEdited()` already handles similar context injection.

**Files changed**:
- `internal/conversation/manager.go` - Fixed role assignment, updated docstring
- `internal/conversation/manager_test.go` - Updated tests for new role
- `internal/conversation/integration_test.go` - Updated integration tests
