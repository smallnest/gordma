# PRD: RDMA Go 语言库（go-rdma）及 perftest 示例工具

## Introduction

实现一个 Go 语言的 RDMA 编程库，参考 [rdma-core](https://github.com/linux-rdma/rdma-core)（libibverbs / librdmacm 的官方用户态实现）提供 verbs API 的 Go 封装，并参考 [perftest](https://github.com/linux-rdma/perftest) 提供对应的带宽/延迟测试示例工具（`ib_send_bw/lat`、`ib_write_bw/lat`、`ib_read_bw/lat` 的 Go 版本，共 6 个）。

目标是让 Go 开发者无需写 C 代码即可使用 RDMA（RC/UD 传输、Send/Recv、RDMA Read/Write），并通过示例工具验证库的正确性与性能。

**已确认的关键决策（来自需求澄清）：**
- 实现方式：cgo 封装 libibverbs + librdmacm（与 rdma-core 行为一致，功能最全）（1A）
- 示例工具：完整集 —— send/write/read 的 bw 和 lat 共 6 个工具（2C）
- 传输类型：RC + UD 两种 QP 类型（3C）
- 建连方式：同时支持 TCP 带外交换 QP 信息（perftest 默认方式）与 rdma_cm 连接管理（3B + 3C）
- 目标环境：真实 RDMA 硬件 —— Mellanox/NVIDIA 网卡 + RoCE v2（以太网链路）（4A）
- 与工作区已有 `gordma` 目录无代码关系，全新独立项目（5A）；模块路径为 `github.com/smallnest/gordma`
- 带宽工具支持多深度 outstanding 请求：`-t tx-depth` 参数，默认 128（与 perftest 一致）
- 延迟工具输出完整直方图（类似 perftest 的 `--output=histogram`），同时输出 min/avg/max/p99 摘要

## Goals

- 提供地道的 Go API 封装核心 verbs 对象：Device、Context、PD、MR、CQ、QP、AH、CompChannel
- 支持 RC 与 UD 两种 QP 类型
- 支持两种建连方式：TCP 带外交换 QP 信息（手动状态机）与 rdma_cm（Dial/Listen 风格）
- 提供 6 个与 perftest 对应的示例命令行工具，支持 client/server 两端运行
- 工具输出带宽（MB/s、Mpps）或延迟统计（完整直方图 + min/avg/max/p99 摘要，单位 µs），格式参考 perftest
- 带宽工具支持多深度 outstanding 请求（`-t tx-depth`，默认 128）
- 在真实 RDMA 硬件（Mellanox/NVIDIA 网卡 + RoCE v2）上验证全部工具
- 资源管理安全：所有对象支持显式 Close，无 cgo 内存泄漏

## User Stories

### US-001: 设备枚举与 Context 打开
**Description:** As a Go developer, I want to list RDMA devices and open a device context so that I can start using RDMA resources.

**Acceptance Criteria:**
- [ ] `rdma.GetDeviceList()` 返回设备列表（名称、GUID、端口数）
- [ ] `Device.Open()` 返回 `*Context`，`Context.Close()` 释放资源
- [ ] `Context.QueryDevice()` / `QueryPort(n)` 返回设备/端口属性（MTU、LID、GID 表、状态等）
- [ ] 无 RDMA 设备时返回明确错误而非 panic
- [ ] `go vet` 与 `go build` 通过

### US-002: PD、MR 内存注册
**Description:** As a Go developer, I want to allocate a protection domain and register memory regions so that the NIC can access my buffers.

**Acceptance Criteria:**
- [ ] `Context.AllocPD()` / `PD.Close()` 正常工作
- [ ] `PD.RegMR(buf []byte, flags)` 注册内存，暴露 LKey/RKey
- [ ] 注册的 buffer 在 MR 生命周期内不被 Go GC 移动/回收（文档说明并在实现中保证）
- [ ] 错误的 access flags 或重复 Close 返回错误，不崩溃
- [ ] 单元测试覆盖注册/注销/越界场景

### US-003: CQ 创建与轮询
**Description:** As a Go developer, I want to create completion queues and poll completions so that I know when operations finish.

**Acceptance Criteria:**
- [ ] `Context.CreateCQ(depth)` / `CQ.Poll(wc []WorkCompletion)` 工作正常
- [ ] WorkCompletion 暴露 status、opcode、wr_id、byte_len、imm_data 字段
- [ ] 支持事件模式：CompChannel + `ReqNotify` + 阻塞等待（用于延迟不敏感场景）
- [ ] Poll 路径无每次调用的堆分配（复用调用方传入的切片）
- [ ] 单元测试通过

### US-004: RC QP 创建与手动建连（TCP 带外交换）
**Description:** As a Go developer, I want to create RC queue pairs, exchange QP info over TCP, and transition them through INIT/RTR/RTS so that two endpoints can communicate the same way perftest does.

**Acceptance Criteria:**
- [ ] `PD.CreateQP(opts)` 创建 RC QP，暴露 QPN
- [ ] `QP.ModifyToInit/ModifyToRTR/ModifyToRTS` 封装状态迁移
- [ ] 提供 TCP 带外握手辅助模块：交换 QPN、PSN、GID/LID、RKey、远端地址
- [ ] 状态参数（dest QPN、PSN、GID、MTU）通过结构体显式传入
- [ ] 非法状态迁移返回包含 errno 信息的错误
- [ ] 两机（或同机双端口）真实硬件集成测试通过

### US-005: UD QP 与 Address Handle
**Description:** As a Go developer, I want to create UD queue pairs and address handles so that I can do unreliable datagram messaging (and run send tests in UD mode).

**Acceptance Criteria:**
- [ ] 支持创建 UD QP，状态迁移（INIT/RTR/RTS，含 qkey）
- [ ] `PD.CreateAH(attr)` 创建地址句柄，PostSend 可携带 AH + remote QPN + qkey
- [ ] UD 收包正确处理 40 字节 GRH 偏移
- [ ] 集成测试：UD 模式下两端 Send/Recv 数据校验一致
- [ ] 消息大小超过 path MTU 时返回明确错误

### US-006: rdma_cm 建连（Dial/Listen 风格）
**Description:** As a Go developer, I want a net-like Dial/Listen API using rdma_cm so that I don't have to manually exchange QP information out-of-band.

**Acceptance Criteria:**
- [ ] `rdma.Listen(addr)` 返回 listener，`Accept()` 返回已建连的连接对象
- [ ] `rdma.Dial(addr)` 完成地址解析、路由解析、连接建立全流程
- [ ] 连接对象内部 QP 已处于 RTS，可直接 post 操作
- [ ] 超时与对端拒绝场景返回明确错误
- [ ] 真实硬件环境两进程间集成测试通过

### US-007: Send/Recv 与 RDMA Read/Write 操作
**Description:** As a Go developer, I want to post send/recv/read/write work requests so that I can transfer data.

**Acceptance Criteria:**
- [ ] `QP.PostSend` 支持 SEND、RDMA_WRITE、RDMA_READ opcode 与 SGE 列表
- [ ] `QP.PostRecv` 工作正常
- [ ] 支持 inline send（小消息优化）与 signaled/unsignaled 标志
- [ ] 集成测试：两端通过 Send/Recv 收发数据、通过 RDMA Write/Read 读写远端内存，数据校验一致
- [ ] post 热路径无每次调用的堆分配（性能基线要求）

### US-008: go-send_bw / go-send_lat 工具
**Description:** As a user, I want Go versions of ib_send_bw and ib_send_lat so that I can measure Send/Recv bandwidth and latency.

**Acceptance Criteria:**
- [ ] 支持 server 模式（无对端参数）与 client 模式（指定 server 地址）
- [ ] 支持参数：`-s`（消息大小）、`-n`（迭代次数）、`-d`（设备）、`-i`（端口号）、`-p`（TCP 握手端口）、`-c RC|UD`（传输类型）、`-R`（使用 rdma_cm 建连）、`-t`（tx-depth，默认 128，bw 工具）、`-x`（GID index）
- [ ] send_bw 输出：#bytes、#iterations、BW average[MB/s]、MsgRate[Mpps]
- [ ] send_lat 输出：t_min、t_avg、t_max、p99 摘要（µs）+ `--output=histogram` 完整直方图，基于逐次 ping-pong 计时
- [ ] RC 与 UD 模式在真实硬件上均跑通

### US-009: go-write_bw / go-write_lat 工具
**Description:** As a user, I want Go versions of ib_write_bw and ib_write_lat so that I can measure RDMA Write performance.

**Acceptance Criteria:**
- [ ] 建连后通过带外握手交换 RKey/远端地址
- [ ] write_bw 输出格式与 US-008 一致
- [ ] write_lat 使用写远端标志位轮询（polling on last byte）方式测延迟，与 perftest 方法一致
- [ ] 真实硬件上跑通

### US-010: go-read_bw / go-read_lat 工具
**Description:** As a user, I want Go versions of ib_read_bw and ib_read_lat so that I can measure RDMA Read performance.

**Acceptance Criteria:**
- [ ] 支持与 US-008 相同的基础参数（read 仅 RC）
- [ ] read_bw / read_lat 输出格式与前述工具一致
- [ ] 真实硬件上跑通

### US-011: 文档与 CI
**Description:** As a developer, I want documentation and CI so that the library is usable and maintainable.

**Acceptance Criteria:**
- [ ] README：安装依赖（libibverbs-dev、librdmacm-dev）、快速开始示例、各工具用法
- [ ] 所有导出 API 有 godoc 注释
- [ ] CI（Linux）：`go vet`、`go build`、不依赖硬件的单元测试
- [ ] 硬件相关集成测试通过构建标签/环境变量隔离，在有 RDMA 设备的机器上手动或专用 runner 执行
- [ ] macOS/无 RDMA 环境下 `go build` 可通过（构建标签隔离），仅运行时报不支持错误

## Functional Requirements

- FR-1: 系统必须通过 cgo 链接 libibverbs 和 librdmacm 提供 RDMA 功能
- FR-2: 系统必须提供设备枚举、Context 打开/关闭 API
- FR-3: 系统必须提供 PD 分配与 MR 注册/注销 API，并保证注册内存对 GC 安全
- FR-4: 系统必须提供 CQ 创建、轮询和事件通知 API
- FR-5: 系统必须提供 RC QP 创建与状态迁移 API
- FR-6: 系统必须提供 UD QP 与 Address Handle API
- FR-7: 系统必须提供 TCP 带外握手模块用于交换 QP 连接信息
- FR-8: 系统必须提供基于 rdma_cm 的 Listen/Accept/Dial 建连 API
- FR-9: 系统必须支持 SEND、RECV、RDMA_WRITE、RDMA_READ 四类工作请求
- FR-10: 系统必须支持 inline 数据与 signaled/unsignaled 完成控制
- FR-11: 每个示例工具必须支持 client/server 双模式运行
- FR-12: 带宽工具必须输出平均带宽（MB/s）与消息速率（Mpps）
- FR-13: 延迟工具必须输出 min/avg/max/p99 延迟（µs）
- FR-13a: 延迟工具必须支持 `--output=histogram` 输出完整延迟直方图
- FR-13b: 带宽工具必须支持 `-t tx-depth` 多深度 outstanding 请求（默认 128）
- FR-14: send 工具必须支持通过参数在 RC 与 UD 之间切换
- FR-15: 工具必须支持通过参数在 TCP 带外握手与 rdma_cm 两种建连方式之间切换
- FR-16: 所有资源对象必须提供 Close 方法且幂等（重复 Close 不崩溃）
- FR-17: 库在非 Linux 平台必须可编译（通过构建标签提供 stub）

## Non-Goals (Out of Scope)

- 不支持 UC、SRQ、XRC、DC 等其他传输类型（后续版本考虑）
- 不支持原子操作（Atomic CAS/FADD）
- 不实现纯 Go（无 cgo）的内核接口；不做 GPU Direct / DMA-BUF
- 不追求与 C 版 perftest 完全等同的性能（性能差距记录但不作为验收阻塞项）
- 不实现 perftest 的全部参数（如 `--run_infinitely`、双向测试、多 QP、CUDA 等）
- 不与 C 版 perftest 互通（其带外握手协议为私有格式），仅 Go 对 Go 测试
- 不支持 Windows / macOS 运行时

## Technical Considerations

- 依赖：Linux、libibverbs-dev、librdmacm-dev、Go 1.22+
- 模块路径：`github.com/smallnest/gordma`，工具放在 `cmd/` 下
- 目标链路为 RoCE v2：寻址使用 GID（GID index 可配置，默认选 RoCE v2 类型的 GID），握手信息携带 GID + QPN + PSN + RKey/addr
- 热路径（PostSend/PostRecv/Poll）需注意 cgo 调用开销，可考虑批量 post 与缓存 C 结构体
- MR buffer 必须用 C 分配或 pinned 方式管理，避免 Go GC 移动内存
- IB 与 RoCE 寻址差异：IB 用 LID，RoCE 用 GID（GID index 需可配置）；握手信息需同时携带
- 开发调试可选用 Soft-RoCE (rxe)，但验收以真实硬件为准
- 与 `gordma` 无代码关系，全新实现

## Success Metrics

- 6 个示例工具在真实 RDMA 硬件上全部跑通，数据校验无误
- send 工具的 RC/UD 两种模式、以及 TCP 握手/rdma_cm 两种建连方式均验证通过
- Go 对 Go 的统计结果数量级合理（与同环境 C 版 perftest 结果对比，带宽差距在可解释范围内并记录）
- 单元测试在无硬件 CI 上稳定通过；集成测试在硬件机器上通过
- 一个新用户按 README 在 30 分钟内跑通第一个工具

## Open Questions

（已全部澄清）
1. ~~硬件环境~~ → Mellanox/NVIDIA 网卡 + RoCE v2
2. ~~模块路径~~ → `github.com/smallnest/gordma`
3. ~~tx-depth~~ → 支持 `-t` 参数，默认 128
4. ~~延迟输出~~ → 完整直方图（`--output=histogram`）+ min/avg/max/p99 摘要
