package main

import (
	//	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// 日志级别
const (
	DEBUG = iota
	INFO
	WARN
	ERROR
)

// 配置常量
const (
	ConfigFile       = "portal.conf"
	LogFileName      = "portal.log"
	HistoryLogDir    = "history"
	MaxLogSize       = 5 * 1024 * 1024 // 5MB
	LogRetentionDays = 30
	CheckURL         = "http://1.1.1.1/generate_204"
	VerifyURL        = "http://www.gstatic.com/generate_204"
	AuthEndpoint     = "http://10.20.16.5/quickauth.do"
)

// 全局变量
var (
	logLevel   = INFO
	logFile    *os.File
	installDir string // 改为变量
)

// Config 配置结构体
type Config struct {
	UserID   string
	Passwd   string
	LogLevel int
}

// AuthParams 认证参数
type AuthParams struct {
	WlanUserIP string
	WlanAcName string
	MAC        string
	Vlan       string
}

// 初始化 installDir
func init() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法获取程序运行目录: %v\n", err)
		os.Exit(1)
	}
	installDir = filepath.Dir(exePath)

}

// 获取日志文件路径
func getLogPath() string {
	return filepath.Join(installDir, LogFileName)
}

// 获取历史日志目录
func getHistoryLogDir() string {
	return filepath.Join(installDir, HistoryLogDir)
}

// 获取配置文件路径
func getConfigPath() string {
	return filepath.Join(installDir, ConfigFile)
}

// 初始化日志系统
func initLogging() error {
	// 确保安装目录存在
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("无法创建安装目录: %v", err)
	}

	// 打开日志文件
	var err error
	logPath := getLogPath()
	logFile, err = os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("无法打开日志文件: %v", err)
	}

	log(DEBUG, "程序启动，日志系统初始化开始")
	log(DEBUG, "日志文件路径: %s", logPath)

	// 检查日志轮转
	if err = checkLogRotation(); err != nil {
		log(ERROR, "日志轮转检查失败: %v", err)
		return err
	}

	// 清理旧日志
	if err = cleanOldLogs(); err != nil {
		log(ERROR, "旧日志清理失败: %v", err)
	}

	log(DEBUG, "日志系统初始化完成")
	return nil
}

// 检查日志轮转
func checkLogRotation() error {
	logPath := getLogPath()
	log(DEBUG, "开始检查日志轮转，路径: %s", logPath)

	info, err := os.Stat(logPath)
	if os.IsNotExist(err) {
		log(DEBUG, "日志文件不存在，无需轮转")
		return nil
	} else if err != nil {
		return fmt.Errorf("获取日志文件信息失败: %v", err)
	}

	if info.Size() < MaxLogSize {
		log(DEBUG, "当前日志大小 %.2fMB < %.2fMB，无需轮转",
			float64(info.Size())/1024/1024, float64(MaxLogSize)/1024/1024)
		return nil
	}

	log(INFO, "当前日志大小 %.2fMB >= %.2fMB，需要轮转",
		float64(info.Size())/1024/1024, float64(MaxLogSize)/1024/1024)

	if err = logFile.Close(); err != nil {
		return fmt.Errorf("关闭日志文件失败: %v", err)
	}

	historyDir := getHistoryLogDir()
	if err = os.MkdirAll(historyDir, 0755); err != nil {
		return fmt.Errorf("无法创建历史日志目录: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	newLogName := fmt.Sprintf("portal_%s.log", timestamp)
	newLogPath := filepath.Join(historyDir, newLogName)

	if err = os.Rename(logPath, newLogPath); err != nil {
		return fmt.Errorf("无法重命名日志文件: %v", err)
	}

	logFile, err = os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("无法重新打开日志文件: %v", err)
	}

	log(INFO, "已轮转日志文件到: %s", newLogPath)
	return nil
}

// 清理旧日志
func cleanOldLogs() error {
	historyDir := getHistoryLogDir()
	log(DEBUG, "开始清理旧日志，目录: %s", historyDir)

	if _, err := os.Stat(historyDir); os.IsNotExist(err) {
		log(DEBUG, "历史日志目录不存在，跳过清理")
		return nil
	} else if err != nil {
		return fmt.Errorf("检查历史日志目录失败: %v", err)
	}

	files, err := os.ReadDir(historyDir)
	if err != nil {
		return fmt.Errorf("读取历史日志目录失败: %v", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -LogRetentionDays)
	log(DEBUG, "将清理 %d 天前的日志文件 (早于 %s)",
		LogRetentionDays, cutoffTime.Format("2006-01-02"))

	pattern := regexp.MustCompile(`^portal_(\d{8})_\d+\.log$`)
	count := 0

	for _, file := range files {
		fileName := file.Name()
		matches := pattern.FindStringSubmatch(fileName)
		if matches == nil || len(matches) < 2 {
			log(WARN, "跳过不符合命名规范的文件: %s", fileName)
			continue
		}

		dateStr := matches[1]
		fileDate, err := time.ParseInLocation("20060102", dateStr, time.Local)
		if err != nil {
			log(WARN, "解析文件日期失败: %s (%v)", dateStr, err)
			continue
		}

		if fileDate.Before(cutoffTime) {
			oldLog := filepath.Join(historyDir, fileName)
			if err := os.Remove(oldLog); err != nil {
				log(ERROR, "删除旧日志失败: %s (%v)", oldLog, err)
				continue
			}
			log(DEBUG, "已删除旧日志: %s (日志日期: %s)",
				fileName, fileDate.Format("2006-01-02"))
			count++
		}
	}

	log(INFO, "共清理了 %d 个旧日志文件", count)
	return nil
}

// 写日志
func log(level int, format string, args ...interface{}) {
	if level < logLevel {
		return
	}

	levelStr := ""
	switch level {
	case DEBUG:
		levelStr = "DEBUG"
	case INFO:
		levelStr = "INFO"
	case WARN:
		levelStr = "WARN"
	case ERROR:
		levelStr = "ERROR"
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	logEntry := fmt.Sprintf("[%s][%s] %s\n", levelStr, timestamp, message)

	// 写入文件
	if logFile != nil {
		if _, err := logFile.WriteString(logEntry); err != nil {
			fmt.Printf("写入日志失败: %v\n", err)
		}
	}

	// 输出到控制台
	fmt.Print(logEntry)
}

// 加载配置文件
func loadConfig() (*Config, error) {
	log(DEBUG, "开始加载配置文件")

	configPath := getConfigPath()
	log(DEBUG, "配置文件路径: %s", configPath)

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return createDefaultConfig(configPath)
	}

	// 读取整个文件内容
	content, err := os.ReadFile(configPath)
	if err != nil {
		log(ERROR, "无法读取配置文件: %v", err)
		return nil, fmt.Errorf("无法读取配置文件: %v", err)
	}

	config := &Config{
		LogLevel: INFO, // 默认日志级别
	}
	hasRequired := map[string]bool{
		"userid": false,
		"passwd": false,
	}

	lines := strings.Split(string(content), "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 处理键值对
		sepIndex := strings.Index(line, "=")
		if sepIndex == -1 {
			log(WARN, "跳过无效配置行 (第 %d 行): 缺少等号", lineNum+1)
			continue
		}

		key := strings.TrimSpace(line[:sepIndex])
		value := strings.TrimSpace(line[sepIndex+1:])

		switch key {
		case "userid":
			config.UserID = value
			hasRequired["userid"] = true
			log(DEBUG, "读取到 userid: %s", value)
		case "passwd":
			config.Passwd = value
			hasRequired["passwd"] = true
			log(DEBUG, "读取到 passwd: %s", strings.Repeat("*", len(value)))
		case "logLevel":
			switch strings.ToUpper(value) {
			case "DEBUG":
				config.LogLevel = DEBUG
			case "INFO":
				config.LogLevel = INFO
			case "WARN":
				config.LogLevel = WARN
			case "ERROR":
				config.LogLevel = ERROR
			default:
				log(WARN, "无效的日志级别: %s (第 %d 行)，使用默认值 INFO", value, lineNum+1)
			}
			log(DEBUG, "读取到 logLevel: %s", value)
		default:
			log(WARN, "跳过未知配置项: %s (第 %d 行)", key, lineNum+1)
		}
	}

	// 检查必要参数
	if !hasRequired["userid"] || !hasRequired["passwd"] {
		log(ERROR, "配置文件中缺少 userid 或 passwd 参数")
		fmt.Printf("\n错误: 配置文件中缺少 userid 或 passwd 参数\n请编辑配置文件: %s\n\n", configPath)
		return nil, errors.New("配置文件中缺少必要参数")
	}

	logLevel = config.LogLevel
	log(DEBUG, "配置文件加载成功")
	return config, nil
}

func createDefaultConfig(configPath string) (*Config, error) {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		log(ERROR, "无法创建配置目录: %v", err)
		return nil, fmt.Errorf("无法创建配置目录: %v", err)
	}

	defaultConfig := `# 认证配置（请勿删除等号和前面的内容）
# userid 是手机号
userid=
passwd=
`
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		log(ERROR, "无法创建配置文件: %v", err)
		return nil, fmt.Errorf("无法创建配置文件: %v", err)
	}

	log(INFO, "已创建默认配置文件: %s", configPath)
	fmt.Printf("\n已创建默认配置文件，请编辑以下文件后重新运行程序:\n%s\n\n", configPath)
	return nil, fmt.Errorf("请编辑配置文件后重新运行: %s", configPath)
}

// 检测网络状态并获取认证信息
func checkNetworkStatus() (string, *AuthParams, error) {
	log(DEBUG, "开始检测网络状态")

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	log(DEBUG, "发送请求到: %s", CheckURL)
	resp, err := client.Get(CheckURL)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			log(INFO, "请求超时，可能不在网络内")
			return "", nil, errors.New("网络超时，可能不在网络内")
		}
		log(ERROR, "请求失败: %v", err)
		return "", nil, fmt.Errorf("请求失败: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log(ERROR, "关闭响应体失败: %v", err)
		}
	}()
	log(DEBUG, "收到响应状态码: %d", resp.StatusCode)

	// 1. 处理301响应 (Cloudflare检测)
	if resp.StatusCode == http.StatusMovedPermanently {
		serverHeader := resp.Header.Get("Server")
		if strings.Contains(strings.ToLower(serverHeader), "cloudflare") {
			log(INFO, "检测到Cloudflare服务器，疑似不在网络内")
			return "", nil, errors.New("检测到Cloudflare，疑似不在网络内")
		}
		return "", nil, nil
	}

	// 2. 处理200响应 (portal.do检测)
	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log(ERROR, "读取响应体失败: %v", err)
			return "", nil, fmt.Errorf("读取响应失败: %v", err)
		}

		// 检查响应体中是否包含portal.do
		if strings.Contains(string(body), "portal.do") {
			log(INFO, "检测到portal.do页面，需要认证")

			// 尝试从响应体中提取重定向URL
			redirectURL := extractRedirectURL(string(body))
			if redirectURL == "" {
				log(ERROR, "在200响应中找到portal.do但未提取到重定向URL")
				return "", nil, errors.New("portal.do页面中未找到重定向URL")
			}

			params, err := parseAuthParams(redirectURL)
			if err != nil {
				return "", nil, err
			}
			return "NEED_AUTH", params, nil
		}

		log(INFO, "200响应但未检测到portal.do内容")
		return "", nil, nil
	}

	// 3. 处理302响应 (登出链接检测和portalScript.do检测)
	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if location == "" {
			log(ERROR, "302重定向响应中没有Location头")
			return "", nil, errors.New("重定向响应中没有Location头")
		}

		log(DEBUG, "获取到重定向Location: %s", location)

		// 检测portalLogout.do (已认证)
		if strings.Contains(location, "portalLogout.do") {
			log(INFO, "当前已认证，无需认证，登出链接: %s", location)
			return location, nil, nil
		}

		// 检测portalScript.do (需要认证)
		if strings.Contains(location, "portalScript.do") {
			log(INFO, "检测到portalScript.do，需要认证")
			params, err := parseAuthParams(location)
			if err != nil {
				return "", nil, err
			}
			return "NEED_AUTH", params, nil
		}

		log(INFO, "302重定向但未检测到portalLogout.do或portalScript.do")
		return "", nil, nil
	}

	// 其他状态码处理
	log(INFO, "未检测到需要认证的情况 (状态码: %d)", resp.StatusCode)
	return "", nil, nil
}

// 从HTML/JavaScript内容中提取重定向URL
func extractRedirectURL(content string) string {
	// 1. 尝试匹配 location.replace(...)
	re := regexp.MustCompile(`location\.replace\("([^"]+)"\)`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// 2. 尝试匹配 portal.do 的直接链接
	//re = regexp.MustCompile(`(http?://[^"']+portal\.do[^"']*)`)
	re = regexp.MustCompile(`(https?://[^\s"']+?portal\.do[^\s"']*?)(?:&url=|$)`)

	matches = re.FindStringSubmatch(content)
	if len(matches) > 0 {
		return matches[1]
	}

	return ""
}

// 从重定向URL解析认证参数
func parseAuthParams(redirectURL string) (*AuthParams, error) {
	log(DEBUG, "开始解析认证参数，URL: %s", redirectURL)

	u, err := url.Parse(redirectURL)
	if err != nil {
		log(ERROR, "解析URL失败: %v", err)
		return nil, fmt.Errorf("解析URL失败: %v", err)
	}

	query := u.Query()
	params := &AuthParams{
		WlanUserIP: query.Get("wlanuserip"),
		WlanAcName: query.Get("wlanacname"),
		MAC:        query.Get("mac"),
		Vlan:       query.Get("vlan"),
	}

	log(DEBUG, "解析到的参数: wlanuserip=%s, wlanacname=%s, mac=%s, vlan=%s",
		params.WlanUserIP, params.WlanAcName, params.MAC, params.Vlan)

	// 验证MAC地址格式
	macRegex := regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	if !macRegex.MatchString(params.MAC) {
		log(ERROR, "无效的MAC地址格式: %s", params.MAC)
		return nil, fmt.Errorf("无效的MAC地址格式: %s", params.MAC)
	}

	if params.WlanUserIP == "" || params.WlanAcName == "" {
		log(ERROR, "缺少必要的认证参数 (wlanuserip或wlanacname为空)")
		return nil, errors.New("缺少必要的认证参数")
	}

	log(INFO, "认证参数解析成功")
	return params, nil
}

// 执行认证请求
func doAuth(config *Config, params *AuthParams) error {
	log(INFO, "开始执行认证请求")

	//rawURL := fmt.Sprintf("%s?userid=%s&passwd=%s&wlanacname=%s&portalpageid=2&mac=%s&wlanuserip=%s",
	//	AuthEndpoint,
	//	config.UserID,
	//	config.Passwd,
	//	params.WlanAcName,
	//	params.MAC,
	//	params.WlanUserIP,
	//)
	//log(DEBUG, "未编码的认证 URL: %s", strings.Replace(rawURL, config.Passwd, "******", 1))

	// 构造认证URL
	authURL := fmt.Sprintf("%s?userid=%s&passwd=%s&wlanacname=%s&portalpageid=2&mac=%s&wlanuserip=%s",
		AuthEndpoint,
		url.QueryEscape(config.UserID),
		url.QueryEscape(config.Passwd),
		url.QueryEscape(params.WlanAcName),
		url.QueryEscape(params.MAC),
		url.QueryEscape(params.WlanUserIP),
	)

	log(DEBUG, "构造的认证URL: %s (密码已隐藏)", strings.Replace(authURL, url.QueryEscape(config.Passwd), "******", 1))

	client := &http.Client{Timeout: 10 * time.Second}
	log(DEBUG, "发送认证请求")
	resp, err := client.Get(authURL)
	if err != nil {
		log(ERROR, "认证请求失败: %v", err)
		return fmt.Errorf("认证请求失败: %v", err)
	}
	var closeErr error
	defer func() {
		closeErr = resp.Body.Close()
		if closeErr != nil {
			log(ERROR, "关闭响应体失败: %v", closeErr)
		}
	}()
	log(DEBUG, "收到认证响应状态码: %d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log(ERROR, "读取响应体失败: %v", err)
		return fmt.Errorf("读取响应失败: %v", err)
	}

	log(INFO, "认证响应: %s", string(body))
	return nil
}

// 验证认证状态
func verifyAuth() (bool, error) {
	log(DEBUG, "开始验证认证状态")

	time.Sleep(2 * time.Second)
	log(DEBUG, "等待 2 秒")

	log(DEBUG, "发送验证请求到: %s", VerifyURL)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(VerifyURL)
	if err != nil {
		log(ERROR, "验证请求失败: %v", err)
		return false, fmt.Errorf("验证请求失败: %v", err)
	}

	var closeErr error
	defer func() {
		closeErr = resp.Body.Close()
		if closeErr != nil {
			log(ERROR, "关闭响应体失败: %v", closeErr)
		}
	}()

	log(DEBUG, "验证响应状态码: %d", resp.StatusCode)

	if resp.StatusCode == http.StatusNoContent {
		log(INFO, "验证成功 (收到204状态码)")
		return true, nil
	}

	log(WARN, "验证未通过 (收到状态码: %d)", resp.StatusCode)
	return false, nil
}

// 主认证流程
func authProcess(config *Config) error {
	log(DEBUG, "启动认证流程")

	// 步骤1: 检测网络状态
	result, params, err := checkNetworkStatus()
	if err != nil {
		return fmt.Errorf("网络检测失败: %v", err)
	}

	// 情况处理
	switch {
	case result == "":
		log(INFO, "当前无需认证")
		return nil

	case result == "NEED_AUTH":
		log(INFO, "开始认证流程...")
		if err := doAuth(config, params); err != nil {
			return fmt.Errorf("认证失败: %v", err)
		}

		// 第一次验证
		if ok, _ := verifyAuth(); ok {
			log(INFO, "第一次验证成功，认证完成")
			return nil
		}

		log(WARN, "第一次验证失败，将尝试第二次认证")

		// 第二次尝试
		if err := doAuth(config, params); err != nil {
			return fmt.Errorf("第二次认证失败: %v", err)
		}

		// 第二次验证
		if ok, _ := verifyAuth(); ok {
			log(INFO, "第二次验证成功，认证完成")
			return nil
		}

		return errors.New("两次认证尝试均失败")

	default: // 已获取登出URL
		log(DEBUG, "当前已认证")
		return nil
	}
}

func main() {

	// 初始化日志系统
	if err := initLogging(); err != nil {
		fmt.Printf("初始化日志系统失败: %v\n", err)
		os.Exit(1)
	}

	// 加载配置
	config, err := loadConfig()
	if err != nil {
		log(ERROR, "加载配置失败: %v", err)
		os.Exit(1)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 创建定时器
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log(INFO, "程序启动，将每分钟运行一次认证流程")

	// 主循环
	for {
		select {
		case <-ticker.C:
			log(DEBUG, "开始定时认证流程")
			if err := authProcess(config); err != nil {
				log(ERROR, "认证流程失败: %v", err)
			}
		case sig := <-sigChan:
			log(INFO, "收到信号 %v，程序退出", sig)
			return
		}
	}
}
