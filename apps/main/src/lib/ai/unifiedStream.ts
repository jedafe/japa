// §115: implementation moved to @japaconnect/ai-panel; this shim keeps
// existing imports working. Main's deps are initialized in the (app)
// layout onMount before any chat surface sends.
export { type ToolSelectionOverride, type StreamChatOptions } from '@japaconnect/ai-panel';
import { createStreamChat } from '@japaconnect/ai-panel';
export const streamChat = createStreamChat();
