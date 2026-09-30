# CoreRP project rules

Read `/home/ubuntu/AGENTS.md` and the shared coding-workflow instructions for local engineering changes.

## Play interface

- `docs/ui/corerp_themes.html` is the **only approved UI prototype** for the live Play experience. Its Claude-style light palette, chat-first composition, top status capsules, left drawer, top-right anchored menu, floating composer, transparent single-line public status and four-theme switch are the visual/interaction baseline. Earlier UI prototypes, screenshots and design proposals are obsolete; do not use them as competing visual requirements.
- The prototype is an offline demonstration: its story data, fake model choices and timed thinking phases are **not product behavior**. Implement in Vue at the real default entry (`src/components/PlayWorkspace.vue`), bind to existing server observations/actions, and show only publicly known request state. Do not copy demonstration dialogue into live records or imply an unconfigured model is connected.
- Keep world authority, permissions, request-key recovery, observation privacy and existing Play business features intact. Prototype omissions do not repeal backend requirements. Apply the same Play theme tokens to features the prototype does not depict without restyling the separate Studio/Inspector entrypoints.
- For visual acceptance, compare the prototype and live Play at the same viewport in main chat, drawer, menu, detail sheet, each theme and request-status states; check keyboard, multiline input and scroll clearance. A fixture screenshot tests layout, not real-provider integration.
