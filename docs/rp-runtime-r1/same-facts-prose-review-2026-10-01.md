# 同一组公开事实的叙事配置对照

结果是：改变现有叙事配置能增加少量衔接，这个短样尚不能证明文学表现力明显改善。本次没有修改Narrator代码，也没有重跑NPC决策。

在此前真实Step八轮对话的SQLite备份上，使用当前 `f4b63534` runtime，对同一个已经结算的第二轮分别应用两个现有style override。输入、NPC已提交对白和点头、来源事件完全相同；事件数、head、台词数、决策数、回合数均保持不变。冻结八轮样本与数据库未修改。详细配置、公开全文、receipts见 [same-facts-prose-diagnostic](same-facts-prose-diagnostic-2026-10-01.json)。

| 配置 | 公开字数 | 公开结果 |
| --- | --- | --- |
| concise / terse / dialogue 100 / description 0 | 79 | 两条署名对白，结尾“（点头）” |
| standard / normal / dialogue 70 / description 40 | 96 | 合并一段，增加“你开口说道”“语气温和地回应”“随后，她点了点头” |

两个render均为真实 `full_prose` 成功，合计3次attempt；原八轮决策receipts没有增加。这个参数对照只说明当前表现受style影响，不能把字数增长当作体验验收。普通配置仍使用机械引语动词，并出现引号后叠加句号、逗号的痕迹。以上是代理的读样意见，真人rubric仍为PENDING。

首次采集脚本把 `data.view.lines` 错读成 `data.lines`，因此报“零字/来源缺失”。该FAIL原文件保留；实际HTTP render和持久化公开render已经成功。随后只读恢复两条公开记录，没有再请求模型、读取私有推理或改变世界。修正后的脚本读取正确envelope，但尚未重新执行；证据来自原两次成功render，不宣称修正脚本已通过重跑。

这是同事实的配置诊断，不是旧架构与新架构的对照，也不是荣庆堂Golden。它支持下一次审计检查公开事实与风格在Narrator入口的实际信息量，不能据此放开创造动作、物品、私有意图或新剧情。R1 RP Experience Exit仍未通过。
