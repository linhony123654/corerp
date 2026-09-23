# CoreRP Play — 本地运行

默认 `/` 是真实 RP Play；`/demo` 是旧的静态 Story/Inspector 演示。当前 Play 固定使用 M2 切片中的 Lin，1 名玩家、3 名真实 NPC、5 个地点。人物采用确定性 Provider；外部模型 E2E 为 `REQUIRED_IF_AVAILABLE`，未配置、未伪造验证。

## 启动

准备独立数据库（同一路径可重复执行）：

```bash
cd /home/ubuntu/corerp-console/backend
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-rp1.db -action rp-travel-prepare
```

为本地服务设置凭证映射；将示例占位符替换成你自己的随机凭证，勿提交环境配置：

```bash
export CORERP_AUTH_TOKENS_JSON='{"<替换为自己的本地随机凭证>":"principal_m2_rp_player"}'
export CORERP_CURSOR_SECRET="$(openssl rand -hex 32)"
/usr/local/go/bin/go run ./cmd/corerp-server -db /tmp/corerp-rp1.db -listen 127.0.0.1:8080
```

另开终端：

```bash
cd /home/ubuntu/corerp-console
npm ci
npm run dev -- --host 127.0.0.1
```

打开 `http://127.0.0.1:5173`，输入映射中的玩家凭证。Vite 将 `/api` 代理到本机 8080。构建部署若以后需要，必须由同源反向代理提供 `/api`；静态 `dist` 自身没有后端代理。本阶段未部署。

## 交互与恢复

- 输入框中的自然语言会作为角色**说出的话**提交，不是任意动作解析器。移动、等待通过明确入口完成。
- 可达地点、当前时间、在场人物以及最近 50 段对话/行动从服务端受限 Observation 返回。旧历史仍在数据库中，本版未提供翻页。
- 浏览器 localStorage 保存会话 ID、打开会话的幂等键以及未完成行动的原请求；凭证仅保留在页面内存，刷新/重开时重新输入。此浏览器书签用于恢复，不是世界权威；建议使用自己的浏览器配置文件。
- 断网、丢失响应或服务中断后，点击“继续未完成的行动”。完成的发言、NPC 效果、移动、等待均按原键返回结果，不重复产生事实。等待预算不足时继续同一行动。
- 关闭服务再以**同一数据库路径**启动，打开同一浏览器并输入凭证即可继续。不要为了恢复而重新创建世界。清除浏览器存储会丢失此 UI 的会话书签，但不会删除数据库世界；跨设备会话选择器不在首版范围。
- 单玩家切片中不要同时用多个标签页或外部管理命令驱动同一分支；本版没有多人/并发会话产品语义。

## 验证

```bash
npm run build
npm run verify:rp1-play
```

E2E 需要 `/usr/local/go/bin/go`、`sqlite3`、项目 Playwright 及 Chromium，并使用空闲的 8080/4178 端口。缺浏览器时可运行项目的 `npx playwright install chromium`。脚本在系统临时目录构建两个 Go 二进制、创建独立 SQLite、生成仅进程内凭证、启动实际 HTTP/Vite/浏览器，并在结束时停止所启动的进程。数据库及截图留在输出的临时目录供检查，不加入 Git。

测试覆盖实际听者认知、拒绝、工作日程影响决策、移动/异地排除、跨越工作/午餐节点的等待、丢失已提交响应、关闭并重启浏览器/服务、无重复事实、继续同一世界及手机布局。它不调用外部 LLM。
