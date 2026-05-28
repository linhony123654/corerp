# CoreRP Acceptance Checklist

## 目的

这份清单不回答“功能有没有写过”，只回答一件事：

```text
CoreRP 是否已经达到 world-first persistent narrative runtime 的终态闭环。
```

判断原则：

- 代码存在，不等于闭环完成
- API 可用，不等于运行质量达标
- UI 有面板，不等于作者真的能理解和干预世界
- 只有“世界能持续运行、人物会被世界改变、作者还能看懂和干预”时，才算真正完成

## 验收分级

### A. 已完成闭环

满足以下条件时，这一层才算完成：

- [ ] 功能已接入主链路，不是孤立接口或演示代码
- [ ] 至少经过一次真实运行验证，而不只是静态阅读
- [ ] 上下游语义一致：runtime、API、frontend、文档没有互相打架
- [ ] 失败时能观察和定位，不是黑箱

### B. 半完成

符合以下任一情况，都只能算半完成：

- [ ] 已有代码和接口，但长期运行质量未验证
- [ ] 已能产出结果，但结果质量不稳定或不可解释
- [ ] 只有局部面板可见，作者还不能系统诊断
- [ ] 依赖人工补救，不能自然维持闭环

### C. 未完成

符合以下任一情况，视为未完成：

- [ ] 只有文档、设想或字段，没有真实运行链路
- [ ] 功能只在单点生效，没有进入 runtime 主循环
- [ ] 语义依旧依赖旧模型，兼容层还在反向主导系统
- [ ] 无法回答“为什么发生、为什么没发生、接下来会怎样”

## 终态验收项

### 1. World-First 主语义

彻底完成标准：

- [ ] 世界是入口，角色不是产品主语
- [ ] `focus_character` 只表示观察视角，不再篡改 scene truth
- [ ] `participants` 稳定表示当前场景在场者
- [ ] `participant_details` 成为 switch / director / trace / UI 的统一参与者模型
- [ ] 旧字段只作为兼容层，不再反向决定新语义

完成标志：

- 新前端即使不读取旧 `character / active_character / loaded_characters`，仍能正常工作
- 切换视角不会把场景收缩成“玩家 + 当前角色”

### 2. Population -> Persona 晋升闭环

彻底完成标准：

- [ ] background NPC 会因为 location / pressure / event / relationship 获得 attention
- [ ] attention 增长会稳定触发 promotion，而不是随机硬编码
- [ ] promoted NPC 会进入 director candidate 与 scene runtime
- [ ] promotion 后会留下 identity core，而不是一次性升格
- [ ] promotion / growth / scene involvement 都可追踪、可解释

完成标志：

- 新的主要角色可以从世界人口中自然长出来
- 不需要预先塞很多角色卡，runtime 也能长出“主要人物”

### 3. 人物自然成长闭环

彻底完成标准：

- [ ] promoted persona 会被经历持续塑形
- [ ] 人格变化有慢变量，不只是当前轮临时状态
- [ ] relationship、memory、pressure 会反向影响后续行为
- [ ] 人物的变化能在 trace / memory / identity 层被看到

完成标志：

- 同一个人物在长期运行后，能表现出“被经历改变了”
- 这种变化不是 prompt 漂移，而是 runtime 结构可解释结果

### 4. Director 选人闭环

彻底完成标准：

- [ ] director 主要按 scene / world / pressure / relationship / population 状态选人
- [ ] `player_role` / `scene_shell` / `scene_presence` 不混入 speaker candidate
- [ ] director 的候选胜出和落选理由可解释
- [ ] follow-up / chain speaker 不是纯随机补位，而是有结构理由

完成标志：

- trace 能清楚回答：
  - 为什么这个人上场
  - 为什么另一个人没上场
  - 这次是 scene、pressure、faction、relationship 中哪类因素主导

### 5. World Structure 驱动闭环

彻底完成标准：

- [ ] factions / locations / pressures 不只是可编辑数据
- [ ] world structure 会持续影响 planner / scheduler / tick / director / events
- [ ] 不同 location / faction / pressure 组合会稳定改变世界走向
- [ ] 作者修改 structure 后，runtime 能观察到可验证差异

完成标志：

- 世界结构变化会真实改变事件生成、人物调度和世界压力走向
- 不是“界面保存成功了”，而是世界行为真的变了

### 6. Autonomous Simulation 闭环

彻底完成标准：

- [ ] 无用户输入时，世界仍能持续推进
- [ ] tick 会稳定产出 pressure / faction / population / event 演化
- [ ] 长时间运行不会迅速空转或退化成噪音
- [ ] tick 的结果可观测、可暂停、可恢复、可手动干预

完成标志：

- 世界在无人交互时仍会发生可解释、可积累的变化
- 作者能区分“真实演化”与“随机抖动”

### 7. Trace / Authoring 可解释闭环

彻底完成标准：

- [ ] trace 能展示 scene participants、director candidates、step traces、关键事件
- [ ] 作者能看见“为何切换 / 为何不可切换 / 为何入选 / 为何未入选”
- [ ] checkpoint / preset / rollback / trace 历史能互相联动
- [ ] 作者能从 trace 看到世界压力、人口变化、事件后果

完成标志：

- 作者工具不是“很多按钮”，而是“能诊断世界为什么这样运行”

### 8. 作者干预闭环

彻底完成标准：

- [ ] 作者主要通过 world structure / population / scenes / player role / presets 干预世界
- [ ] 干预后能在 runtime 中看到可验证结果
- [ ] 不需要频繁手改角色定义来救 runtime
- [ ] 世界级 authoring 已经比角色卡编辑更重要

完成标志：

- 世界可以被“运营”和“调度”，不是只能靠改 prompt 或角色卡补锅

## 最终完成定义

只有以下问题都能回答“是”，才算项目达到终态：

- [ ] 世界自己能转吗？
- [ ] 人物会被世界改变吗？
- [ ] 新主要角色能从人口层自然长出来吗？
- [ ] director 会顺着世界状态而不是角色卡来选人吗？
- [ ] 作者能看懂世界为什么这样运行吗？
- [ ] 作者能稳定干预世界，而不是靠人工救火吗？

## 当前项目判断口径

给后续模型或开发者的简短结论：

```text
功能骨架完成度已经很高；
终态闭环完成度还不能按“已完成”判断。
```

当前更接近：

- world-first 主语义：**半完成，接近验收**（骨架已切换，API 与 Runtime Audit 前端主路径 fallback 已通过短测/搜索验证；兼容层和底层旧存储字段仍需继续收束）
- population / director / world structure / simulation：**已接线，并已有 30+ tick、120 tick、200 tick、多样本矩阵、真实 world 目录矩阵、structure outcome 分叉与 API 层 authoring 证据，仍待更大样本池验收**
- 人物自然成长、长期自治演化、作者级诊断能力：**已交付核心机制，待长期运行验证**
