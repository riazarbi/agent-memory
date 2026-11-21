# Open Knowledge Format (OKF) and `memory`: recommendations

Prepared 2026-11-21. Sources: the OKF v0.3 specification (SPEC.md in
`github.com/open-knowledge-format/open-knowledge-format`), the okf.md guide, this repo's spec
(`docs/spec/agent-memory.md`), and hands-on tests with the `memory` CLI built from this repo.

## Summary

OKF is a new open format for sharing knowledge: a folder of Markdown files with YAML frontmatter.
It looks very like our memory store, so we asked whether we should adopt it. Our conclusions:

1. **Do not adopt OKF as the store format.** It solves a different problem: knowledge *exchanged*
   between parties who do not control each other. Its design choices cost us more than they give us
   in a store that one agent owns. Portability, if we need it, comes more cheaply from an export.
2. **OKF does not provide multi-agent, shared or distributed memory.** Looking into it did expose
   three gaps in our own design for sharing memory, which we should record as open design work.
3. **Adopt two ideas from OKF:**
   - a stored "content last changed" date, which fixes a real defect;
   - a review-by date for claims that expire.

   Also adopt two small conventions: record where a claim came from, and reserve a key prefix for
   protocols.

The decisions requested are at the end.

## Background

**OKF** was released by Google Cloud in June 2026. v0.3 came out on 2026-11-18, and the format is
still pre-1.0. It describes itself as a format for "the metadata, context, and curated insight about
data and systems". Its examples are database tables, metrics, APIs and playbooks.
- Each file is a "concept" and must carry a `type`. A concept is identified by its file path.
- Links are ordinary Markdown links by path.
- Since v0.2 it adds metadata about who wrote and who verified each concept, and whether it is
  still current, because agents increasingly generate this content.
- It prescribes no tools, storage or concurrency rules, and recommends git for distribution.

**`memory`** is a long-term store for one AI agent. It holds what the agent learns that it could not
look up again: corrections, preferences, people, commitments.
- Notes are identified by a short name (a "stem") that does not depend on where the note sits in
  the folder tree.
- Each paragraph carries a permanent ID.
- The tool's operations keep the store consistent: moving or renaming a note rewrites every link to
  it, writes are locked and atomic, and a write that would break the store is rolled back.
- The agent sees a summary index of the store kept to a fixed size.
- You can browse the store in Obsidian.

## Recommendation 1: do not adopt OKF as the store format

**Why it looked attractive:**
- portability, which is OKF's purpose;
- the belief that OKF has had more design thought than our store.

**Why neither reason holds:**

- **Portability is only worth paying for if someone needs it.** Today a store is read by one agent
  and by its operator in Obsidian. It can already be copied, versioned in git and shared between
  agents on one machine. OKF portability means *OKF tools* can read it, and no such reader is
  planned. If one appears, an export command (store to OKF bundle, on demand) provides most of the
  value. It also insulates us from OKF's churn: v0.3 shipped three days ago with two breaking
  changes.

- **OKF's design thought went into different problems.** It is strong where exchange needs it:
  trust and provenance for readers who do not know the writer, freshness, readers that tolerate
  anything, and versioning. It leaves out everything a writer needs:
  - It has no way to address part of a file.
  - It has no concurrency rules.
  - It ignores context-window budgets.
  - Its answer to moving a concept is to leave a deprecated stub at the old path. That suits
    exchange, where other people hold links you cannot update. In a store that is reorganised
    often, stubs pile up as clutter, which our design deliberately avoids.

  Our design was not ad hoc either. It rests on research into memory systems, and each decision has
  a recorded rationale. The two are about equally young; neither is battle-tested.

- **Converting would be expensive and would weaken what makes our store work.** Our tests show the
  two are incompatible in how links and identity work:
  - Moving a note keeps our links working and breaks OKF's. Each format sees no links at all in the
    other's files.
  - Switching to path-based links would turn our cheap, safe "move" into a rewrite across the whole
    store.
  - The agent would have to work with file paths instead of names.
  - Every note would need a `type`, which forces either a meaningless constant or a fixed
    classification scheme. We rejected the latter on purpose.
  - OKF's trust metadata uses nested YAML and footnotes, which our store forbids so that it stays
    valid for Obsidian.

**What it would take to change this:** a concrete consumer that needs to read memory live (an OKF
tool or another agent framework), or OKF reaching 1.0. Even then we would build the export first.

## Recommendation 2: record the gaps in sharing memory as open design work

OKF does not help with several agents writing one store, shared memory banks or memory replicated
across machines. Its only answer is "use git", plus per-file attribution. Checking our design for
these cases found:

- **One store, several agents, one machine: this already works.** Writes are locked, atomic,
  checked against changes made meanwhile, and retried on contention. Agents can be given different
  roles (for example, many agents that only add, and one curator that reorganises). What is missing
  is *attribution*: the store does not know which agent wrote what.
- **A shared bank linked into several agents' stores has three gaps:**
  1. **Name collisions.** Names are unique across a whole store, so if a private note and a shared
     note have the same name, one of them is dropped together with everything beneath it.
  2. **Unsynchronised writes.** Each store has its own lock, so two agents writing the same shared
     note through different stores are not coordinated.
  3. **Broken links in other stores.** Renaming a shared note rewrites links only in the store that
     did the rename. Links to it in other stores break.
- **Replicas on several machines need a merge strategy.** Two replicas adding to the same note
  produce a git conflict. Two replicas creating the same name collide.

**The underlying trade-off:**
- Store-wide names are ideal inside one store: links survive any reorganisation, and the agent
  never handles paths.
- They combine poorly across stores.
- OKF made the opposite choice because combining and exchanging are its purpose.

If shared memory becomes a goal, this is the decision to revisit. One option is names qualified by
the bank they come from, such as `team:priya`. That would be a targeted change, not an adoption of
OKF.

**Recommendation:** add these as open items in the spec, as documentation only, with no build work
now.

## Recommendation 3: adopt a stored "content last changed" date (fixes a defect)

**The problem:** a note's date is currently its file's modification time. That date is shown to the
agent, decides which notes the index drops first when it is over its size budget, and breaks ties
in search ranking. In our tests, starting with every note dated 2026-01-15:
- **Renaming one note** made two other notes show today's date, although the only change in them
  was a rewritten link. Every reorganising operation does this to every note it touches.
- **Cloning the store with git** gave every note today's date. Any sync or copy that does not keep
  file times does the same.

So routine maintenance makes old notes look new, and any sync erases their real ages. The index
then drops the wrong notes, and the agent misjudges how old a claim is. This gets worse with the
sharing and syncing scenarios above.

**The OKF idea:** OKF stores "the last meaningful change to the content" in the file itself, and
says explicitly that trivial edits such as link repairs do not change it.

**Recommendation:**
- The store records this date in each note's frontmatter.
- It changes when content changes (paragraphs, description, tags) and not when only links are
  rewritten.
- The displayed date comes from it, falling back to the file time for notes edited by hand.

This reverses an earlier design rule ("not stored"), for the reasons above.

**Size:** medium. It changes the store's file layout, so the spec, documentation, agent files and
tests change with it.

## Recommendation 4: adopt a review-by date for claims that expire

**The problem:** much of what an agent remembers expires: "Priya is out until 2026-12-31", "Sam
covers while she is away". Nothing tells the agent that such a claim has lapsed, and acting on a
stale claim is a known failure of memory systems.

**The OKF idea:** OKF's `stale_after` field holds an absolute date after which content must be
re-checked. Its rules are worth copying:
- stale means "check again", not "false";
- leave the field out if no date is known, and never invent one;
- a date that cannot be read is never treated as current.

**Recommendation, in two steps:**
1. **Now, small:** extend the agent's guidance on saving. A claim that expires carries a review-by
   date in its own text (for example `[review_by:: 2026-12-31]`), so the agent sees it whenever it
   reads the claim. It goes in the text rather than the note's metadata because expiry usually
   applies to one claim, not a whole note, and the agent does not currently see a note's metadata
   when it reads the note.
2. **Later, only if the agent misses these dates:** the tool recognises the field and visibly marks
   expired claims when reading, searching and showing the index. It would mark them, never hide
   them.

## Recommendation 5: two small conventions

- **Record the source of a claim.** The agent is told to "re-read the source before acting on a
  claim", but it is never asked to note where that source is. Add one line to its guidance on
  saving: when a claim rests on a file, link, message or person, say which. (Small, guidance text
  only.)
- **Reserve a key prefix for protocols.** OKF promises never to define keys that begin with `x-`, so
  extensions cannot collide with future versions. Recommendations 3 and 4 would add keys that the
  tool itself defines, so we need the same promise. (Small, documentation only.)

## Considered and not recommended

| OKF feature | Why not |
|---|---|
| Required `type` field | Forces a classification scheme on every note; we deliberately impose none. |
| `title` | The short name and the one-line description already cover it. |
| Lifecycle `status` and "superseded by" stubs | Correcting or deleting claims, and merging notes with link rewriting, already cover this. Stubs add clutter. |
| Change log file (`log.md`) | The agent's own event log and git already record history. |
| Attached files, copies of external material, citation footnotes, attested computations | They do not fit what memory is for: knowledge that cannot be looked up again. |

**Later, if shared memory goes ahead:** OKF's naming convention for who did something
(`human:<id>`, `<agent>/<version>`); its rule that a verification only counts if it is newer than
the last change; and a description for the whole store that tells an agent what a shared bank is
for.

## Where OKF independently agrees with our design

These are reassurance, not action:
- **Quoting values that look like numbers or booleans in metadata:** confirmed by test.
- **Leaving unchanged metadata exactly as written:** we already do this.
- **Attributing claims by a permanent key rather than by position, "because agents rewrite these
  documents frequently":** our paragraph IDs.
- **Folders give the hierarchy; links are untyped:** the same model.

## Decisions requested

| # | Decision | Recommendation | Size |
|---|---|---|---|
| 1 | Adopt OKF as the store format | No. Revisit if a live OKF consumer appears or OKF reaches 1.0. | none |
| 2 | Record the gaps in sharing memory as open spec items | Yes, documentation only | small |
| 3 | Stored "content last changed" date | Yes, as a defect fix | medium |
| 4 | Review-by date for expiring claims | Yes: guidance now, tool support only if needed | small, then medium |
| 5 | Record claim sources; reserve the `x-` prefix | Yes | small |
| 6 | OKF export command | Not now; build it when a consumer exists | medium |

## Appendix: test evidence

The tests used a sample store built with the `memory` CLI from this repo and a small checker
written from the OKF v0.3 spec.

| Test | Result |
|---|---|
| Our store, read as OKF | Valid bundle, but zero concepts, because no note has a `type`. |
| `type` added to every note | Our store stays valid; OKF sees six concepts. |
| Links | OKF sees no links, because it does not recognise `[[name]]` links. |
| An OKF-style link inside a note, then the target note moved | Our link still worked; the OKF link broke. A rename did not update the OKF link. |
| A note with the property `status=active` | Collides with OKF's reserved `status` field, so OKF readers would exclude the note from current knowledge. |
| OKF trust metadata added by hand | Our store rejects it (nested YAML), and the note disappears from the agent's view. |
| OKF per-claim citation (a footnote) | Our store refuses it. |
| An OKF bundle used as our store | Nothing loads: wrong folder layout, and its rich formatting (headings, tables) is not allowed in our notes. |
| A rename, then a git clone | Notes whose content did not change showed new dates (see Recommendation 3). |
