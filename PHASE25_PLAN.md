# Phase 25 plan — first-quest survey (migration plan §3 item 5 prerequisite)

Status: COMPLETED (2026-11-16; dispatched under Tyler's "keep going
without confirming" instruction; no per-phase approval sought).
Analysis only: no Go code changed, no quest ported, no quest picked.

## Scope

Migration plan §3 item 5 says the first quest must not be picked
until dialogue, objects, and combat exist, and that selection needs
its own survey preferring the shortest `*.Quest.js` with no combat,
no instancing, and no cross-quest dependencies. All 11 engine
milestones are DONE, so this phase ran that survey: measure the
upstream quest plugins by script, apply the plan's criteria, produce
a ranked shortlist with evidence, recommend one quest, and record
what a port phase for the recommended quest would need first. The
deliverable is `QUEST_SURVEY.md` in the repo root, plus this plan
and a short `PORT_STATUS.md` note.

Out of scope: porting any quest or any other content, the far-WALK
bug, the y=3521–3522 map wall, the real production scrypt-hash
check, perf work, the ranged-data resolver gap. No scope expansion
without grounding. No Google Drive upload: the parent handles the
post-phase archive.

## Method

Three parallel measurement streams, integrated and re-verified by
the coordinator (who re-measured the inventory, read Doric's Quest
and Rune Mysteries in full, verified the Go hook-surface mapping
against `internal/plugins/hooks.go`, and spot-checked the dialogue
entries, the smithing gate, and the QuestRuntime storage claims):

1. QuestRuntime engine + registration machinery: `QuestRuntime.js`,
   `QuestList.QuestRuntime.js`, `ChatheadAnimations.QuestRuntime.js`,
   `Quests.plugin.js`, companion modules, QuestCape/QuestCommand/
   QuestHelper, puzzle/lock/scene machinery — read in full, with a
   def-field census across all 193 quest definitions and export
   usage counts.
2. Deep dives on the 6 shortest quest files, read in full with
   require chains: LearningTheRopes, DoricsQuest, DruidicRitual,
   CooksAssistant, WitchsPotion, ImpCatcher.
3. Deep dives on 5 more candidates (WhileGuthixSleeps + 9 `wgs/`
   units, RecruitmentDrive + 8 `recruitmentdrive/` units,
   CurseOfTheEmptyLord, BearYourSoul, VampyreSlayer) plus a
   whole-catalogue lexical sweep over all 194 `*.Quest.js` files,
   computed twice (Python regex + grep) with exact agreement.

Full method, patterns, and commands: `QUEST_SURVEY.md` §10.

## Measured results

- Inventory reconciled: 273 JS files / 277,911 lines under
  `quests/` (matches the plan); 193 `QUESTS` entries = 193
  top-level `*.Quest.js` files (261,277 lines); 194 `*.Quest.js`
  files by find (the extra is the CowbellAmulet item unit);
  195 registered quests on a members world (2 Mage Arena
  miniquest-likes); median quest file 1,199 lines.
- Sweep: COMBAT 129, INSTANCING 79 strict (25 strong), CROSSQUEST
  127 of 194 files; 25 files clean on all three strict flags.
  17 quests inflate via subdirectory requires (up to 7.8×).
- 12 quest files read in full; 7 ruled out with reasons, 5 ranked.
- Engine subset for quest 1 estimated at ~550–700 lines of Go +
  ~400–600 lines of tests (judgment, grounded in the engine
  analysis; excludes quest content).

## Outcome

`QUEST_SURVEY.md` written: method appendix, reconciled inventory,
30-shortest table, whole-catalogue sweep + 25-file clean list,
subdir true-size table, QuestRuntime engine analysis + minimal
subset, Go hook-surface mapping (45 firable / 63 table-only, with
the quest-relevant unfired families named), a ranked shortlist of
5 quests with measured evidence and per-candidate blockers, 7
ruled-out quests with reasons, and a brief-ready list of what a
Doric's Quest port phase needs first.

Recommendation (not a decision): Doric's Quest (188 lines, F2P,
standalone, all hooks firable, one small new seam — the animation
send path). Runners-up: Cook's Assistant (247), Rune Mysteries
(506), Druidic Ritual (222, members-only), Witch's Potion (249,
narrow combat coupling). The pick is Tyler's; if he picks a
different quest, the engine subset applies unchanged and only the
candidate-specific seams differ.

`PORT_STATUS.md` updated with a short pointer to the survey (header
left at phase 24: no code changed, verification counts unchanged).
`CONTENT_MIGRATION_PLAN.md` §3 item 5 annotated: survey complete,
recommendation recorded, decision pending Tyler.

## Verification

No Go files touched: `go build ./...`, `go vet ./...`,
`gofmt -l .`, `CGO_ENABLED=0 go build ./...` all clean; `go test
./... -p 1 -count=1` unchanged at 3,799 passing cases across 17
packages; `go test -race` passes. Zero JS files in the tree; no
probe references; nothing written under `~/tsps-bfd81e3`.

## Open question

Which quest to port first — a decision for Tyler after the survey
lands. The coordinator recommends Doric's Quest (§6 of the survey)
but did not pick one.
