# CLI13 — `move-block`

**Milestone:** restructuring  |  **Builds on:** CLI12 (link rewriting)

Goal: one block moves to another existing node (`move-block ID --to STEM`), keeping its ID, with
inbound `[[old#^ID]]` links rewritten. Spec: III.8. The piece `split` and `merge` rest on.
