# CLI12 — `move` and `rename`

**Milestone:** restructuring  |  **Builds on:** CLI11

Goal: a node can be relocated with its subtree (`move STEM --parent P`) and given a new stem
(`rename STEM NEW`), with every `[[stem]]` and `[[stem#^id]]` link rewritten on rename.
Spec: III.8. Introduces store-wide link rewriting.
