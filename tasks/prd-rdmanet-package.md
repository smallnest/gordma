# PRD: gordma/rdmanet — net 风格的高层 RDMA 连接库

> 本文档取代 `tasks/prd-conn-package.md`（包名由 `conn` 改为 `rdmanet`；接口由严格
> `net.Conn` 改为形似 API；核心语义改为消息式 + 流式适配；单边操作移出范围）。

## Introduction

gordma 目前暴露的是 rdma-core 的 verbs 对象模型（Device/Context/PD/MR/CQ/QP），使用者必须自己注册内存、提交 Work Request、轮询 CQ、管理 QP 状态机，门槛很高。本特性新增子包 `github.com/smallnest/gordma/rdmanet`，提供类似标准库 `net` 风格的高层 API：

- **消息语义为基础**：RC 连接上的 `SendMsg`/`RecvMsg`，每次收发一条完整消息，贴近 RDMA 的消息型传输本性；
- **流式适配在其上**：`Conn` 同时提供 `Read([]byte)`/`Write([]byte)`，满足 `io.Reader`/`io.Writer`，方便接入现有流式代码；
- **UD 数据报**：类似 `net.UDPConn` 的 `PacketConn`（`ReadFrom`/`WriteTo`）；
- **批量收发与零拷贝**：面向性能用户的进阶接口。

API 形似 `net` 包（Dial/Listen/Accept/Close/Read/Write）但**不强求**实现 `net.Conn`/`net.Listener` 标准接口，Deadline 系列首版不提供。底层细节（MR 注册、WR 提交、CQ 轮询、流控）全部隐藏。现有根包 API 与 `handshake`、`perftest`、`cmd/` 完全不受影响。

## Goals

- 用户用 `rdmanet.Dial` / `rdmanet.Listen` + `SendMsg`/`RecvMsg` 或 `Read`/`Write` 即可完成 RDMA 通信，无需接触 QP/CQ/MR
- 消息语义是第一公民：一次 `SendMsg` 对应一条消息，`RecvMsg` 保留消息边界
- 流式 `Read`/`Write` 作为消息层之上的适配，满足 `io.Reader`/`io.Writer`
- UD 提供 `PacketConn` 数据报接口，保留消息边界
- 提供批量收发 API（一次提交多条消息，摊薄每次 post 的开销）
- 提供零拷贝高级接口（借出预注册 buffer），绕过 bounce-buffer 拷贝
- 连接建立支持 rdma_cm（默认）与 TCP out-of-band 握手两种方式
- CQ 处理模型支持 busy-poll 与 CompChannel 事件两种模式，可通过 Option 选择
- 每个特性在 `examples/` 下有独立目录的可运行示例
- 与根包同样的跨平台策略：非 Linux/无 cgo 平台编译通过，运行返回 `ErrNotSupported`

## User Stories

### US-001: rdmanet 包骨架与配置选项
**Description:** 作为开发者，我需要 rdmanet 包的基础结构和统一的配置入口，后续故事都在其上构建。

**Acceptance Criteria:**
- [ ] 新建 `rdmanet/` 子包，包文档说明定位与根包的关系
- [ ] 提供 `Option` 函数式选项：`WithDevice(name)`、`WithPort(n)`、`WithGIDIndex(n)`、`WithQueueDepth(n)`、`WithBufferSize(n)`、`WithHandshake()`（选择 TCP 握手而非 rdma_cm）、`WithPollMode(mode)`（busy-poll / 事件驱动，见 US-011）
- [ ] stub 构建（darwin/windows/CGO_ENABLED=0）编译通过，所有入口函数返回 `gordma.ErrNotSupported`
- [ ] `go vet ./...` 通过；新增 stub 一致性检查覆盖 rdmanet 包导出 API

### US-002: rdma_cm 方式 Dial/Listen（默认）
**Description:** 作为开发者，我想用 `rdmanet.Listen("0.0.0.0:9000")` / `rdmanet.Dial("10.0.0.1:9000")` 建立 RDMA 连接，像用 net 包一样。

**Acceptance Criteria:**
- [ ] `Listen(addr string, opts ...Option) (*Listener, error)`，`Listener` 提供 `Accept() (*Conn, error)` / `Close()` / `Addr()`
- [ ] `Dial(addr string, opts ...Option) (*Conn, error)` 与 `DialTimeout`，底层复用根包 rdma_cm 路径
- [ ] `Conn.LocalAddr()`/`RemoteAddr()` 返回有意义的地址（IP:port 语义）
- [ ] 单元测试覆盖参数校验与 stub 行为；硬件相关测试隔离

### US-003: TCP 握手方式建连
**Description:** 作为开发者，在没有 rdma_cm 或需要 perftest 兼容的环境，我想通过 TCP out-of-band 握手建立同样的 `Conn`。

**Acceptance Criteria:**
- [ ] `WithHandshake()` 选项使 Dial/Listen 走 `handshake` 包交换 QPN/PSN/GID/RKey，内部完成 INIT→RTR→RTS
- [ ] 两种建连方式返回的 `Conn` 行为一致（同一类型，调用方无感知）
- [ ] handshake 路径的信息交换逻辑有纯 Go 单元测试（无需硬件）

### US-004: RC 消息语义 SendMsg/RecvMsg
**Description:** 作为开发者，我想一次收发一条完整消息，不关心 MR/WR/CQ，且消息边界被保留。

**Acceptance Criteria:**
- [ ] `Conn.SendMsg(p []byte) error`：阻塞至消息发出（拿到发送完成）或出错
- [ ] `Conn.RecvMsg() ([]byte, error)` 与 `Conn.RecvMsgBuf(p []byte) (int, error)`：阻塞至收到一条完整消息；缓冲不足返回明确错误而非截断
- [ ] 内部维护预注册的环形 bounce buffer，发送侧自动把大于单条上限的消息分片、接收侧重组（如 16MB 消息能正确收发且边界保留）
- [ ] 实现基于信用（credit）的流控，慢接收方不会导致 RNR 错误或数据丢失
- [ ] 并发模型：一个 goroutine 收 + 一个 goroutine 发安全；并发多收/多发串行化或返回明确错误
- [ ] `go test -race` 通过（stub + 模拟路径）

### US-005: 流式 Read/Write 适配
**Description:** 作为开发者，我已有依赖 `io.Reader`/`io.Writer` 的代码，想直接把 `Conn` 当字节流用。

**Acceptance Criteria:**
- [ ] `Conn.Read(p []byte) (int, error)` / `Conn.Write(p []byte) (int, error)` 满足 `io.Reader`/`io.Writer` 语义：Write 全量写出或返回错误；Read 阻塞至至少 1 字节
- [ ] 流式接口构建在消息层之上：Read 可跨消息边界连续取字节（消息内容拼接为流）
- [ ] 编译期断言 `var _ io.ReadWriteCloser = (*Conn)(nil)`
- [ ] 文档明确：同一连接上混用 SendMsg/RecvMsg 与 Read/Write 的约束（允许并定义语义，或禁止并返回错误）
- [ ] 单元测试覆盖流式语义（含跨消息读取）与 stub 行为

### US-006: Close 与资源回收
**Description:** 作为开发者，我希望 `Close()` 一次性释放 QP/CQ/MR/PD 等全部资源，且行为幂等。

**Acceptance Criteria:**
- [ ] `Close` 幂等；Close 后所有收发方法返回明确的 closed 错误
- [ ] Close 能唤醒阻塞中的 SendMsg/RecvMsg/Read/Write
- [ ] 对端关闭后本端 RecvMsg/Read 返回 `io.EOF`
- [ ] 无资源泄漏：内部对象的释放顺序正确（QP→CQ→MR→PD）

### US-007: UD PacketConn 与 Addr 格式
**Description:** 作为开发者，我想用数据报语义在 UD QP 上收发消息，类似 `net.UDPConn`。

**Acceptance Criteria:**
- [ ] `ListenPacket(addr string, opts ...Option) (*PacketConn, error)` 提供 `ReadFrom(p []byte) (int, *Addr, error)` / `WriteTo(p []byte, to *Addr) (int, error)` / `Close()` / `LocalAddr()`
- [ ] 定义 `Addr` 类型封装 UD 寻址信息（GID/QPN/QKey）
- [ ] `Addr.String()` 采用 `gid%qpn` 格式（如 `fe80::1%0x12ab`；QKey 非默认值时附加 `#qkey`），`ResolveAddr(s string)` 能解析该格式并与 `String()` 互逆（round-trip）
- [ ] 保留消息边界；超过 UD MTU 上限的 WriteTo 返回明确错误（不静默截断）
- [ ] 内部管理 AH 缓存（按目的地址复用）
- [ ] 单元测试覆盖地址解析（含非法输入）与 stub 行为

### US-008: 批量收发 API
**Description:** 作为追求吞吐的开发者，我想一次提交/收割多条消息，摊薄每次 post/poll 的固定开销。

**Acceptance Criteria:**
- [ ] `Conn.SendBatch(msgs [][]byte) error`（或等价 API）：一次 post 多条消息，全部完成后返回
- [ ] `Conn.RecvBatch(max int) ([][]byte, error)`（或等价 API）：一次最多收割 max 条已到达消息，至少返回 1 条（无消息则阻塞）
- [ ] `PacketConn` 提供对应的批量 `WriteToBatch`/`ReadFromBatch`（或等价 API）
- [ ] 批量与单条 API 可混用，语义一致
- [ ] 单元测试覆盖批量参数校验、空批次、stub 行为

### US-009: 零拷贝高级接口
**Description:** 作为追求性能的开发者，我想借出预注册的 buffer 直接收发，避免 bounce buffer 拷贝。

**Acceptance Criteria:**
- [ ] `Conn.AllocBuffer(size int) (*Buffer, error)` 返回预注册（pinned）buffer，`Buffer.Bytes()` 暴露内容
- [ ] `Conn.SendBuffer(b *Buffer) error` / `Conn.RecvBuffer() (*Buffer, error)`（或等价借还模型）零拷贝收发
- [ ] Buffer 生命周期文档明确：发出后何时可复用、Close 后 buffer 失效
- [ ] 与普通 SendMsg/RecvMsg 可混用或文档明确不可混用的约束
- [ ] 单元测试覆盖借还状态机与误用（如重复释放）

### US-010: PacketConn 带外地址注册机制
**Description:** 作为开发者，我需要一个轻量的带外机制来发现/交换 UD 对端地址（GID/QPN/QKey），不必自己实现交换协议。

**Acceptance Criteria:**
- [ ] 提供轻量注册器：`rdmanet.NewRegistry(addr string)` 启动一个 TCP 注册服务（风格与 `handshake` 包一致，纯 Go、可独立单测）
- [ ] `PacketConn.Register(registryAddr, name string) error` 向注册器登记本端 `Addr`
- [ ] `rdmanet.LookupAddr(registryAddr, name string) (*Addr, error)` 按名称查询对端 `Addr`
- [ ] 注册器为可选组件：用户也可自行带外分发 `Addr.String()` 后用 `ResolveAddr` 构造
- [ ] 注册/查询协议有纯 Go 单元测试（无需硬件）

### US-011: CQ poller 双模式（busy-poll / 事件驱动）
**Description:** 作为开发者，我想根据延迟与 CPU 占用的取舍选择 CQ 处理模式：低延迟场景用 busy-poll，省 CPU 场景用 CompChannel 事件驱动。

**Acceptance Criteria:**
- [ ] `WithPollMode(PollBusy | PollEvent)` 选项对 `Conn` 与 `PacketConn` 均生效，默认值在 godoc 中明确（建议默认 `PollEvent`）
- [ ] `PollBusy`：专用 goroutine 忙轮询 CQ（可结合 `runtime.Gosched` 让出）
- [ ] `PollEvent`：基于 CompChannel（`ibv_get_cq_event`）阻塞等待，唤醒后批量收割
- [ ] 两种模式下功能行为一致（仅性能特征不同），共享同一套完成分发逻辑
- [ ] 单元测试覆盖模式选项的解析与 stub 行为；硬件性能对比纳入 US-012 工具的 flag（如 `--poll=busy|event`）

### US-012: 示例与基准工具
**Description:** 作为用户，我想看到 rdmanet 包的端到端示例，并能验证其性能开销相对原始 verbs 路径可接受。

**Acceptance Criteria:**
- [ ] 新增 `cmd/go_rdmanet_bw` 与 `cmd/go_rdmanet_lat`：基于 rdmanet 包的带宽/延迟工具，支持 server/client 模式
- [ ] 工具支持 `-d` 设备、`-x` GID index、`-s` 大小、`-n` 次数、`--poll=busy|event` 等与现有 cmd 一致的常用 flag
- [ ] 现有 `cmd/` 六个工具不做改动（不迁移、不破坏）
- [ ] README 新增 rdmanet 包 Quick Start 小节

### US-013: examples 目录与分特性示例
**Description:** 作为新用户，我想在 `examples/` 下按特性找到独立、可直接运行的最小示例，快速上手每个能力。

**Acceptance Criteria:**
- [ ] 新建 `examples/` 目录，每个特性一个子目录，各含独立 `main.go`（server/client 通过 flag 或参数区分）与简短 `README.md`（运行方法、预期输出）
- [ ] 至少覆盖以下子目录：
  - `examples/echo-msg/` — RC 消息语义 SendMsg/RecvMsg 最小 echo（US-002/US-004）
  - `examples/echo-stream/` — 流式 Read/Write 适配（US-005）
  - `examples/handshake-dial/` — TCP 握手方式建连（US-003）
  - `examples/packet/` — UD PacketConn 收发（US-007）
  - `examples/batch/` — 批量收发（US-008）
  - `examples/zerocopy/` — 零拷贝 Buffer 借还（US-009）
  - `examples/registry/` — 带外地址注册与发现（US-010）
  - `examples/pollmode/` — busy-poll 与事件模式切换（US-011）
- [ ] 所有示例在 stub 平台能编译（`go build ./examples/...` 纳入 CI），运行时打印 `ErrNotSupported` 友好提示
- [ ] echo 示例核心代码 < 30 行（不含 import/错误处理样板）

### US-014: 文档与 CI
**Description:** 作为维护者，我需要 rdmanet 包有完整 godoc 和 CI 覆盖。

**Acceptance Criteria:**
- [ ] rdmanet 包 doc.go 说明：消息语义与流式适配的关系、两种建连方式、两种 poll 模式、批量与零拷贝接口的适用场景、平台支持
- [ ] CI 覆盖 rdmanet 包与 examples：vet、cgo 与 stub 双构建、`go test -race`、darwin/windows 交叉编译
- [ ] 所有导出符号有 godoc 注释

## Functional Requirements

- FR-1: 系统必须在 `rdmanet` 子包提供 `Dial`/`DialTimeout`/`Listen`，返回 `*Conn`/`*Listener`（形似 net 包，不要求实现 `net.Conn`/`net.Listener` 接口）
- FR-2: 系统必须默认使用 rdma_cm 建连，并通过 `WithHandshake()` 选项切换为 TCP out-of-band 握手
- FR-3: 系统必须提供消息语义 `SendMsg`/`RecvMsg`，保留消息边界，自动完成内存注册/复用、分片与 WR 提交
- FR-4: 系统必须在消息层之上提供流式 `Read`/`Write`，满足 `io.Reader`/`io.Writer` 语义
- FR-5: 系统必须实现基于 credit 的接收流控，防止接收方 buffer 耗尽
- FR-6: `Close` 必须幂等、唤醒阻塞中的收发调用，且按依赖顺序释放全部底层资源
- FR-7: 系统必须提供 `ListenPacket` 返回 UD 数据报 `PacketConn`（`ReadFrom`/`WriteTo`）
- FR-8: UD `Addr.String()` 必须采用 `gid%qpn` 格式（QKey 非默认值时附加 `#qkey`），且 `ResolveAddr` 与之互逆
- FR-9: UD `WriteTo` 超过 MTU 限制时必须返回错误而非截断
- FR-10: 系统必须提供批量收发 API（`SendBatch`/`RecvBatch` 及 PacketConn 对应物）
- FR-11: 系统必须提供 `AllocBuffer`/`SendBuffer`/`RecvBuffer` 零拷贝接口
- FR-12: 系统必须通过 `WithPollMode` 提供 busy-poll 与 CompChannel 事件驱动两种 CQ 处理模式
- FR-13: 系统必须提供轻量带外注册器（`NewRegistry`/`Register`/`LookupAddr`）用于 UD 地址发现，且为可选组件
- FR-14: 每个特性必须在 `examples/` 下有独立子目录的可运行示例，并纳入 CI 构建
- FR-15: 非 Linux 或无 cgo 构建必须编译通过且运行时返回 `gordma.ErrNotSupported`
- FR-16: rdmanet 包不得修改根包、`handshake`、`perftest`、`cmd/` 的现有导出 API 与行为

## Non-Goals

- 不实现 `net.Conn`/`net.Listener`/`net.PacketConn` 标准接口契约；Deadline（`SetDeadline` 系列）首版不提供
- 不提供单边操作（RDMA Write/Read 到远端内存窗口、`WriteAt`/`ReadAt`）的高层封装 — 需要时用根包 verbs API
- 不实现 RDMA 之上的 RPC/序列化协议（只做消息/字节流/数据报）
- 不做 TCP 自动回退（RDMA 不可用时不降级为 TCP）
- 不支持 TLS/加密
- 不迁移/改写现有六个 `cmd/` perftest 工具
- 不支持 Windows/macOS 的真实 RDMA（仅 stub）
- 不实现连接池、重连、多路复用等上层功能
- 注册器不做高可用/持久化（仅内存表，进程退出即失效）

## Technical Considerations

- RC 是消息型传输，SendMsg/RecvMsg 与之天然对应；大消息需内部分帧（头部携带长度/分片序号 + credit 回执），复用 SEND/RECV 通道
- 流式 Read/Write 是消息层之上的薄适配：维护一个"当前未读完的消息"游标即可，无须独立数据通道
- CQ 处理模型：busy-poll 与 CompChannel 事件两种模式均实现（US-011），完成分发逻辑共享；默认事件模式省 CPU
- bounce buffer 环形池大小 = QueueDepth × BufferSize，需文档化内存占用
- 批量 API 直接映射 verbs 的链式 post（`ibv_post_send` 的 wr 链表）与一次 poll 多个 WC，开销摊薄效果可在 bw 工具中量化
- UD 注册器协议建议沿用 `handshake` 包的 line-delimited JSON 风格，纯 Go 实现便于单测
- stub 一致性：沿用根包 `stub_consistency_test.go` 的模式
- examples 子目录均为独立 main 包，避免相互依赖；CI 用 `go build ./examples/...` 验证

## Success Metrics

- echo 示例代码行数 < 30 行（不含 import/错误处理样板），对比现在 cmd 工具数百行
- `var _ io.ReadWriteCloser = (*Conn)(nil)` 编译期成立，现有依赖 io.Reader/io.Writer 的代码可零修改接入
- 零拷贝路径带宽达到 go_send_bw（原始 verbs）的 90% 以上；bounce 路径有量化数据；批量 API 相对单条有可测的吞吐提升
- CI 全绿（Linux cgo/nocgo + darwin/windows 交叉编译 + race + examples 构建）

## Open Questions

（无 — 历史决议已合入：UD Addr 采用 `gid%qpn` 格式（US-007/FR-8）；CQ poller 双模式做成 Option（US-011/FR-12）；PacketConn 提供轻量带外注册机制（US-010/FR-13）；每特性独立 examples 子目录（US-013/FR-14）。）
