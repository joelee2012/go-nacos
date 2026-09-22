# go-nacos 优化待办

## P0 — 生产环境必现问题

- [x] **并发安全**：`GetToken` 读写 `c.Token`/`c.ExpiredAt` 无锁保护，并发调用存在 data race
  - ✅ 添加 `sync.RWMutex` 保护 token 刷新，使用双重检查锁定模式
  - ✅ `GetVersion` 同样使用 RWMutex 保护 `c.state` 读写
  - ✅ 嵌入 `*Token`/`*State` 改为 `token`/`state` 命名字段，消除命名歧义
- [x] **构造函数做 I/O 且吞错误**：`NewClient` 调用 `DetectAPIVersion` 但无法返回错误，失败时静默降级为 v1
  - ✅ `NewClient` 改为返回 `(*Client, error)`
  - ✅ `DetectAPIVersion` 改为返回 `error`
  - ✅ Token 过期计算移到请求成功后（同时修复了 P1 的 Token 过期偏差问题）

## P1 — 功能性缺陷

- [x] **自定义 HTTP Client**：使用 `http.DefaultClient` 不可配置超时/重试/代理
  - ✅ `Client.HTTPClient` 默认设置 `Timeout: 30 * time.Second`
  - ✅ 新增 `Option` 函数式选项与 `WithHTTPClient(*http.Client)`，`NewClient` 改为变参 `opts ...Option`（向后兼容）
  - ✅ `WithHTTPClient(nil)` 为 no-op，避免误清空传输层
  - 文件：`nacos.go`
- [x] **`nil, nil` 语义模糊**：`GetConfig`/`GetNamespace`/`GetUser` 等未找到时返回 `(nil, nil)`，调用方无法区分“不存在”和“成功但空”
  - ✅ 已定义 `var ErrNotFound = errors.New("not found")`，未找到时返回 `(nil, ErrNotFound)`
  - ✅ 新增 `NacosErr.Is(target error) bool`：HTTP 404 的 `NacosErr` 也匹配 `ErrNotFound`，调用方统一用 `errors.Is(err, nacos.ErrNotFound)`，不再需要同时判 `IsNotFound()` 与 `ErrNotFound` 两套
  - 文件：`nacos.go`
- [x] **Token 过期计算偏差**：已在 P0 修复中一并完成
  - ✅ 移到请求成功后再计算 `ExpiredAt = time.Now().Unix() + TokenTTL`

## P2 — 设计改进

- [x] **移除嵌入 `*Token`/`*State`**：已在 P0 修复中一并完成
  - ✅ 改为 `token *Token` / `state *State` / `apiVersion string` **未导出字段**，强制通过 `GetToken()`/`GetVersion()`/`Init()` 访问，外部无法绕过锁改写运行态
  - ✅ 消除 `c.AccessToken`（Token 嵌入）和 `c.User`（Client 自身）的命名歧义
- [x] **`ListConfig` 静默修改调用方 `opts`**：`PageNumber`/`PageSize` 为 0 时会写回默认值到入参结构体，对外是 surprising side effect
  - ✅ 改用局部变量 `pageNo`/`pageSize`，不再 mutate `opts`；`ListConfigInNs` 自行维护游标
  - 文件：`nacos.go`
- [ ] **`doRequest` 静默丢弃 values**：当 `body != nil && values != nil` 时 POST/PUT 的 values 被忽略但无提示
  - 增加参数冲突检查，返回明确错误
  - 文件：`nacos.go:538-548`
- [x] **`GetVersion` 有副作用**：已在 P0 修复中一并完成
  - ✅ `GetVersion` 不再直接修改 `c.State`，而是解码到局部变量后赋值

## P3 — 代码质量

- [ ] **注释掉的代码**：`NamespaceList` 中注释字段应清理或恢复
  - 文件：`type.go:26-27`
- [ ] **`DeleteCfgOpts` 类型别名**：`type DeleteCfgOpts = GetCfgOpts` 语义不清
  - 直接使用 `GetCfgOpts` 或定义独立类型
  - 文件：`nacos.go:375`
- [ ] **部分方法缺少双版本测试**：`TestCreateConfig`/`TestDeleteConfig`/`TestCreateUser` 等未覆盖 v1/v3
  - 文件：`nacos_test.go:340-385`
- [ ] **测试共享 Client 状态**：测试间复用同一 Client 实例，Token/State 可能相互污染
  - 每个子测试创建独立的 `startServer()` + Client
  - 文件：`nacos_test.go`
