# Eino CheckPointStore 调研

调研日期：2026-09-22

## 结论摘要

项目当前 `go.mod` 固定使用 `github.com/cloudwego/eino v0.9.19`。

对当前项目 `BaseAgent` 的实际结论：

1. `BaseAgent` 仅把 `adk.CheckPointStore` 注入 `adk.Runner`，并在 `runner.Run` 时传入 `adk.WithCheckPointID(checkpointID)`。
2. 当前代码没有调用 `runner.Resume` 或 `runner.ResumeWithParams`。因此即使 Store 中存在 checkpoint，当前 `Generate`/`GenerateStream` 也不会主动从 checkpoint 恢复；它们仍然调用新的 `Run`。
3. checkpoint 不是每个 agent turn 自动保存。ADK v0.9.19 的 Runner 主要在 agent interrupt，或被 Eino 识别为可持久化 interrupt 的 cancel 路径上保存。
4. 保存的是可重建执行现场，不是单纯的聊天历史：包括运行上下文、会话中的事件/值、agent 运行路径、interrupt 地址与状态，以及 interrupt 附带的数据。具体内容随 agent 和 interrupt 类型而变。
5. “内部思考”不会因为 CheckPointStore 自动获得完整模型 CoT。只有已经进入 Eino 可序列化 checkpoint 状态的事件、消息、工具/agent 状态或 interrupt 数据才会保存；模型服务端未返回、未暴露或未写入这些字段的隐藏推理不会被 checkpoint 捕获。项目当前使用普通 `schema.Message` 结果消费，也没有看到显式保存隐藏思考链的代码。
6. 进程重启后理论上可以恢复，但前提是 Store 使用持久化实现、checkpoint 已成功写入、恢复时使用相同 checkpoint ID/兼容的 agent 与可序列化类型，并显式调用 `Resume`/`ResumeWithParams`。内存 Store 或进程内对象不能跨重启恢复。
7. `context.Cancel` 并不等于必然可恢复：v0.9.19 对带 interrupt signal 的 cancel 会尝试保存 checkpoint；普通取消、在保存前进程崩溃、Store 写入失败或底层网络断开导致 `Set` 失败时，不能保证有可用 checkpoint。
8. 网络断开分两类：如果只是调用方与服务端的连接断开，而 Go 进程仍收到取消并成功执行 checkpoint `Set`，之后可恢复；如果网络断开使上下文取消、Store 不可达或进程退出，则恢复取决于 checkpoint 是否已成功持久化。CheckPointStore 本身只定义 `Get`/`Set`，重试、超时、持久化和一致性由具体实现负责。

## 项目当前用法

文件：`internal/ai/agent/base_agent.go`

- `RunnerConfig.CheckPointStore = a.checkpointStore`
- Store 非空时，从 `ctx.Value("checkpointID")` 读取 ID；没有 ID 会直接返回错误。
- 随后调用：

```go
runner.Run(ctx, messages, adk.WithCheckPointID(checkpointID))
```

- `GenerateStream` 也是同样路径。
- 两个方法都遍历 Run 返回的事件，但没有使用 `runner.Resume` 或 `runner.ResumeWithParams`。

因此，项目当前的 checkpoint ID 主要让 Runner 在运行期间具备“写入 checkpoint”的条件；它不是当前代码中的“自动恢复开关”。要恢复中断执行，需要在后续请求中构造 Runner，并调用 Eino 的恢复 API。

## 官方/源码定义

### CheckPointStore 接口

Eino v0.9.19 的内部 core 定义：

```go
type CheckPointStore interface {
    Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
    Set(ctx context.Context, checkPointID string, checkPoint []byte) error
}
```

另有可选的 `CheckPointDeleter`。接口没有规定存储介质、TTL、重试、版本控制或跨进程语义。

来源：

- 本地模块源码：`%GOMODCACHE%/github.com/cloudwego/eino@v0.9.19/internal/core/interrupt.go:27`
- 项目版本：`go.mod:8`

### ADK checkpoint 根载荷

`adk/interrupt.go` 中的 `serialization` 结构包含：

- `RunCtx`：运行上下文；
- `Info`：interrupt 信息；
- `InterruptID2Address`、`InterruptID2State`：interrupt 地址和状态；
- `EnableStreaming`：原执行是否为 streaming；
- 兼容旧版本 checkpoint 的恢复信息。

保存时使用 Go `gob` 编码，然后调用 `store.Set(ctx, key, bytes)`；读取时调用 `store.Get`，再解码并重新填充 interrupt state。

来源：

- `adk/interrupt.go:208-240`
- `adk/interrupt.go:290-317`

### 运行上下文保存了什么

`adk/runctx.go` 中的 `runContext` 包含：

- 根输入和 agent 运行路径；
- agentic root input；
- `Session`。

`runSession` 包含：

- `Values`；
- 已产生的 agent events；
- lane events；
- typed events。

这说明 checkpoint 的粒度是“可恢复执行上下文/会话快照”，而不是只保存最后一条回答。

来源：

- `adk/runctx.go:33-54`
- `adk/runctx.go:346-365`

## 保存粒度：是否每个 agent turn？

不是普通意义上的“每个 turn 保存一次”。

v0.9.19 的 Runner 事件处理逻辑显示：

- 检测到 `CancelError` 且带有 `interruptSignal`，并且存在 checkpoint ID 时，调用 `runnerSaveCheckPointImpl`；
- 检测到 agent 产生内部 interrupt action 时，调用 `runnerSaveCheckPointImpl`；
- 普通成功事件、普通模型响应、普通工具调用不会因为每个 turn 自动调用 CheckPointStore。

来源：

- `adk/runner.go:typedRunnerHandleIterImpl`
- `adk/interrupt.go:290-317`

因此 checkpoint 更接近“中断点/取消恢复点”的快照。若业务需要每轮持久化对话历史，应单独保存业务消息或使用明确的 checkpoint 写入策略，不能仅依据 CheckPointStore 接口假设每轮都有记录。

## 是否包含内部思考？

不能把 Eino checkpoint 等同于完整内部思考记录。

源码明确保存的是可序列化的运行上下文、事件、消息相关状态和 interrupt 数据。模型供应商没有返回给应用的隐藏推理、模型内部 token 级思考或未被 Eino 状态承载的内容，不会因 checkpoint 自动出现。

如果某个模型/agent 将可见的 reasoning 字段、工具调用参数、事件或自定义 interrupt 数据放入 checkpoint schema，它们可能被保存；这取决于具体 agent、模型适配器和 serializer，而不是 `CheckPointStore` 接口本身。

当前项目的 `BaseAgent` 最终只提取 `MessageOutput.GetMessage()` 作为结果，没有显式把隐藏思考写入 Store。因此不能认为项目当前 checkpoint 包含完整 CoT。

项目来源：

- `internal/ai/agent/base_agent.go:91-123`
- `internal/ai/agent/base_agent.go:160-180`

## 取消、重启、网络断开后的恢复

### Context cancel

v0.9.19 对带 interrupt signal 的 cancel 会尝试保存 checkpoint；保存失败会向事件流发送 `failed to save checkpoint on cancel` 错误。没有 interrupt signal 的普通取消不保证产生 checkpoint。

此外，保存调用使用传入的 `ctx`。如果该 context 已经被取消，具体 Store 可能立即拒绝 `Set`。因此“收到取消”与“checkpoint 已成功落库”是两个不同条件。

来源：

- `adk/runner.go:typedRunnerHandleIterImpl`
- `adk/interrupt.go:313-317`

### 进程重启

可以恢复，但不是无条件保证：

- Store 必须是跨进程持久化实现，而非内存 map；
- `Set` 必须在进程退出前成功完成；
- 新进程必须使用相同 checkpoint ID；
- agent、注册的序列化类型和版本必须兼容；
- 新进程必须调用 `Resume` 或 `ResumeWithParams`。

Eino 提供的 `Resume` 明确表示“从 checkpoint 继续中断执行”，并通过 `Get` 读取 checkpoint。

来源：

- `adk/runner.go:117-153`
- `adk/interrupt.go:223-240`
- `internal/core/interrupt.go:27-33`

### 网络断开

CheckPointStore 不定义网络容错。若 Redis/数据库等远程 Store 的 `Get` 或 `Set` 失败，Eino 返回相应错误；是否重试、是否有事务/幂等写入、是否保留旧 checkpoint，均由 Store 实现决定。

调用方网络断开本身也不等于 checkpoint 成功：只有 Eino 进程继续运行、检测到可持久化取消/interrupt，并成功执行 Store 的 `Set`，后续才可能恢复。

## 官方文档

- Eino 官方文档入口（Checkpoint & interrupt/resume）：<https://www.cloudwego.io/docs/eino/core_modules/chain_and_graph_orchestration/checkpoint_interrupt/>
- Eino 官方仓库 README 的 ADK 概述：<https://github.com/cloudwego/eino>
- 与项目实际依赖完全对应的源码版本：`github.com/cloudwego/eino v0.9.19`，见项目 `go.mod` 和本机 Go module cache。

## 最终判断

对当前项目不能作出“每个 agent turn 都已可靠保存、断线后自动续跑”的判断。准确表述是：项目给 Eino Runner 配置了 checkpoint store 和 checkpoint ID，使中断/特定取消路径具备保存能力；但当前 `BaseAgent` 没有实现恢复调用，也没有证据表明它保存完整隐藏思考或按每轮保存。恢复能力最终取决于持久化 Store、保存成功时机、序列化兼容性以及后续请求是否显式调用 `Resume`。
