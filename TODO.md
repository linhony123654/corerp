# CoreRP TODO

## 当前主线

项目主方向保持不变：

- `world-first persistent narrative runtime`
- `focus_character` 是观察视角
- `participants` 是场景在场者
- `participant_details` 是 switch / director / trace / UI 共用的结构化参与者模型
- 人物定义负责 persona seed，background NPC 通过事件与关注度自然晋升

完成标准请直接参考 [ACCEPTANCE_CHECKLIST.md](ACCEPTANCE_CHECKLIST.md)。

## 当前可信状态

以下项目已在代码中落地，并且本轮重新抽查通过：

- [x] `focus_character` 已成为内部主语义，`/api/characters` 与 `/api/instances` 输出新字段
- [x] `participant_details` 已进入 `/api/characters`、`/api/instances`、trace 视图与前端参与者面板
- [x] 视角切换不再覆盖 scene truth；切换 focus 后原场景参与者继续保留
- [x] director 已接入参与者模型；`player_role` / `scene_shell` / `scene_presence` 不进入 speaker candidate
- [x] director 权重已显式支持 `kind/source/loaded`
- [x] population runtime 已接入 attention / promotion / promoted persona / director candidate 主链路
- [x] world structure API、population API、simulation API 已存在并可正常读取
- [x] Simulation 运维接口已落地：`/api/sim/status`、`/api/sim/tick`、`/api/sim/pause`、`/api/sim/resume`
- [x] `PUT /api/worlds` 与 `PATCH /api/worlds` 已实现
- [x] `README.md` 当前与 world-first 主路线基本一致

## 待补验证

这些方向代码已存在，但当前不要夸大成“完全完成”：

- [ ] simulation 长期稳定性还需要更长时间回归；目前只能确认 `pressure_states / faction_tensions / npc_tick_exposure` 已产生
- [ ] world structure 对 planner / scheduler / tick 的深度驱动已接线，但仍需更多场景回放验证
- [ ] population growth 闭环已成型，但“人格慢变量长期塑形”还不是最终形态
- [ ] trace / 作者控制台已经能解释多数候选差距，但还不是完整的 runtime 诊断面板

## 文档约束

- [ ] 不再把“未来路线”写成 `[x]`
- [ ] 不再把错误的 HTTP 预期写成回归结论
  - `GET /api/switch` 返回 `405` 是正确行为
  - `POST /api/switch` 返回 `200` 才表示兼容路径可用
- [ ] `SESSION_LOG.md` 当前可作为变更素材库，不可直接当严格时间线；后续需要统一时区并重排

## 下一阶段

按价值排序，后续优先做这些：

1. 强化 simulation / population / world structure 的真实闭环，而不是继续堆角色定义。
2. 继续让前端与 API 实际依赖 `focus_* + participants + participant_details`，把旧字段压缩为兼容层。
3. 让 trace / 作者控制台更像 runtime 调试器，补齐“为何进入 / 为何未进入 / 当前世界压力如何作用”的解释链。
4. 继续让世界目录成为唯一主入口；人物卡只作为导入材料，不再主导运行时语义。

是否能把这些项从“做过”升级为“闭环完成”，统一按 [ACCEPTANCE_CHECKLIST.md](ACCEPTANCE_CHECKLIST.md) 判断。

## 最小验证

```bash
/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core
node --check web/app.js
/usr/local/go/bin/go build -o corerp ./cmd/corerp
pm2 restart corerp && pm2 save
```

## 建议抽查接口

```text
GET  /api/characters
GET  /api/instances
GET  /api/world-structure
GET  /api/population
GET  /api/population-insights
GET  /api/sim/status
POST /api/switch
```
