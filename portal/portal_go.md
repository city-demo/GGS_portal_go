# 认证程序文档

## 概述
- 使用GO语言编写的独立认证程序（不依赖外部组件）
- 包含完整的日志系统功能

## 日志系统功能
- **自动日志轮转**:
    - 当日志文件(`portal.log`)超过5MB时自动轮转
    - 创建历史日志目录(`portal.history`)（若不存在）
    - 重命名当前日志为带时间戳格式（如`portal_20240410_153000.log`）
    - 将旧日志移动到`portal.history`目录

- **日志管理**:
    - 支持多级别日志：DEBUG/INFO/WARN/ERROR
    - 日志格式示例：`[[INFO][2025-04-09 20:44:24] 程序启动，日志系统初始化完成`
    - 自动清理30天以上的历史日志文件(基于文件名中的日期)
    - 自动将日志写入日志文件(`portal.log`)
    - 日志级别从portal.conf中获取，不存在将自动生成logLevel

## 配置处理
1. 从`portal.conf`读取认证凭证
2. 配置文件不存在时自动生成
3. 配置检查：
    - 必须包含`userid`和`passwd`参数
    - 若参数缺失：
        - 记录ERROR日志
        - 终止程序执行

## 获取登录信息
1. 发送请求到 `http://1.1.1.1/generate_204` ：
    - 超时：记录INFO日志"可能不在网络内" → 退出程序
    - 检测到（200响应）和链接内容中存在 portal.do： → 进入认证流程
    - 检测到（301响应）和链接文本中存在 cloudflare：记录INFO日志"疑似不在网络内" → 退出程序
    - 检测到（302响应）：
        - 检测到portalScript.do→ 进入认证流程
             ```
             HTTP/1.1 302 Object moved
             Connection: close
             Location: http://1.1.1.2/portalScript.do?wlanuserip==3.3.3.3&wlanacname=NFV-BASE-02&mac=11:a1:11:22:22:33&vlan=1111&hostname=&rand=52wsf&url=http://1.1.1.1/
             ```
          
        - 检测到portalLogout，记录INFO日志"无需认证"，提取Location头中的登出链接（示例）：
              ```
              HTTP/1.1 302 Object moved
              Location: http://1.1.1.2/portalLogout.do?wlanuserip=1.1.1.1&wlanacname=02&username=111111@11110&vlan=1532&rand=2q235606e286
              ```
              - 退出程序

## 认证流程（portal.do/portalScript.do）

1. 从重定向URL解析关键参数：
    - MAC地址（格式：`00:1A:2B:3C:4D:5E`）
    - wlanuserip（用户当前IP）
    - 解析失败则记录ERROR并退出

2. 示例重定向URL内容：

   
   ```
    HTTP/1.1 302 Object moved
    Connection: close
    Location: http://10.20.16.5/portalScript.do?wlanuserip==3.3.3.3&wlanacname=NFV-BASE-02&mac=11:a1:11:22:22:33&vlan=1111&hostname=&rand=52wsf&url=http://1.1.1.1/
      ```
   
   ```html
   <html>
   <head><script>
   location.replace("http://1.1.1.2/portal.do?wlanuserip=3.3.3.3&wlanacname=NFV-BASE-02&mac=11:a1:11:22:22:33&vlan=1111&hostname=&rand=52wsf&url="+encodeURIComponent("http://1.1.1.1"));
   </script></head>
   <body></body>
   </html>
   ```
   
4. 构造认证请求：
```
http://10.20.16.5/quickauth.do?userid=&passwd=&wlanacname=NFV-BASE-02&portalpageid=2&mac=&wlanuserip=
```
 - 将提取的参数填入对应字段
 - 执行请求并记录响应日志

4. 第一次验证:
    - 访问 http://www.gstatic.com/generate_204
    - 检查是否返回HTTP/1.1 204
      - 成功：记录INFO日志 → 退出验证
      - 失败：再次执行认证流程

5. 第二次验证（第一次失败时）:
    - 访问 http://www.gstatic.com/generate_204
    - 检查是否返回HTTP/1.1 204
        - 成功：记录INFO日志 → 退出验证
        - 仍失败： 记录ERROR日志 → 返回非零错误码

