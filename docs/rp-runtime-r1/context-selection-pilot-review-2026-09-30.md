# 统一上下文 · Step小样读样记录

本次两份独立世界采用同一作者明确声明的合成配置、相同28轮模拟历史和相同末轮问题。before runtime为f4d5051f，after为f6c96778；Step step-5-preview、low、4096、120s与原一次有界修复不变。两份前28轮实际调用的是本地mock，不是Step，也不是人类RP。每份第29轮才转发到原Step中转；两次真实请求均首试成功，Narrator使用deterministic。

| 实际末轮请求 | before | after |
| --- | ---: | ---: |
| user message内Context wrapper字节 | 99807 | 65135 |
| HTTP JSON请求字节 | 114684 | 80093 |
| 当前问题全文副本 | 4 | 1 |
| 旧回答全文副本 | 2 | 1 |
| topic相关旧交流组 | 1 | 1 |

after的真实NPC packet为65090字节，低于65536；46项重复原话使用同包来源引用，省略5项较低优先级OwnActions。必要角色、当前问题与完整旧约定继续存在。wrapper的45字节差额不属于NPC packet预算。上下文wrapper大小下降34.7%，HTTP请求下降30.2%。这些是该固定合成样本的实际大小，不是所有场景的性能比例。

玩家问：Nora，那本蓝皮诗集，你当时怎么答应我的？

before公开回答：我当时说的是：蓝皮诗集可以带来，但缺了末页。

after公开回答：我当时说的是：蓝皮诗集可以带来，不过它缺了末页。

两版都正确回忆了“可以带来，但缺末页”，都没有宣称诗集已递交。此小样没有证明新版本更像人物或更好玩；它证明去掉冗余和部分低优先级历史后仍能使用28轮之前的精确来源。单次耗时23.758s/8.933s，受模型随机性及缓存影响，不能得出因果速度提升。中转返回usage是服务方报告，未独立验证计费/缓存/思考量。

完整公开样本及安全请求统计见[JSON](context-selection-pilot-2026-09-30.json)。临时HTTP诊断代理仅记录长度、计数与标准usage，不保存API凭据、模型请求正文或private全文；台词是经typed链批准的合成世界公开台词。旧proxy fixture初次省略chat/completions路径，运行时正确拒绝endpoint，双方零请求；失败另存[preflight](context-selection-pilot-preflight-2026-09-30.json)，不算模型失败。

这不是冻结荣庆堂Golden、真实32轮或真人RP验收。相同配置的新源码全32仍待；作者canon未声明，不能用本合成配置代替真实世界设定。R1 NOT DONE。
