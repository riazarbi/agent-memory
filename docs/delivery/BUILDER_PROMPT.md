Build the work unit specified in `docs/delivery/doing/`. That folder contains exactly one markdown
file: it is your specification. Read it in full, then read **every document in `docs/spec/`** and
`CLAUDE.md` in full.

Build exactly that unit's scope, on top of the code already in the tree. Do not build beyond it —
anything the unit lists as not in scope belongs to later work, and building it now is a defect.

Work test-first. The QA gate must pass. Do NOT modify or weaken the gate, the documents, or any
existing test: if an existing test blocks you, STOP and report it rather than changing it.

Write real tests that fail when the code is wrong — no vacuous assertions, no fake doubles where the
standards forbid them. Every acceptance criterion in the unit must have a test that would fail if
that criterion were violated. Integration tests gate on a bounded reachability check (a
short-timeout connection attempt), never a client call that can retry indefinitely.

If a live environment is reachable per `CLAUDE.md`, use it while you work.

Work noninteractively: make and record your own decisions; fail fast if a needed command is blocked —
report it rather than working around it. Run NO git commands whatsoever. Leave all changes
uncommitted.

Return: (1) what you built, task by task; (2) the module layout; (3) the QA-gate result, pasted;
(4) which test covers each acceptance criterion; (5) assumptions you made where the documents were
silent; and (6) **SPEC GAPS** — anything unclear, contradictory, or insufficient in your inputs. The
spec-gaps section is your most valuable output: be specific and complete.
