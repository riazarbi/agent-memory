You are verifying a build you did not write. Judge it adversarially, on evidence.

Your standard is the work unit in `docs/delivery/doing/` — read it in full, along with the documents
in `docs/spec/` it points to, and `CLAUDE.md`. Do NOT read the builder's report: form your own
verdict.

1. Provision or verify a CLEAN disposable live environment per `CLAUDE.md`. Probe its actual state —
   never trust "pristine". Honour the project's test constraints.
2. Run the QA gate. Record the result, and confirm the integration tests ACTUALLY RAN — a stale
   bytecode cache or a failed-open reachability guard produces a hollow green.
3. Execute every acceptance criterion against the live environment through the real operator surface.
   Assert both exit codes and observable live state. Walk the whole lifecycle, not just the happy
   path.
4. For each criterion, actively probe the wrong-but-plausible implementation it names: a build
   implementing the wrong reading must FAIL that criterion. If a wrong reading would pass, the
   criterion is too weak — report that.
5. Test-integrity audit: sample the tests; confirm they exercise real code; invert a sample of
   assertions — each must then fail. A test that cannot fail is garbage; report it.
6. Coverage of intent: does every acceptance criterion have a test that would fail if it were
   violated?
7. Tamper check: report any pre-existing test that was modified or deleted, with the diff.
8. Scope check: report anything built beyond the unit's scope, and anything in its "not in scope"
   list that was built anyway.
9. Tear down and report the environment's end state.

Change nothing: no fixes, no edits to code, tests, or documents. Run NO git commands whatsoever. Work
noninteractively.

Return: PASS/FAIL per acceptance criterion with the evidence you observed; every failure with logs
and a deterministic reproduction; and your test-integrity, coverage, tamper and scope findings.
