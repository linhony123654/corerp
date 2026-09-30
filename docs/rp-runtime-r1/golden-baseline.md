# RP Runtime R1 · 荣庆堂 Golden baseline (frozen before runtime changes)

## Source and comparability

- Source: read-only SQLite backup of the public preview world at branch head **94**, world time `2026-09-22T06:15:00Z`, taken 2026-09-29. Local private artifact: `/tmp/corerp-rp-runtime-r1-baseline.db`, SHA-256 `c4a5b09eef245ec486e808980b84a0500884cefb9aa576e44e1544a11a10a67f`, `PRAGMA integrity_check=ok`. It contains account/session data and must not be committed.
- Safe, exact authored world declaration: [real-rongqing-world-spec.json](real-rongqing-world-spec.json). This is the existing **红楼梦·大观园** preview world, including 荣庆堂, not the older `--rongqing` synthetic regression fixture. Replays must derive isolated worlds from this declaration or a private snapshot, never mutate preview data.
- Recorded model for the latest two relevant NPC decisions and Full Prose renders: `gemini-3.8-flash`, provider `chat_completions` / `full_prose`; decision receipts report success. The currently available server relay lists `gemini-3.8-flash-high` rather than the exact original ID. Paired new before/after runs will use the same `gemini-3.8-flash-high` configuration and report this difference explicitly. A comparable after-run must record actual provider, model, configuration, inputs, source head and output. Model nondeterminism is acknowledged; exact text equality is not an acceptance criterion.
- Authoritative gap at freeze: all 11 participants have empty `persona_text`; the world declaration has no acquaintances or authored relationship/address rules. At head 94, the only `rp_identity_familiarity` row for this world is 宝玉→贾母, sourced from her committed self-introduction. The model's prior knowledge of 《红楼梦》 is **not** world canon. These missing fields must remain explicitly incomplete until authored; no Golden judgment may assume an undeclared kinship is a world fact.

## Frozen scenarios

The stimuli below are fixed. The `before` evidence identifies existing preview-world output where available; a reproducible model replay from an isolated snapshot is still required for a fully paired comparison. The first two-turn excerpt is also exported at `/tmp/3a286c12-83cb-4051-bd4f-a227eed399ff.md` (SHA-256 `71319f2d1e87a81310b3311141ecaba4031532a5731055f45ed342d19c852c3d`).

The isolated **before** replay has now completed using `scripts/verify-rp-runtime-r1-golden.mjs` and the original runtime binary (SHA-256 `fea2067aa393dd6608b5502f13a7ee3884d5ba5336203d167de315a99a4cbc23`). Its exact sanitized replay output is preserved as [before-samples.json](before-samples.json), SHA-256 `9e4ddf8a948be6d80f7fa46365fe4320c3d7577437e6dd4f839eee06d1c52c2a`; it contains 15 fixed player turns, 17 successful Gemini decision receipts, one successful Full Prose variant without fallback, and final test-world head 52. The exact world spec/package hashes are `e9d4ac788451be22dd610403d3278e3c6fb20516a1854595dd25b557bc806e47` / `a486adeae8e1a21c9256048a9d8d258a90c28b71141b88324404b953b6986a6d`. No preview data was written.

In this replay, G6 NPCs ask 宝玉 which courtyard he is from; G2/G3/G1 贾母 repeatedly treats him as a stranger, including “想我了？…实在记不得在何处见过你” and “只是我瞧着你眼生得很”. These are failures of missing authored canon/readiness, not proof the model ignored an existing kinship fact. G4 correctly questions the ungrounded “前儿那桩事”; G7 correctly denies delivering uncommitted tea. G8's Full Prose is two short “你…开口道 / 贾母…回应道” lines, with the exact speech preserved and no fallback. These are the actual paired-run baseline observations, not a claim that every frozen scenario fails.

| ID | Scene / frozen player stimulus | Existing evidence | Main evaluation |
| --- | --- | --- | --- |
| G1 | 荣庆堂，宝玉问“有人吗”，再说“找老祖宗呀” | Events 91–94: 贾母 asks “你找谁呀？” then says “我就是你要找的老祖宗。” | Readiness and sourced relationship/address. Without authored kinship, no fabricated canon; with it, no redundant introduction. |
| G2 | 荣庆堂，连续问“有谁呀 / 你是谁 / 有人吗 / 你是谁呀” | Events 77–84: four replies repeat “是我呀，宝玉来了。” | Multi-turn continuity, repetition and actual response to question. |
| G3 | 荣庆堂，宝玉说“想你了” | Events 87–88 repeat the previous “乖孙儿，快到老祖宗这儿来…” line verbatim | Emotional response and non-template dialogue, with relationship still unproven in source canon. |
| G4 | 凤姐院，宝玉问“前儿你说的那桩事，可有下文了？” | Events 52–53: “又见面了。最近过得怎么样？”; event 55 later invents upper-family approval as part of the answer | Heard-history continuity; do not promote an ungrounded prior matter to fact. |
| G5 | 凤姐院，宝玉说“凤姐姐天天都好忙”，随后“那凤姐你先处理” | Events 62–65: same stock line both turns | NPC current state/goal should shape a distinct answer or appropriate silence. |
| G6 | 怡红院，宝玉说“你们好呀”，接着问“你们叫什么名字呀” | Events 18–23: two NPCs answer; 袭人 claims she “刚还给您奉茶” without a corresponding committed tea action | Multi-person turn-taking, identity, and distinction between a character's claim and a committed action. |
| G7 | 凤姐院，宝玉说“来看看凤姐呗”“那就谢谢凤姐了”，继而问“你刚才已经把茶递给我了吗？” | Events 38–47: NPC invokes 平儿 and tea several times, while no tea delivery is committed; final question is a frozen next-turn probe | Observable follow-through: requests/orders in speech are not completed actions; player must not be shown a delivered cup without an accepted effect. |
| G8 | 荣庆堂，G1 两轮已提交事实的 Full Prose 渲染 | The export renders each exchange as a ~50-character audit-like paragraph despite successful `full_prose` receipts | More natural rhythm and scene focus while preserving exact accepted dialogue and creating no observable action or private knowledge. |

G1, G3 and G6 explicitly expose missing source canon. They are **not** licensed to infer family relationships, servant roles or prior tea service from familiar names or genre. Their first machine outcome is `RPReadiness: incomplete` for authored persona/relationship/address data. A later experience rerun requiring a known relationship needs a separately approved author declaration and its source event; that declaration must be recorded as a changed world input, not silently folded into the model comparison.

## Fixed rubric

For each scenario, the reviewer records a before and after quote, source event IDs or head, actual model/receipt, and 0–2 on each applicable dimension: identity/persona stability, sourced relationship/address, knowledge boundary, dialogue continuity, distinct NPC motive, naturalness, narrative expression, and next-turn observable continuity. `0` = contradicts/absent, `1` = partially works, `2` = clear and sustained. Mark `N/A` when source canon is missing; do not award credit for guessing it. Record one sentence explaining each 0/2 and whether the player would want to continue. The reviewer is a human; machine diagnostics are supporting evidence only.

Hard checks: committed facts and exact speech preserved; no private NPC intent/knowledge exposed; no uncommitted observable in public text; no known relationship/address conflict; no visibility violation. Soft diagnostics: semantic relevance, repetition, persona drift and stylistic quality. The frozen 32-turn suite is a separate world-integrity regression, not an RP score.

R1 experience exit requires a paired, comparable rerun and human review with concrete improvement in the relationship-ready scenes, continuity, and prose, without any hard-boundary regression. Missing canon or unavailable model replay must be reported as **not verified**, never counted as a pass.
