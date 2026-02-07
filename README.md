# GGS校园网认证

一个使用 Go 编写的独立认证守护程序，针对 GGS 校园网环境自动检测网络状态并完成认证。内置完整日志系统与 Windows 任务计划安装器，可在系统启动后以 SYSTEM 权限静默运行，每分钟进行一次网络检测与认证。

## 功能特性
- 自动检测与认证
  - 探测 `http://1.1.1.1/generate_204` 的重定向与页面内容
  - 识别 `portal.do` 或 `portalScript.do` 并解析认证所需参数
  - 调用认证端点 `http://10.20.16.5/quickauth.do` 完成认证
  - 认证后访问 `http://www.gstatic.com/generate_204` 验证是否返回 204，无效则二次重试

- 日志系统
  - 日志等级：DEBUG / INFO / WARN / ERROR（可通过配置文件设置，默认 INFO）
  - 日志写入到可执行文件同目录的 `portal.log`
  - 大于 5MB 自动轮转到 `history/portal_YYYYMMDD_HHMMSS.log`
  - 历史日志保存于 `history/` 目录，保留期 30 天，自动清理过期文件

- 配置管理
  - 首次运行自动生成 `portal.conf` 模板，缺少必要参数时提示后退出
  - 必填项：`userid`（手机号）、`passwd`（临时登录密码）
  - 可选项：`logLevel`（DEBUG/INFO/WARN/ERROR）

- Windows 任务计划安装器
  - 一键创建名为 `auto_portal` 的任务计划，触发器为系统启动（onstart），以 SYSTEM 身份运行
  - 自动复制 `portal.exe` 与 `portal.conf` 到 `C:\Program Files\portal\`
  - 支持删除任务与查看任务状态

## 目录结构
- `portal/portal.go` 认证守护程序源码
- `portal/portal.conf` 配置模板（示例）
- `portal/portal_go.md` 认证流程与实现细节说明
- `portal_windows_install/portal_windows_install.go` Windows 任务计划安装器源码
- `portal_windows_install/portal_windows_install.md` 安装器使用说明
- `README.md` 项目总览（本文档）

## 构建
前置条件：已安装 Go（建议较新版本）。

```powershell
# 在项目根目录执行（Windows 示例）
go build -o portal.exe portal/portal.go
go build -o portal_windows_install.exe portal_windows_install/portal_windows_install.go
```

## 安装与运行
### 方式 A：手动运行
1. 将 `portal.exe` 放置到任意目录并运行一次，程序会在同目录下创建 `portal.conf` 模板后退出。
2. 编辑同目录的 `portal.conf`：
   - 必填：`userid`、`passwd`
   - 可选：`logLevel`（默认 INFO）
3. 重新运行 `portal.exe`。程序会每分钟执行一次认证流程，日志写入到同目录的 `portal.log`，轮转日志在 `history/`。

### 方式 B：使用 Windows 任务计划安装器
1. 在根目录构建得到 `portal.exe` 与 `portal_windows_install.exe`。
2. 双击运行 `portal_windows_install.exe`（若非管理员将自动请求提权）。
3. 在主菜单选择“添加开机自启动任务”：
   - 安装器会复制 `portal.exe` 与 `portal.conf` 到 `C:\Program Files\portal\`
   - 创建 SYSTEM 权限、触发器为开机的任务计划 `auto_portal`，并立即运行一次
4. 配置文件路径：`C:\Program Files\portal\portal.conf`。如需修改，编辑后下一次程序循环将生效。
5. 也可使用安装器删除任务或查看当前任务状态与配置内容。

## 配置文件说明（portal.conf）
示例：
```
# =号后面输入账号（手机号）
userid=
# =号后面输入密码（密码为临时登录的密码）
passwd=
# 可选：日志级别（不填则为 INFO）
logLevel=INFO
```
- `userid`：手机号
- `passwd`：临时登录密码
- `logLevel`：DEBUG / INFO / WARN / ERROR（可选，大小写不敏感，默认 INFO）

配置文件应与 `portal.exe` 位于同一目录。程序首次运行时如未找到 `portal.conf` 会自动生成模板并提示编辑后再次运行。

## 日志说明
- 路径：与 `portal.exe` 同目录的 `portal.log`
- 轮转：超过 5MB 自动移动到 `history/portal_YYYYMMDD_HHMMSS.log` 并重新创建新的 `portal.log`
- 清理：自动删除 30 天前的历史日志（基于文件名中的日期）
- 格式：`[LEVEL][YYYY-MM-DD HH:MM:SS] message`

## 认证流程概览
1. 访问 `http://1.1.1.1/generate_204` 获取网络状态：
   - 301 且 `Server` 包含 cloudflare：认为不在目标网络内，退出
   - 200 且页面含 `portal.do`：解析重定向并进入认证
   - 302：
     - `Location` 含 `portalScript.do`：解析参数并进入认证
     - `Location` 含 `portalLogout.do`：判定已认证，无需处理
2. 解析重定向 URL 中的参数：`wlanuserip`、`wlanacname`、`mac`（支持 `AA:BB:CC:DD:EE:FF` 或 `AA-BB-CC-DD-EE-FF` 格式）、`vlan`
3. 构造并发送认证请求至 `http://10.20.16.5/quickauth.do`
4. 验证认证结果：访问 `http://www.gstatic.com/generate_204`，若返回 204 为成功；否则进行第二次认证与验证

详细的实现说明与示例见 `portal/portal_go.md`。

## 故障排查
- 程序提示“配置文件中缺少必要参数”
  - 编辑与可执行文件同目录的 `portal.conf`，填写 `userid` 与 `passwd`
- 认证失败或始终无法返回 204
  - 检查凭证是否正确，确认是否处在目标网络内
  - 查看 `portal.log` 与 `history/` 轮转日志获取详细错误
- MAC 地址格式错误
  - 程序从重定向 URL 解析 MAC；若格式不规范会报错，需要确认网络侧返回是否正确
- Windows 安装/运行权限问题
  - 安装器会自动请求管理员权限；任务创建使用 SYSTEM 身份

## 安全与提示
- 请勿将包含敏感信息的 `portal.conf` 提交到版本库或公开分享
- 本程序的认证端点与流程与 GGS 校园网环境相关，其他环境可能需要调整常量（如 `AuthEndpoint`、`CheckURL`、`VerifyURL`）与解析逻辑

## 开发/定制
核心常量位于 `portal/portal.go`：
- `CheckURL`：网络可达性探测地址（默认 `http://1.1.1.1/generate_204`）
- `VerifyURL`：认证后验证地址（默认 `http://www.gstatic.com/generate_204`）
- `AuthEndpoint`：认证端点（默认 `http://10.20.16.5/quickauth.do`）
- 定时器间隔：当前为每 1 分钟一次，可在 `main` 函数中调整

如需支持其他环境，请根据实际 portal 行为与参数格式调整解析与请求构造。

——
若你在使用中发现问题或有改进建议，可通过仓库 issue 提交反馈。
