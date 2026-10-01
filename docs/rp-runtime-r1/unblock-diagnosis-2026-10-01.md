# R1 卡点复查与最小推进路线

当前结论：没有证据要求推倒账本或 typed owner。实际卡点是体验输入没有就绪、叙事模型缺少独立推理控制、验证与预览交付脱节。R1 仍未验收，不能以新工具或短对话代替原两个出口。

## 确定的问题

1. **真实冻结配置缺数据。** 11 个角色含 1 个玩家、10 个 NPC；10 个 NPC 全缺 `persona`，没有关系、称呼、熟人或日程声明。不是新 builder 漏读了旧字段。实际预检返回 `INCOMPLETE` / exit2；四个冻结文件 hash 未变。继续对这份输入运行体验 Golden 只能验证不补 canon 的负例。来源：[声明](real-rongqing-world-spec.json)、[预检结果](rongqing-preflight-2026-10-01.json)、`backend/internal/storage/rp_decision.go` 的 readiness 装配。

2. **推理控制没有覆盖叙事。** 原界面明确将该配置限定于 NPC 决策和输入解析，原 `narrative.Config` 没有相应选项。不是说旧界面承诺过支持，而是 R1 使用同一推理模型做叙事时存在配置缺口。本轮增加独立 `CORERP_NARRATIVE_REASONING_EFFORT` / `CORERP_NARRATIVE_DISABLE_THINKING`，并让用户明确选择的模型 override 也传给两个 narrator provider。缺省不发送新参数、不继承 NPC 的环境设置或凭证；未改变模型时限、token 预算或世界事实校验。实际接收 HTTP 的测试先出现两个 RED，修复后四个 override/default 子场景通过；独立凭证、独立环境、非法配置也通过。尚不能把先前每次超时或 prose unavailable 都归因于这个缺口。

3. **修改尚未进入在线预览。** 本次直接校验 `/proc/722907/exe` SHA 为 `fea2067a…`，仍是旧运行时；R1 的原本地运行时为 `f4b63534…`。GitHub 同步不等于预览升级。直接替换程序仍不能让旧空配置变成角色世界，交付时需要同时有可用角色声明。

4. **验收顺序不能继续原样重复。** 原32轮在第29轮因 Step 两次 attempt 超过120秒而触发合法沉默回退，原 no-fallback 断言失败；该事实保留，没有单凭这个结果判定账本损坏或改成PASS。技术故障保护要定向验证，实际提供器稳定性要独立记录，完整原32仍是最终验收项。不能每个小改动都先花大段时间跑完整32，再对没有设定的 Golden 复评。

## 本轮的具体工作

- 新增 `backend/cmd/corerp-rp-preflight`，直接复用 Studio 的 `Validate` / `RPConfigurationReadiness`，没有第二套 readiness 算法、模型调用或世界写入。无效 JSON/拓扑、体积上限、缺人设、缺称呼、关系方向、未知关系和私有人设不出现在报告均有检查。
- 新增独立两人测试作者世界与短测入口：[候选 fixture](rongqing-minimal-test-fixture-2026-10-01.json)、`scripts/verify-rp-runtime-r1-short.mjs`。它采用讨论中的祖孙候选，明确 `test_author_only / NOT_APPROVED`；减少了角色数量并改变玩家初始位置，不是冻结 Golden，不是已审定真实世界设定。框架不会读取它替旧世界补 canon。
- 四轮真实 Step 决策均首试成功，称呼使用了“宝玉”，第四轮复述了第三轮的承诺；第二轮有实际提交的招手。完整首轮短测因末轮 Full Prose 失败而 **exit1**，失败没有删除。公开样本见 [四轮记录](rongqing-minimal-short-samples-2026-10-01.json)。
- 修复 speech token 插入时的引号外重复句号：已提交原话和其中的嵌套引号、字面 token、标点仍逐字保留，只处理模板在其后另加的冗余 `。`。原话无句末标点、独立逗号/感叹号均保留。七个 RED 子场景及现有 speech/事实/私有边界套件已通过。
- 新增同事实叙事复验入口 `scripts/diagnose-rp-runtime-r1-narrative.py`，只对测试世界 SQLite 只读备份重新渲染，不重跑 NPC、不改变世界事实。提供器往返只记录 HTTP 状态、finish reason、长度/计数；不保存凭证或 reasoning 内容。

## 最小路线

1. 先完成一个明确的贾母/宝玉角色版本。候选已做成文件，作者可修订；不要求先补完另外九个角色才能看到可玩短段。旧配置仍原样保留为 missing-data 负例。
2. 同一份角色设定、同一模型条件先跑4–8轮并审读公开台词、来源和叙事。每次只修可复现的缺口，保留失败样本。先看关系/称呼、情绪回应、上一轮承诺、实际手势和未知信息，避免用全32代替这个检查。
3. 用明确场景复核持续意图和近期记忆。如果它们在短段实际失败，再扩展现有 packet / 本人已应用决策历史；当前最多三个私有历史不能宣称为完整目标生命周期或长期语义记忆。不先新增另一套角色代理状态。
4. 数据与短段体验成立后再升级预览，让实际游玩与被验证版本一致；随后运行完整原32、可比较的真实 Golden 和真人 rubric。原32失败、旧 Golden 缺数据、新测试输入变化均应如实记录。

此路线是在用户授权调整推进顺序下收窄 R1，不取消世界可信性、隐私边界或最终32/Golden要求，不扩展R2/R3。

## 本次验证状态

- `go test`：narrative、decision、core、新 CLI 完整包 PASS；HTTP模型 override 定向 PASS；相关 `go vet` PASS。
- 两名角色、四个玩家回合的首轮短测：四个 NPC 决策成功、末轮叙事 FAIL，整体exit1保留。
- 同事实旧/新叙事复验两次真实Full Prose均首试成功：请求中`reasoning_effort`从未发送变为`low`；三项事件来源、head21、事件21、台词8、决策4、回合4均相同。两次耗时60.967/38.113秒，不足以证明稳定提速，也不能归因首轮原叙事失败。详见 [同事实结果](narrative-controls-same-facts-2026-10-01.json)。测试器首次读错`event_ids`字段且只读到了保存原叙事，零模型调用、FAIL保留；修正后显式新渲染的结果才作为实际两次复验。
- frontend TypeScript/Vite构建 PASS；存储六项事实权限、叙事持久化/重启及回退定向 PASS（3.755s）。全部本轮任务已终态。
- 原全后端12包覆盖、原32第29轮FAIL及冻结Golden17 NOT READY仍是先前 `f4b63534` 检查点，不转写成新叙事源码的整套回归结果。
- 本轮源码、测试和公开证据用于 GitHub 分支审阅；发布状态以最终回执为准。没有覆盖真实 canon、部署预览或通过真人体验验收。逐项检查及源码hash见 [验证记录](unblock-verification-2026-10-01.json)。

四轮仍有具体质量风险：首轮“身子可好些了”暗示未经声明的病史；“娘儿俩”在本设定中的称呼是否自然，以及反复“安安心心”“老祖宗”的模板感，需要人工判断。新版prose结尾还复述了对白中的承诺，不能说文学表现已明显改善。这些不会因引用ID合法而自动变成真实前史；现有语义硬检查不能穷尽此类隐含主张。当前三个本人私有历史也不是完整持续目标或长期记忆。这些是后续短段要验证的实际缺口。

参考：SillyTavern 的正式上下文组装也明确包含角色、用户 persona、场景和聊天历史。这里可借鉴它先准备角色内容与连续历史的工作方式，继续由 CoreRP 自己的提交与观察边界约束事实。[官方 Prompt Manager](https://docs.sillytavern.app/usage/prompts/prompt-manager/)。
