// Shared flag so the app layout can hide the global widget sidebar on /ai
// only while the chat-template rail is open — closing the rail hands the
// right edge back to the widgets. JapaAIChat mirrors its panel state here.
// `mounted` tracks whether the unified chat has mounted: until it has, the
// layout assumes the rail will open (its desktop default) so the widgets
// never flash into view during SSR/first paint.

export const chatTemplateRail = $state({ open: false, mounted: false });
