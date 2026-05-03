# gopm 使用文档

gopm 是一个 Go 语言编写的内网穿透工具，通过反向 TCP 隧道将公网流量转发到内网服务。

---

## 快速开始

### 1. 编译

```bash
go build -o gopm ./cmd/gopm/
```

### 2. 启动服务端（公网服务器上执行）

```bash
./gopm -mode server -port 9000
```

### 3. 启动本地服务

```bash
# 示例：启动一个简单的 HTTP 服务
python3 -m http.server 8080
```

### 4. 启动客户端（内网机器上执行）

```bash
./gopm -mode client -server <公网IP>:9000 -local 8080 -map 8080
```

### 5. 访问

```bash
curl http://<公网IP>:8080
```

---

## 命令行参数

### 服务端

```bash
./gopm -mode server -port <控制端口> [-token <token>] [-timeout <秒>] [-verbose]
```

| 参数 | 说明 | 必填 | 示例 |
|---|---|---:|---|
| `-mode` | 运行模式，固定为 `server` | 是 | `server` |
| `-port` | 控制端口，接收客户端连接 | 是 | `9000` |
| `-token` | 鉴权令牌，客户端必须提供相同值 | 否 | `abc123` |
| `-timeout` | 自动关闭时长（秒），0 为永久运行 | 否 | `3600` |
| `-verbose` | 输出详细调试日志 | 否 | |

### 客户端

```bash
./gopm -mode client -server <地址> -local <本地地址> -map <端口> [-token <token>] [-name <名称>] [-retry] [-timeout <秒>] [-verbose]
```

| 参数 | 说明 | 必填 | 示例 |
|---|---|---:|---|
| `-mode` | 运行模式，固定为 `client` | 是 | `client` |
| `-server` | 服务端控制地址 | 是 | `34.112.23.98:9000` |
| `-local` | 本地服务地址或端口 | 是 | `8080` 或 `127.0.0.1:8080` |
| `-map` | 服务端暴露的映射端口 | 是 | `8080` |
| `-token` | 鉴权令牌，需与服务端一致 | 否 | `abc123` |
| `-name` | 客户端标识名称，用于日志 | 否 | `dev-macbook` |
| `-retry` | 启用断线自动重连 | 否 | |
| `-timeout` | 自动关闭时长（秒），0 为永久运行 | 否 | `3600` |
| `-verbose` | 输出详细调试日志 | 否 | |

---

## `-local` 参数格式

支持以下写法：

| 输入 | 实际连接地址 |
|---|---|
| `8080` | `127.0.0.1:8080` |
| `127.0.0.1:8080` | `127.0.0.1:8080` |
| `192.168.1.100:3306` | `192.168.1.100:3306` |

---

## 使用场景示例

### 场景一： exposing 本地 Web 服务

将本地的 `localhost:3000` 通过公网服务器的 `8080` 端口暴露出去：

```bash
# 公网服务器
./gopm -mode server -port 9000 -verbose

# 内网机器
./gopm -mode client -server 1.2.3.4:9000 -local 3000 -map 8080 -retry
```

访问 `http://1.2.3.4:8080` 即可到达内网的 `localhost:3000`。

### 场景二：带 Token 鉴权

```bash
# 公网服务器
./gopm -mode server -port 9000 -token my_secret_token

# 内网机器
./gopm -mode client -server 1.2.3.4:9000 -local 8080 -map 8080 -token my_secret_token -retry
```

若 Token 不匹配，客户端将收到 `unauthorized` 错误并注册失败。

### 场景三：映射内网数据库

将内网的 MySQL 通过公网暴露（不建议在生产环境无 Token 使用）：

```bash
# 内网机器
./gopm -mode client -server 1.2.3.4:9000 -local 192.168.1.50:3306 -map 13306 -token db_pass
```

外部通过 `1.2.3.4:13306` 连接即可访问内网 MySQL。

### 场景四：多客户端映射不同端口

一个服务端可以同时服务多个客户端，每个客户端映射不同端口：

```bash
# 客户端 A：映射 Web 服务
./gopm -mode client -server 1.2.3.4:9000 -local 3000 -map 8080

# 客户端 B：映射 API 服务
./gopm -mode client -server 1.2.3.4:9000 -local 8080 -map 9090
```

### 场景五：临时调试，定时自动关闭

使用 `-timeout` 设置自动关闭时长（秒），适合临时调试场景，避免忘记关闭进程：

```bash
# 公网服务器，1 小时后自动关闭
./gopm -mode server -port 9000 -timeout 3600

# 内网机器，30 分钟后自动关闭
./gopm -mode client -server 1.2.3.4:9000 -local 8080 -map 8080 -timeout 1800
```

超时后会自动触发优雅关闭，打印日志并退出。默认值为 0（永久运行）。

---

## 工作原理

```
┌─────────┐       ┌──────────────┐       ┌──────────────┐
│ Visitor │──────>│   Server     │<──────│   Client     │
│ (外部)   │       │  (公网服务器)  │       │  (内网机器)    │
└─────────┘       └──────────────┘       └──────┬───────┘
                                                │
                                                v
                                         ┌──────────────┐
                                         │ Local Service│
                                         │ (本地服务)     │
                                         └──────────────┘
```

1. **客户端注册**：客户端连接服务端控制端口，发送 `register` 消息注册映射
2. **服务端监听**：服务端在映射端口上开始监听外部访问
3. **访客连接**：外部用户访问映射端口，服务端生成唯一 `conn_id` 并通知客户端
4. **建立隧道**：客户端新建数据连接，发送 `join` 消息，服务端将两条连接桥接
5. **流量转发**：访客流量经服务端 → 数据连接 → 客户端 → 本地服务，原样双向透传

---

## 特性说明

### 心跳保活

客户端每 15 秒向服务端发送 `ping`，服务端回复 `pong`。若超过 45 秒未收到心跳，判定连接断开，自动回收映射资源。

### 断线重连

使用 `-retry` 启用。连接断开后自动重连，重连间隔逐步增加：

```
1秒 → 2秒 → 5秒 → 10秒 → 10秒 ...
```

重连成功后自动重新发送注册请求，恢复映射。

### Token 鉴权

- 服务端配置了 `-token` 时，客户端注册和建立数据连接都必须携带相同 Token
- 服务端未配置 `-token` 时，不进行鉴权

### 端口冲突检测

同一服务端上同一映射端口不允许被重复注册，会返回 `map port already in use` 错误。

---

## 日志示例

### 服务端日志

```text
[INFO] server listening on :9000
[INFO] client registered: name=dev-macbook map=8080 local=127.0.0.1:8080
[INFO] mapping listen started on :8080
[INFO] new visitor map=8080 src=8.8.8.8:52341 conn_id=c_1710000000_a1b2c3d4
[INFO] join success conn_id=c_1710000000_a1b2c3d4
[INFO] client disconnected, mapping removed: :8080
```

### 客户端日志

```text
[INFO] connecting to server 34.112.23.98:9000
[INFO] register success map=8080 -> local=127.0.0.1:8080
[INFO] new_conn conn_id=c_1710000000_a1b2c3d4
[WARN] control connection lost, reconnecting...
```

---

## 常见问题

### 客户端注册失败：`map port already in use`

该映射端口已被其他客户端占用。换一个 `-map` 端口，或等待占用该端口的客户端断开后重试。

### 客户端注册失败：`unauthorized`

Token 不匹配。检查客户端 `-token` 参数是否与服务端配置一致。

### 客户端注册失败：`listen failed`

服务端无法监听该端口，通常是端口已被系统其他程序占用或权限不足（Linux 下非 root 用户无法监听 1024 以下端口）。

### 外部访问无响应

1. 检查服务端防火墙是否放行了控制端口和映射端口
2. 检查内网本地服务是否正常运行
3. 使用 `-verbose` 参数查看详细日志排查

### 客户端频繁断连

使用 `-retry` 参数启用自动重连。若网络不稳定，断开后会自动恢复。
