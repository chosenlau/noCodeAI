# Agent Memory System Plan

## Goal

Use one memory model for ChatAgent and Graph generation. Summary must be stored separately from recent messages so it is never lost by list trimming.

## Redis Structure

Use the recommended mixed structure:

```text
memory:{appId}:summary   -> string, latest summary
memory:{appId}:messages  -> list, recent user/assistant schema.Message JSON
```

Rules:

- `summary` is long-term memory and is never stored in the trim-able message list.
- `messages` is short-term memory and can be trimmed by count.
- Any update refreshes TTL for both keys.
- `GetMessages` returns `SystemMessage(summary)` first, then recent messages.

## MySQL Structure

Keep full chat history as the source of truth.

- User messages: `messageType=user`
- AI messages: `messageType=ai`
- Summary messages: `messageType=summary`

When a new summary is generated, append a new summary row. The latest summary row is the active summary.

## Loading Memory

Both ChatAgent and Graph should call the same memory loading path:

1. Read Redis memory.
2. If Redis has summary or messages, use it.
3. If Redis is empty, load from MySQL:
   - latest summary;
   - recent non-summary messages after latest summary;
   - write summary to Redis summary key;
   - write recent messages to Redis message list.
4. Build context as `summary + recent messages + current request`.

## Summary Trigger

Start with a simple threshold:

- after AI message is saved;
- if recent memory size is above threshold, asynchronously summarize;
- summary failures must not block chat or code generation.

Later improvements:

- use token estimation instead of character count;
- add a Redis lock `summary-lock:{appId}`;
- keep a small tail of recent messages after summary.

## Implementation Order

1. Inject `ChatSummaryAgent` into `ChatHistoryService`.
2. Change Redis memory storage to summary key + message list key.
3. Update `LoadChatHistoryToMemory` to fill the separated structure.
4. Add `EnsureMemoryLoaded` to the chat history service.
5. Make ChatAgent and Graph use `EnsureMemoryLoaded`.
6. Replace TODO summary generation with `ChatSummaryAgent.SummarizeChat`.
7. Add tests for separated summary and recent message loading.
8. change the prompt for summary agent ,demanding it have to protect the critical info about the app(arch,users demand and so on)
