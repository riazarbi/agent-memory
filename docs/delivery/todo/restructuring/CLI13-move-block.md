# CLI13 — `move-block`

**Milestone:** restructuring  |  **Builds on:** CLI12 (link rewriting)

Goal: one block moves to another existing node (`move-block ID --to STEM`), keeping its ID, with
inbound `[[old#^ID]]` links rewritten. Spec: III.8. The piece `split` and `merge` rest on.

Also in this unit (ruled at CLI12's banking): `rename` checks `NEW` as strictly as `write` checks a
new stem (any folder in the tree carrying it, loaded or not; names differing only by case). See
III.8 *`rename`, exactly*.
