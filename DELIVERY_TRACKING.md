# CoreRP 终态闭环交付跟踪

> 与 `ACCEPTANCE_CHECKLIST.md` 逐项对应。
> 每完成一项验收标准，在此打勾并给出简短交付说明。

---

## 验收项 1：World-First 主语义

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- 前端 10 处 `focus_character || character` fallback
- API handler 6 处 `character` fallback
- 旧字段仍在反向主导系统行为

**交付计划：**
1. 前端删除所有 `|| character` fallback，只依赖 `focus_character`
2. API handler 删除 `character` fallback，强制使用 `focus_character`
3. 响应中 `focus_character` 保证始终有值
4. `participant_details` 成为 UI 统一参与者模型

---

## 验收项 2：Population → Persona 晋升闭环

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- attention 驱动因素单一（主要靠 tick exposure 线性累积）
- promotion 周期约 200 ticks（~3.3h），未调优
- 无降级/遗忘机制

**交付计划：**
1. 丰富 attention 计算：location match、pressure match、event relevance、relationship delta
2. 调优 promotion 阈值/周期
3. 增加降级/遗忘机制
4. 完整 trace 展示晋升过程

---

## 验收项 3：人物自然成长闭环

- [ ] **彻底完成**

**当前状态：** 尚未达到终态 → 推进中

**差距：**
- `evolveIdentityCore` 逻辑简单，trait 变化浅
- 缺少"慢变量"人格骨架
- 长期运行后变化不可感知

**交付计划：**
1. 丰富 `evolveIdentityCore`：事件类型 → trait 映射
2. 增加慢变量：trait 渐变而非突变
3. identity core 变化在 trace / memory 中可见

---

## 验收项 4：Director 选人闭环

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- follow-up / chain speaker 选择仍偏随机补位
- 决策权重可配置但可追踪性不够

**交付计划：**
1. 丰富 chain speaker 选择逻辑：情感状态、对话上下文、世界状态
2. director 决策产生结构化 trace（哪类因素主导）
3. candidate 胜出/落选理由进一步强化

---

## 验收项 5：World Structure 驱动闭环

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- Planner 仅 7 条硬编码 if-else 规则
- 不同 location/faction/pressure 组合的可验证差异不足

**交付计划：**
1. 增加 Planner 规则深度（从 7 条扩展到结构化的规则集）
2. 让 faction relationships 动态影响 NPC 互动
3. 让 location pressures 的组合产生更多行为分支
4. 作者修改 structure 后可验证 runtime 差异

---

## 验收项 6：Autonomous Simulation 闭环

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- 阈值触发可能出现抖动（在边界反复触发/不触发）
- 长期运行质量未验证
- "真实演化"与"随机抖动"难以区分

**交付计划：**
1. 增加 hysteresis（迟滞）机制防止阈值抖动
2. 增加 tick 演化历史趋势图
3. 长期运行验证（连续运行 N ticks 观测稳定性）

---

## 验收项 7：Trace / Authoring 可解释闭环

- [ ] **彻底完成**

**当前状态：** 接近完成 → 推进中

**差距：**
- checkpoint / preset / rollback / trace 联动诊断不够紧密
- 缺少"世界诊断报告"一键生成

**交付计划：**
1. 强化 trace 中 world state 变化的完整链条
2. 增加"诊断报告"：当前 world 健康状况、异常指标、建议干预
3. checkpoint 与 trace 历史联动回溯

---

## 验收项 8：作者干预闭环

- [ ] **彻底完成**

**当前状态：** 半完成 → 推进中

**差距：**
- 干预后缺少即时可验证反馈
- 角色卡编辑仍是主要操作路径

**交付计划：**
1. 干预后显示预期效果 vs 实际效果对比
2. 增加"干预建议"：基于当前 world 状态推荐作者操作
3. 让 world structure authoring 成为首选入口

---

## 当前会话交付（2026-05-27）

已完成：
- [x] TODO.md 5 个主线任务全部落地
- [x] Race condition 修复（`tickCount` → `atomic.Int64`）
- [x] 全项目接口检测（编译/测试/race/覆盖率/API一致性/Go接口实现）
- [x] 代码质量清理（删除 `VectorStore.db` 未使用字段，添加 5 处编译时接口断言）
- [x] `ARCHITECTURE.md` 更新过时描述
- [x] `SESSION_LOG.md` 补全本次会话记录
