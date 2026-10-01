# 荣庆堂产品示例世界：作者声明与验证

本次作者声明已通过现有 Studio schema 与 RP readiness 预检：十名原有 NPC 的性情、公开语气、对玩家的关系与称呼全部 READY；没有不完整的关系方向。

## 作者来源与适用边界

用户已委托完成产品并授权作出 canon/design 决定。本文件与 [rongqing-product-world-spec-2026-10-01.json](rongqing-product-world-spec-2026-10-01.json) 是在该授权下明确创作的 CoreRP 产品示例声明。人物之间的亲疏、角色与称呼是本示例的作者选择，进入世界的依据是提交的声明；运行时不得根据姓名再补全原著情节。它不是原著校注，也不是从模型输出、历史对话或冻结世界数据库反推得到的事实。

原始来源是 [real-rongqing-world-spec.json](real-rongqing-world-spec.json)。新文件保持原始世界的全部结构，逐项保留十一人的 key/name/place/player、九处地点、十条链接及距离、开局时间、人口、金钱、库存、版本与世界名。玩家仍在怡红院，贾母仍在荣庆堂。玩家的原始声明未改。仅为十名 NPC 添加 `persona` 与 `public_presentation`，并添加十对玩家熟识关系和二十条明确方向的 `relationships`。没有新增 routine、对象、活动、账目、事件、时间推进或初始移动。

两种文本用途分开：`persona` 是演员私有的作者性情和开场偏好；`public_presentation` 只描述可用于公开叙述的说话风格，不含私人愿望或情绪事实。关系中的 `role` 描述 from 一方相对于 to 一方的角色；`address_to` 是 from 如何称呼 to；`self_reference` 是 from 的自称。双方方向都明确写入，没有依赖运行时自动推断反向关系。

## 十名 NPC 的创作选择

| NPC | 对玩家的角色与称呼 | 私有开场意愿 | 公开声音 |
| --- | --- | --- | --- |
| 林黛玉 | 姑表妹妹；宝玉、宝哥哥 | 说些彼此真正在意的话，得到认真倾听 | 清灵、含蓄，偶有轻巧反问 |
| 薛宝钗 | 姨表姐姐；宝玉、宝兄弟 | 听清来意，给出有用而不强求的回应 | 平和、清楚，有分寸 |
| 贾母 | 祖母；宝玉、玉儿 | 知道来意，陪晚辈把话说完 | 亲切、从容，轻巧而稳重 |
| 王熙凤 | 嫂子；宝玉、宝兄弟 | 分清闲聊或具体请求，再判断能否相帮 | 爽快、鲜活，适度打趣 |
| 李纨 | 嫂子；宝玉、宝兄弟 | 让交谈舒适，分清陪伴与建议 | 温和、朴素，回应直接 |
| 妙玉 | 清修友人；宝玉、宝二爷 | 真诚而不过分侵扰地交谈 | 清简、克制，边界清楚 |
| 贾探春 | 妹妹；二哥哥、宝玉 | 交换有意思的想法，先听清实际问题 | 明快、有条理，坦率不盘问 |
| 袭人 | 日常侍女；宝二爷、二爷 | 听清当下需要，在可做范围内回应 | 柔和、自然，提醒具体 |
| 晴雯 | 怡红院侍女；宝二爷、二爷 | 听实在的话，接有趣来意，能说清拒绝 | 鲜活、简短，俏皮而真诚 |
| 紫鹃 | 潇湘馆侍女；宝二爷、二爷 | 听清来意，尊重彼此意愿 | 细致、平实，温柔而清楚 |

开场意愿是作者偏好，不是永久心情、已生效承诺、待办账本或已经完成的动作。声明明确允许随实际交谈和已发生的事调整；需要什么、能否答应与下一步行动仍须服从当前证据和合法能力。没有预设疾病、赠礼、茶席、家务成果或隐藏争执。人物身份不增加经济/管理权限；照应他人也不提供读取其私人状态的权限。公开声音是风格线索，不是必须说出的台词或每回合必须做的手势。

本文件没有为 NPC 之间自动建立熟识关系。除明确列出的玩家配对外，其他身份关系依既有观察和后续正式声明处理；人物名字和角色类型不能补出新的身份知识。

## 可复现验证与来源摘要

新声明原始 UTF-8 文件 SHA-256：

```text
sha256:31b6ce345006c380e34cf1d4e8384283e45c5aaa8f34e51b9d06dfce8dc44cf7
```

冻结来源原始 UTF-8 文件 SHA-256：

```text
sha256:e9d4ac788451be22dd610403d3278e3c6fb20516a1854595dd25b557bc806e47
```

从项目 `backend` 目录执行的预检命令：

```sh
TMPDIR=/tmp GOTMPDIR=/tmp \
GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 \
GOPROXY=off GOTOOLCHAIN=local \
/usr/local/go/bin/go run ./cmd/corerp-rp-preflight \
  -spec ../docs/rp-runtime-r1/rongqing-product-world-spec-2026-10-01.json
```

结果 PASS，退出码 0：`corerp.rp-preflight.v1`、源摘要匹配、总体 `READY`；十名 NPC 各自 `persona`、`relationship_to_interlocutor`、`address_to_interlocutor`、`public_presentation` 均为 `READY`，`incomplete_relationships=[]`。命令直接使用当前 Go source 与现有缓存，关闭依赖下载；没有建立世界或调用 provider。

独立 Node 结构断言 PASS：去除新增 `acquaintances`、`relationships`，并仅去除人物的 `persona`、`public_presentation` 后，与原始冻结 JSON 深度相等；玩家对象深度相等；十个熟识配对、二十个关系方向都完整。这比 Golden authored-overlay 比较更严格：Golden 当前还允许 routine，但本声明没有添加它。十名 NPC 文本亦通过现有每项最多 500 字等 Studio 校验。

作者源步骤的完成状态：作者源与预检已完成；该步骤没有修改原始冻结来源、代码、UI、预览数据库或已发布世界，没有 provider 调用、提交或部署。技术 READY 只说明声明完整且合法，不能代替真实会话中的声音、连续性、主动性或用户体验验收。后续集成应以此源摘要创建独立产品示例，并在有授权的运行环境中验证实际 provider 输入、公开输出和恢复；发布状态由父任务管理。

## 产品入口与公开叙述客户端增量

后续明确授权的 UI 集成在现有 `/studio/create` 增加「起点示例」选择：默认自定义世界保持原样，选择大观园可检查完整十一人声明，仍需创建授权范围、玩家身份、凭证与本地恢复同意。切换不会覆写自定义草稿。预设读取上面的同一 JSON，逐请求独立克隆，使用原有 Studio owner 创建端点、包哈希生成/自定义包导入、不可变请求标识、冻结请求核对及中断恢复流程；没有自动创建或假保存回执。代码位于 `src/lib/studioWorldPresets.ts`、`src/lib/studioCreate.ts` 和 `src/components/StudioCreate.vue`，作者源摘要未变化。

`src/lib/narrativeStream.ts` 保留并验证 `composition_version` 和 `fact_groups`。v2 的每行必须有对应分组，分组拼接必须与独立 `event_ids` 完全相同且每个来源只出现一次；stream done 的分组还须逐行等于实际片段引用。未知版本、重复/缺失/乱序来源和不一致分组均拒绝完成。Play 刷新历史前和恢复原叙述后使用同一验证器，错误保留当前文本及恢复意图。v1 stream 同样核对独立来源；旧 saved v1 若未暴露独立 ID 列表，仅做结构/唯一性验证以保留可读性，不升级为 v2，也不声称完成独立权威核对。无版本旧 prose 保持原来的多段重复引用兼容行为。客户端不能证明原话的语义真实性；冻结来源与已保存内容的权威核对仍由后端负责。

Core 的合法空公开输入亦得到支持：只有 v2 且 `lines=[]`、独立 `event_ids=[]`、`fact_groups=[]` 全部一致，stream `count=0` 时才可完成。部分为空、来源缺失、空分组夹在非空正文中及空 v1/legacy stream 均拒绝。专门协议断言覆盖合法空 v2 的 saved/stream 两条路径与不一致空结果，不用补造事实来满足客户端。

已执行的客户端验证：

```sh
node --experimental-strip-types scripts/rp-runtime-role-create-checks.mjs
node --experimental-strip-types scripts/rp-product-client-checks.mjs
npm run build
```

PASS：原有自定义人设/未知关系/方向称呼/私有公开区分/导入包检查；预设文件摘要与完整 spec 相等、草稿和逐请求克隆隔离、必要授权/预算校验；v1/v2 元数据保留、saved exact-once/order、stream 分组一致以及旧 prose 兼容；Vue TypeScript 校验和 Vite 生产构建。

可复现的浏览器检查（已有本地依赖，无安装）：

```sh
npm run dev -- --host 127.0.0.1 --port 4317 --strictPort
node scripts/rp-product-studio-browser-checks.mjs http://127.0.0.1:4317
```

浏览器 PASS：390×844 完整声明检查无横向溢出；预设↔自定义往返保留草稿；需要 consent/授权/凭证；提交 body 与作者 JSON 完整相等；测试在网络发出前主动中断创建，原请求保留且 reload 后完全相等；没有假「世界已保存」回执；凭证不落盘且 reload 后清空；无页面脚本错误。检查同时执行现有 `rp6-stream-parser-checks.mjs`，验证逐字节 UTF-8、完成前预览、legacy 多来源 prose 及十种断流/顺序/数量/结束标记/来源错误拒绝。首轮浏览器发现预设 select 的可访问名称含选项文字，已改成显式 `aria-labelledby` 并通过复测。

浏览器测试不连接真实后台：所有 `/api/` 请求被拦截中断，未伪造成功响应，没有创建世界或模型调用。本 worker 没有部署/提交。真实 Studio 保存、实际 v2 历史与重新连接的全链验收由父任务的存储集成及运行检查负责，不能从这次客户端测试推定已完成。

模型设置保留原来的 `fullProse` checkbox 和 `full_prose` wire semantics，但显示名称改为「模型组织叙事」，避免把有限事实组织误称为自由小说扩写。说明区分首次呈现与重新生成：普通世界及本预设的首次自然叙述不为呈现增加人物决策或叙述模型调用；只有作者明确声明世界 FullProse，才允许一次首次组织。已保存正文直接读取，重新生成使用当前选择的模型配置。模型可选择来源约束的节奏/措辞，不改写原台词或补写动作/心理。此次仅改名称、说明和类型注释，没有改配置值、checkbox 行为或后端。

名称/说明增量验证 PASS：`node --experimental-strip-types scripts/verify-api-profiles.mjs` 保持全部现有 profile 检查通过（网络部分由脚本自身的 fetch fixture 拦截，没有真实模型调用）；随后 `npm run build` 完成 Vue 类型检查和 Vite 生产构建，`git diff --check` 通过。
