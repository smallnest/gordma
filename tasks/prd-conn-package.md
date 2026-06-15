# PRD: gordma/conn — net 风格的高层 RDMA 连接库

## Introduction

gordma 目前暴露的是 rdma-core 的 verbs 对象模型（Device/Context/PD/MR/CQ/QP），使用者必须自己注册内存、提交 Work Request、轮询 CQ、管理 QP 状态机，门槛很高。本特性新增子包 `github.com/smallnest/gordma/conn`，提供类似标准库 `net` 的高层 API：RC 上的流式 `Conn`（实现 `net.Conn`）、UD 上的 `PacketConn`（实现 `net.PacketConn` 风格接口）、以及单边 RDMA Write/Read 的 `WriteAt`/`ReadAt` 风格接口。底层细节（MR 注册、WR 提交、CQ 轮询、流控）全部隐藏；同时为追求性能的用户提供可选的零拷贝高级接口。现有根包 API 与 `handshake`、`perftest`、`cmd/` 完全不受影响。

## Goals

- 用户用 `conn.Dial` / `conn.Listen` + `Read`/`Write` 即可完成 RDMA 通信，无需接触 QP/CQ/MR
- `conn.Conn` 完整实现 `net.Conn` 接口（含 SetDeadline 系列），可直接传给依赖 `net.Conn` 的现有代码
- UD 提供 `net.PacketConn` 风格的数据报接口，保留消息边界
- 提供单边操作高层 API：向对端暴露内存窗口后可 `WriteAt`/`ReadAt`
- 提供零拷贝高级接口（借出预注册 buffer），绕过 bounce-buffer 拷贝
- 连接建立支持 rdma_cm（默认）与 TCP out-of-band 握手两种方式
- CQ 处理模型支持 busy-poll 与 CompChannel 事件两种模式，可通过 Option 选择
- 每个特性在 `examples/` 下有独立目录的可运行示例
- 与根包同样的跨平台策略：非 Linux/无 cgo 平台编译通过，运行返回 `ErrNotSupported`

## User Stories

### US-001: conn 包骨架与配置选项
**Description:** 作为开发者，我需要 conn 包的基础结构和统一的配置入口，后续故事都在其上构建。

**Acceptance Criteria:**
- [ ] 新建 `conn/` 子包，包文档说明定位与根包的关系
- [ ] 提供 `Option` 函数式选项：`WithDevice(name)`、`WithPort(n)`、`WithGIDIndex(n)`、`WithQueueDepth(n)`、`WithBufferSize(n)`、`WithHandshake()`（选择 TCP 握手而非 rdma_cm）、`WithPollMode(mode)`（busy-poll / 事件驱动，见 US-012）
- [ ] stub 构建（darwin/windows/CGO_ENABLED=0）编译通过，所有入口函数返回 `gordma.ErrNotSupported`
- [ ] `go vet ./...` 通过；新增 stub 一致性检查覆盖 conn 包导出 API

### US-002: rdma_cm 方式 Dial/Listen（默认）
**Description:** 作为开发者，我想用 `conn.Listen("0.0.0.0:9000")` / `conn.Dial("10.0.0.1:9000")` 建立 RDMA 连接，像用 net 包一样。

**Acceptance Criteria:**
- [ ] `Listen(addr string, opts ...Option) (*Listener, error)`，`Listener` 实现 `net.Listener`（`Accept` 返回 `net.Conn`）
- [ ] `Dial(addr string, opts ...Option) (*Conn, error)` 与 `DialTimeout`，底层复用根包 rdma_cm 路径
- [ ] `LocalAddr()`/`RemoteAddr()` 返回有意义的地址（IP:port 语义）
- [ ] 单元测试覆盖参数校验与 stub 行为；硬件相关测试隔离

### US-003: TCP 握手方式建连
**Description:** 作为开发者，在没有 rdma_cm 或需要 perftest 兼容的环境，我想通过 TCP out-of-band 握手建立同样的 `Conn`。

**Acceptance Criteria:**
- [ ] `WithHandshake()` 选项使 Dial/Listen 走 `handshake` 包交换 QPN/PSN/GID/RKey，内部完成 INIT→RTR→RTS
- [ ] 两种建连方式返回的 `Conn` 行为一致（同一类型，调用方无感知）
- [ ] handshake 路径的信息交换逻辑有纯 Go 单元测试（无需硬件）

### US-004: RC 流式 Read/Write
**Description:** 作为开发者，我想用 `conn.Read(p)` / `conn.Write(p)` 收发字节流，不关心 MR/WR/CQ。

**Acceptance Criteria:**
- [ ] `Read`/`Write` 满足 `io.Reader`/`io.Writer` 语义：Write 全量写出或返回错误；Read 阻塞至至少 1 字节
- [ ] 内部维护预注册的环形 bounce buffer，自动分片大于 buffer 的写入
- [ ] 实现基于信用（credit）的流控，慢接收方不会导致 RNR 错误或数据丢失
- [ ] 大于单条消息上限的数据（如 16MB）能正确传输（分片+重组）
- [ ] 并发模型：一个 goroutine 读 + 一个 goroutine 写安全；并发多读/多写返回明确错误或串行化
- [ ] `go test -race` 通过（stub + 模拟路径）

### US-005: Deadline 支持
**Description:** 作为开发者，我想用 `SetDeadline`/`SetReadDeadline`/`SetWriteDeadline` 控制超时，使 `Conn` 满足完整 `net.Conn` 契约。

**Acceptance Criteria:**
- [ ] 三个 Deadline 方法生效：超时后 Read/Write 返回满足 `net.Error`（`Timeout() == true`）的错误
- [ ] 超时后连接仍可用（与 net.Conn 一致：deadline 可重设后继续收发）
- [ ] 编译期断言 `var _ net.Conn = (*Conn)(nil)`
- [ ] 单元测试验证 deadline 在 stub/模拟路径上的行为

### US-006: Close 与资源回收
**Description:** 作为开发者，我希望 `Close()` 一次性释放 QP/CQ/MR/PD 等全部资源，且行为幂等。

**Acceptance Criteria:**
- [ ] `Close` 幂等；Close 后 Read/Write 返回 `net.ErrClosed` 语义的错误
- [ ] Close 能唤醒阻塞中的 Read/Write
- [ ] 对端关闭后本端 Read 返回 `io.EOF`
- [ ] 无资源泄漏：内部对象的释放顺序正确（QP→CQ→MR→PD）

### US-007: 单边操作 WriteAt/ReadAt
**Description:** 作为开发者，我想向对端暴露一块内存窗口，然后用 RDMA Write/Read 直接读写远端内存，绕过对端 CPU。

**Acceptance Criteria:**
- [ ] `Conn.ExposeWindow(size int) (*Window, error)`：注册本地内存并把 rkey/addr 告知对端（经内部控制消息）
- [ ] `Conn.RemoteWindow() (*RemoteWindow, error)`：获取对端暴露的窗口句柄
- [ ] `RemoteWindow.WriteAt(p []byte, off int64)` / `ReadAt(p []byte, off int64)` 实现 `io.WriterAt`/`io.ReaderAt`，越界返回错误
- [ ] `Window.Bytes()` 允许本端直接读写自己暴露的内存
- [ ] 单元测试覆盖越界校验与 stub 行为

### US-008: UD PacketConn 与 Addr 格式
**Description:** 作为开发者，我想用数据报语义在 UD QP 上收发消息，类似 `net.UDPConn`。

**Acceptance Criteria:**
- [ ] `ListenPacket(addr string, opts ...Option) (*PacketConn, error)` 实现 `net.PacketConn`（`ReadFrom`/`WriteTo`/`Close`/`LocalAddr`/Deadline 系列）
- [ ] 定义 `Addr` 类型封装 UD 寻址信息（GID/QPN/QKey），实现 `net.Addr`
- [ ] `Addr.String()` 采用 `gid%qpn` 格式（如 `fe80::1%0x12ab`；QKey 非默认值时附加 `#qkey`），`ResolveAddr(s string)` 能解析该格式并与 `String()` 互逆（round-trip）
- [ ] 保留消息边界；超过 UD MTU 上限的 WriteTo 返回明确错误（不静默截断）
- [ ] 内部管理 AH 缓存（按目的地址复用）
- [ ] 单元测试覆盖地址解析（含非法输入）与 stub 行为

### US-009: 零拷贝高级接口
**Description:** 作为追求性能的开发者，我想借出预注册的 buffer 直接收发，避免 bounce buffer 拷贝。

**Acceptance Criteria:**
- [ ] `Conn.AllocBuffer(size int) (*Buffer, error)` 返回预注册（pinned）buffer，`Buffer.Bytes()` 暴露内容
- [ ] `Conn.WriteBuffer(b *Buffer) error` / `Conn.ReadBuffer() (*Buffer, error)`（或等价借还模型）零拷贝收发
- [ ] Buffer 生命周期文档明确：写出后何时可复用、Close 后 buffer 失效
- [ ] 与普通 Read/Write 可混用或文档明确不可混用的约束
- [ ] 单元测试覆盖借还状态机与误用（如重复释放）

### US-010: PacketConn 带外地址注册机制
**Description:** 作为开发者，我需要一个轻量的带外机制来发现/交换 UD 对端地址（GID/QPN/QKey），不必自己实现交换协议。

**Acceptance Criteria:**
- [ ] 提供轻量注册器：`conn.NewRegistry(addr string)` 启动一个 TCP 注册服务（风格与 `handshake` 包一致，纯 Go、可独立单测）
- [ ] `PacketConn.Register(registryAddr, name string) error` 向注册器登记本端 `Addr`
- [ ] `conn.LookupAddr(registryAddr, name string) (*Addr, error)` 按名称查询对端 `Addr`
- [ ] 注册器为可选组件：用户也可自行带外分发 `Addr.String()` 后用 `ResolveAddr` 构造
- [ ] 注册/查询协议有纯 Go 单元测试（无需硬件）

### US-011: 示例与基准工具
**Description:** 作为用户，我想看到 conn 包的端到端示例，并能验证其性能开销相对原始 verbs 路径可接受。

**Acceptance Criteria:**
- [ ] 新增 `cmd/go-conn_bw` 与 `cmd/go-conn_lat`：基于 conn 包的带宽/延迟工具，支持 server/client 模式
- [ ] 工具支持 `-d` 设备、`-x` GID index、`-s` 大小、`-n` 次数等与现有 cmd 一致的常用 flag
- [ ] 现有 `cmd/` 六个工具不做改动（不迁移、不破坏）
- [ ] README 新增 conn 包 Quick Start 小节（含 net.Conn 直接替换示例）

### US-012: CQ poller 双模式（busy-poll / 事件驱动）
**Description:** 作为开发者，我想根据延迟与 CPU 占用的取舍选择 CQ 处理模式：低延迟场景用 busy-poll，省 CPU 场景用 CompChannel 事件驱动。

**Acceptance Criteria:**
- [ ] `WithPollMode(PollBusy | PollEvent)` 选项对 `Conn` 与 `PacketConn` 均生效，默认值在 godoc 中明确（建议默认 `PollEvent`）
- [ ] `PollBusy`：专用 goroutine 忙轮询 CQ（可结合 `runtime.Gosched` 让出）
- [ ] `PollEvent`：基于 CompChannel（`ibv_get_cq_event`）阻塞等待，唤醒后批量收割
- [ ] 两种模式下功能行为一致（仅性能特征不同），共享同一套完成分发逻辑
- [ ] 单元测试覆盖模式选项的解析与 stub 行为；硬件性能对比纳入 US-011 工具的 flag（如 `--poll=busy|event`）

### US-013: examples 目录与分特性示例
**Description:** 作为新用户，我想在 `examples/` 下按特性找到独立、可直接运行的最小示例，快速上手每个能力。

**Acceptance Criteria:**
- [ ] 新建 `examples/` 目录，每个特性一个子目录，各含独立 `main.go`（server/client 通过 flag 或参数区分）与简短 `README.md`（运行方法、预期输出）
- [ ] 至少覆盖以下子目录：
  - `examples/echo/` — RC 流式 Conn 最小 echo（US-002/US-004）
  - `examples/handshake-dial/` — TCP 握手方式建连（US-003）
  - `examples/deadline/` — Deadline 超时控制（US-005）
  - `examples/rdma-window/` — 单边 WriteAt/ReadAt（US-007）
  - `examples/packet/` — UD PacketConn 收发（US-008）
  - `examples/zerocopy/` — 零拷贝 Buffer 借还（US-009）
  - `examples/registry/` — 带外地址注册与发现（US-010）
  - `examples/pollmode/` — busy-poll 与事件模式切换（US-012）
- [ ] 所有示例在 stub 平台能编译（`go build ./examples/...` 纳入 CI），运行时打印 `ErrNotSupported` 友好提示
- [ ] echo 示例核心代码 < 30 行（不含 import/错误处理样板）

### US-014: 文档与 CI
**Description:** 作为维护者，我需要 conn 包有完整 godoc 和 CI 覆盖。

**Acceptance Criteria:**
- [ ] conn 包 doc.go 说明：语义模型、两种建连方式、两种 poll 模式、零拷贝接口的适用场景、平台支持
- [ ] CI 覆盖 conn 包与 examples：vet、cgo 与 stub 双构建、`go test -race`、darwin/windows 交叉编译
- [ ] 所有导出符号有 godoc 注释

## Functional Requirements

- FR-1: 系统必须在 `conn` 子包提供 `Dial`/`DialTimeout`/`Listen`，返回实现 `net.Conn`/`net.Listener` 的类型
- FR-2: 系统必须默认使用 rdma_cm 建连，并通过 `WithHandshake()` 选项切换为 TCP out-of-band 握手
- FR-3: `Conn.Write` 必须自动完成内存注册/复用、分片与 WR 提交，调用方只提供普通 `[]byte`
- FR-4: `Conn.Read` 必须阻塞直到有数据、连接关闭（EOF）或 deadline 超时
- FR-5: 系统必须实现基于 credit 的接收流控，防止接收方 buffer 耗尽
- FR-6: 系统必须支持 `SetDeadline`/`SetReadDeadline`/`SetWriteDeadline`，超时错误满足 `net.Error`
- FR-7: `Close` 必须幂等，且按依赖顺序释放全部底层资源
- FR-8: 系统必须提供 `ExposeWindow`/`RemoteWindow`，支持 `WriteAt`/`ReadAt` 单边操作
- FR-9: 系统必须提供 `ListenPacket` 返回实现 `net.PacketConn` 的 UD 数据报连接
- FR-10: UD `Addr.String()` 必须采用 `gid%qpn` 格式（QKey 非默认值时附加 `#qkey`），且 `ResolveAddr` 与之互逆
- FR-11: UD `WriteTo` 超过 MTU 限制时必须返回错误而非截断
- FR-12: 系统必须提供 `AllocBuffer`/`WriteBuffer`/`ReadBuffer` 零拷贝接口
- FR-13: 系统必须通过 `WithPollMode` 提供 busy-poll 与 CompChannel 事件驱动两种 CQ 处理模式
- FR-14: 系统必须提供轻量带外注册器（`NewRegistry`/`Register`/`LookupAddr`）用于 UD 地址发现，且为可选组件
- FR-15: 每个特性必须在 `examples/` 下有独立子目录的可运行示例，并纳入 CI 构建
- FR-16: 非 Linux 或无 cgo 构建必须编译通过且运行时返回 `gordma.ErrNotSupported`
- FR-17: conn 包不得修改根包、`handshake`、`perftest`、`cmd/` 的现有导出 API 与行为

## Non-Goals

- 不实现 RDMA 之上的 RPC/序列化协议（只做字节流/数据报）
- 不做 TCP 自动回退（RDMA 不可用时不降级为 TCP）
- 不支持 TLS/加密
- 不迁移/改写现有六个 `cmd/` perftest 工具
- 不支持 Windows/macOS 的真实 RDMA（仅 stub）
- 不实现连接池、重连、多路复用等上层功能
- 注册器不做高可用/持久化（仅内存表，进程退出即失效）

## Technical Considerations

- 流式语义需要内部消息分帧（RC 是消息型传输）：建议头部携带长度 + credit 回执，复用 SEND/RECV 通道
- CQ 处理模型：busy-poll 与 CompChannel 事件两种模式均实现（US-012），完成分发逻辑共享；默认事件模式省 CPU
- Deadline 实现可参考内部用 timer + 唤醒通道，CQ poll 循环检查截止时间
- bounce buffer 环形池大小 = QueueDepth × BufferSize，需文档化内存占用
- `ExposeWindow` 的 rkey/addr 交换走连接内置控制消息（带类型标记的帧），不要求用户做带外交换
- UD 注册器协议建议沿用 `handshake` 包的 line-delimited JSON 风格，纯 Go 实现便于单测
- stub 一致性：沿用根包 `stub_consistency_test.go` 的模式
- examples 子目录均为独立 main 包，避免相互依赖；CI 用 `go build ./examples/...` 验证

## Success Metrics

- echo 示例代码行数 < 30 行（不含 import/错误处理样板），对比现在 cmd 工具数百行
- `var _ net.Conn = (*Conn)(nil)`、`var _ net.PacketConn = (*PacketConn)(nil)` 编译期成立，现有依赖 net.Conn 的代码可零修改接入
- 零拷贝路径带宽达到 go_send_bw（原始 verbs）的 90% 以上；bounce 路径有量化数据
- CI 全绿（Linux cgo/nocgo + darwin/windows 交叉编译 + race + examples 构建）

## Open Questions

（无 — 原有三项已决议：UD Addr 采用 `gid%qpn` 格式（US-008/FR-10）；CQ poller 双模式做成 Option（US-012/FR-13）；PacketConn 提供轻量带外注册机制（US-010/FR-14）。）
