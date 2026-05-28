# Looping Isekai Return DCL

Original return-by-death inspired world pack for CoreRP.

This pack is intentionally not an official IP adaptation. It demonstrates the
CoreRP DCL shape:

- declarative `manifest.yml`
- world/population/scenes/presets patches
- declarative hooks with no script execution
- installable into `worlds/` through the DCL loader

Primary runtime loop:

1. Start from `first_loop`.
2. Save a checkpoint before a risky route.
3. Run ticks or turns until pressure diverges.
4. Archive the experiment report.
5. Replay from checkpoint and compare `witch_scent`, population promotion, and director candidates.
