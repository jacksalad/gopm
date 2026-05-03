# gopm

Go 语言编写的轻量级内网穿透工具，通过反向 TCP 隧道将公网流量转发到内网服务。服务端与客户端共用一个二进制文件，通过参数区分模式，零外部依赖。

## 特性

- TCP 端口映射，支持任意 TCP 协议（HTTP、SSH、数据库等）
- 一个 Server 支持多个 Client 同时映射不同端口
- 控制连接 + 每请求独立数据连接
- 15 秒心跳保活 + 45 秒超时自动回收映射
- 客户端断线自动重连（退避策略）
- 可选 Token 鉴权
- 优雅关闭（SIGINT / SIGTERM）
- 协议层消息大小限制，防止 DoS
- 纯 Go 标准库实现，无第三方依赖
- 可选 `-timeout` 参数，支持定时自动关闭

## 快速开始

### 编译

```bash
go build -o gopm ./cmd/gopm/
```

### 运行

```bash
# 公网服务器 — 启动服务端
./gopm -mode server -port 9000

# 内网机器 — 启动客户端，将本地 8080 映射到公网 8080
./gopm -mode client -server <公网IP>:9000 -local 8080 -map 8080 -retry
```

然后访问 `http://<公网IP>:8080` 即可到达内网的 `localhost:8080`。

按 `Ctrl+C` 可优雅关闭服务端或客户端，自动释放端口和连接资源。

## 原理

```
Visitor ──> Server:map_port ──> Server ──> Client(data conn) ──> Local Service
```

1. Client 连接 Server 控制端口，注册端口映射
2. Server 在映射端口监听外部访问
3. 外部访客访问映射端口时，Server 通知 Client 建立数据隧道
4. 双方通过 `io.Copy` 双向透传流量

## 参数

### 服务端

| 参数 | 说明 | 必填 |
|---|---|---:|
| `-port` | 控制端口 | 是 |
| `-token` | 鉴权令牌 | 否 |
| `-timeout` | 自动关闭时长（秒） | 否 |
| `-verbose` | 详细日志 | 否 |

### 客户端

| 参数 | 说明 | 必填 |
|---|---|---:|
| `-server` | 服务端地址 | 是 |
| `-local` | 本地服务地址或端口 | 是 |
| `-map` | 公网映射端口 | 是 |
| `-token` | 鉴权令牌 | 否 |
| `-name` | 客户端名称 | 否 |
| `-retry` | 断线自动重连 | 否 |
| `-timeout` | 自动关闭时长（秒） | 否 |
| `-verbose` | 详细日志 | 否 |

## 安全特性

| 特性 | 说明 |
|---|---|
| Token 鉴权 | 服务端配置 `-token` 后，客户端注册和数据连接均需携带相同 Token |
| 消息大小限制 | 协议层限制单条消息不超过 4 KiB，防止内存耗尽攻击 |
| 心跳超时回收 | 45 秒无心跳自动移除映射，防止静默断连占用端口 |
| 并发写保护 | 控制连接 Writer 通过互斥锁保护，防止多 goroutine 并发写入导致数据损坏 |

## 文档

- [doc.md](doc.md) — 完整使用文档（场景示例、原理、FAQ）
- [CLAUDE.md](CLAUDE.md) — 开发者架构参考

## License

MIT
