# FlowSpace 任务完成联动

日语端的学习计划任务完成后，通过后台可靠投递更新 FlowSpace 中已关联的当天执行实例。手动打卡和完整听力训练共用同一状态写入路径；播放音频或未完成的训练不触发。当前不反向创建练习记录，也不自动重新打开 FlowSpace 已完成任务。

## 配置与使用

1. 在服务端 `.env.local` 配置 `FLOWSPACE_BASE_URL`、`FLOWSPACE_SOURCE_INSTANCE_ID` 和 `FLOWSPACE_INTEGRATION_MASTER_KEY`。前者为 FlowSpace 服务的固定入口（可含部署子路径）；生产必须 HTTPS，本机测试可用 loopback HTTP。实例 ID 在重启和发布间保持不变，测试/生产不同。
2. 用 `openssl rand -hex 32` 生成主密钥，把结果放入受保护的环境或本地配置，不提交到版本库。该密钥用于 AES-GCM 加密数据库中的连接凭据，重启时必须保持一致；更换主密钥前应断开连接并重新配对。
3. 按现有 `make start-backend` 启动流程重启服务，启动会自动应用新增 PostgreSQL 迁移。未设置入口时功能显示为未配置，不影响学习。
4. 在 FlowSpace 当前工作区生成一次性配对码，在日语项目“设置 → FlowSpace 任务联动”输入。浏览器只接触一次性配对码，不接收长期签名凭据。
5. 在 FlowSpace 选择来源学习任务，并关联对应的重复任务。应先关联再完成学习；初次连接不会把历史打卡批量补发。
6. 在日语项目的当天学习计划完成任务。设置卡可查看待投递、已送达、远端处理结果及错误；投递失败会自动重试，配置错误修复后可点重试。

日语端断开连接会停止并取消本地待投递任务；如果还要撤销 FlowSpace 服务端凭据，应同时在 FlowSpace 工作区撤销对应连接。已进入远端处理的事件以 FlowSpace 的处理回执为准。

## 状态与可靠性

- 完成状态、递增版本和 outbox 在同一个 PostgreSQL 事务中提交；听力训练失败或事务回滚不会发送完成事件。
- 计划有稳定 `plan_id`；删除重建换代，跨代际目录修订不会归零。撤销保留状态版本，防止旧完成事件覆盖更新状态。
- 同一计划代际内，任务 ID 不能在删除后复用，也不能改为其他学习路径；应为新的学习内容使用新 ID。改标题、时间或学习日不会改变身份，关联规则变更由 FlowSpace 重新确认。
- 后台 worker 使用租约和数据库队列，重启后继续。每次 HTTP 重试使用新的 nonce 和签名时间，事件 ID 与正文不变。
- HTTP 202 只表示 FlowSpace 已持久收件；收到 `applied` 或 `already_done` 处理回执后才表示目标完成。远端需处理冲突不会冒充成功。
- 上述目标完成判断适用于来源 `completed=true`。撤销事件 `completed=false` 的 `applied` 仅表示来源状态版本已记录，不会重新打开目标或证明目标已完成。
- `needs_attention` 回执每分钟在后台重新查询；在 FlowSpace 修复并重试后，日语端会更新结果。日语端“重试”也可立即安排查询已送达事件的处理回执，不会再次 POST 同一完成事件。
- 连接凭据和事件不含练习答案或录音。HTTPS、HMAC、独立连接身份与工作区绑定共同保护传输和目标选择。

## 接口与验证

接口均位于当前用户 JWT 认证下：`GET/DELETE /api/v1/integrations/flowspace`、`POST .../pair`、`POST .../retry`。来源用户身份取自服务端登录上下文，不能从请求正文指定。

通过 `make test` 在独立 PostgreSQL 测试库运行后端测试；`internal/module/studyplan` 覆盖完成/撤销/重复/回滚与计划代际，`internal/module/flowspace` 覆盖配对、凭据加密、账号隔离、签名、失败重试和回执。跨项目真实 HTTP 验证的运行方式见 FlowSpace 仓库对应测试与使用文档。
