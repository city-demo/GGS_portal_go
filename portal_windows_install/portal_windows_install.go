package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	taskName    = "auto_portal"
	portalDir   = "C:\\Program Files\\portal\\"
	defaultConf = `# 认证配置（请勿删除等号和前面的内容）
# userid 是手机号
userid= 
passwd= `
)

func main() {
	// 设置控制台输出为UTF-8编码
	setConsoleUTF8()

	if !isAdmin() {
		runAsAdmin()
		return
	}
	clearScreen()

	showMenu()
}

// 设置控制台为UTF-8编码
func setConsoleUTF8() {
	cmd := exec.Command("cmd", "/c", "chcp", "65001")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

func runAsAdmin() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	args := strings.Join(os.Args[1:], " ")
	var psCommand string
	if args != "" {
		psCommand = fmt.Sprintf("Start-Process -FilePath %q -ArgumentList %q -Verb RunAs", exe, args)
	} else {
		psCommand = fmt.Sprintf("Start-Process -FilePath %q -Verb RunAs", exe)
	}

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("提权失败: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// 清屏函数
func clearScreen() {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("清屏失败: %v\n", err)
	}
}

// 暂停并等待用户输入
func pause() {
	fmt.Print("\n按任意键返回菜单...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func showMenu() {
	for {
		fmt.Println("portal任务计划管理")
		fmt.Println("1. 添加开机自启动任务")
		fmt.Println("2. 删除现有任务计划")
		fmt.Println("3. 显示任务计划状态")
		fmt.Println("0. 退出程序")
		fmt.Print("请选择操作: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			addTask()
		case "2":
			deleteTask()
		case "3":
			showTaskStatus()
		case "0":
			os.Exit(0)
		default:
			fmt.Println("无效输入，请重新选择")
			pause()
			clearScreen()
		}
	}
}

func taskExists() bool {
	cmd := exec.Command("schtasks", "/query", "/tn", taskName)
	err := cmd.Run()
	return err == nil
}

func addTask() {
	if taskExists() {
		fmt.Printf("任务计划 %s 已存在，请先删除\n", taskName)
		pause()
		clearScreen()
		return
	}

	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取当前目录失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	exePath := filepath.Join(currentDir, "portal.exe")
	if _, errStat := os.Stat(exePath); os.IsNotExist(errStat) {
		fmt.Println("错误: 当前目录下未找到portal.exe")
		pause()
		clearScreen()
		return
	}

	confPath := filepath.Join(currentDir, "portal.conf")
	if _, errStat := os.Stat(confPath); os.IsNotExist(errStat) {
		fmt.Println("正在创建portal.conf配置文件...")
		if errWrite := os.WriteFile(confPath, []byte(defaultConf), 0644); errWrite != nil {
			fmt.Printf("创建配置文件失败: %v\n", errWrite)
			pause()
			clearScreen()
			return
		}
		fmt.Println("已创建默认配置文件，请编辑文件后重新运行程序")
		fmt.Println("配置文件路径:", confPath)
		fmt.Println("配置文件内容:")
		fmt.Println(defaultConf)
		pause()
		clearScreen()
		return
	}

	// 读取并检查配置文件内容
	confContent, err := os.ReadFile(confPath)
	if err != nil {
		fmt.Printf("读取配置文件失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	// 替换原来的检查逻辑
	confStr := string(confContent)
	if !strings.Contains(confStr, "userid=") || !strings.Contains(confStr, "passwd=") {
		fmt.Println("\n错误: 配置文件格式不正确，缺少 userid 和 passwd 配置项")
		fmt.Println("请检查配置文件:", confPath)
		pause()
		clearScreen()
		return
	}

	// 更宽松的检查方式
	getConfigValue := func(content, key string) string {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), key) {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
		return ""
	}

	userid := getConfigValue(confStr, "userid")
	passwd := getConfigValue(confStr, "passwd")

	if userid == "" || passwd == "" {
		fmt.Println("\n错误: userid 或 passwd 未配置")
		fmt.Println("当前配置:")
		fmt.Printf("userid=%s\n", userid)
		fmt.Printf("passwd=%s\n", passwd)
		fmt.Println("\n请编辑配置文件:", confPath)
		pause()
		clearScreen()
		return
	}

	if err := os.MkdirAll(portalDir, 0755); err != nil {
		fmt.Printf("创建目录失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	targetExe := filepath.Join(portalDir, "portal.exe")
	if err := copyFile(exePath, targetExe); err != nil {
		fmt.Printf("复制portal.exe失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	targetConf := filepath.Join(portalDir, "portal.conf")
	if err := copyFile(confPath, targetConf); err != nil {
		fmt.Printf("复制portal.conf失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	// 创建任务计划
	cmdCreate := exec.Command("schtasks", "/create", "/tn", taskName, "/tr",
		"\""+filepath.Join(portalDir, "portal.exe")+"\"",
		"/sc", "onstart", "/ru", "SYSTEM", "/f")
	outputCreate, errCreate := cmdCreate.CombinedOutput()
	if errCreate != nil {
		fmt.Printf("创建任务计划失败: %v\n%s\n", errCreate, string(outputCreate))
		pause()
		clearScreen()
		return
	}

	cmdRun := exec.Command("schtasks", "/run", "/tn", taskName)
	outputRun, errRun := cmdRun.CombinedOutput()
	if errRun != nil {
		fmt.Printf("立即运行任务失败: %v\n%s\n", errRun, string(outputRun))
		pause()
		clearScreen()
		return
	}

	fmt.Println("开机自启动任务添加成功并已启动")
	fmt.Println("程序将在后台静默运行，配置文件位于:", targetConf)
	pause()
	clearScreen()
}

func deleteTask() {
	if !taskExists() {
		fmt.Printf("任务计划 %s 不存在\n", taskName)
		pause()
		clearScreen()
		return
	}

	cmdTasklist := exec.Command("tasklist", "/FI", "IMAGENAME eq portal.exe")
	outputTasklist, errTasklist := cmdTasklist.CombinedOutput()
	if errTasklist != nil {
		fmt.Printf("检查 portal.exe 进程失败: %v\n%s\n", errTasklist, string(outputTasklist))
		pause()
		clearScreen()
		return
	}

	// 检查 C:\Program Files\portal\portal.exe 是否存在且运行
	targetExe := filepath.Join(portalDir, "portal.exe")
	if _, errStat := os.Stat(targetExe); errStat == nil {
		// 文件存在，检查是否在运行
		cmdCheck := exec.Command("tasklist", "/FI", "IMAGENAME eq portal.exe")
		outputCheck, errCheck := cmdCheck.CombinedOutput()
		if errCheck == nil && strings.Contains(strings.ToLower(string(outputCheck)), "portal.exe") {
			fmt.Println("检测到 portal.exe 正在运行，尝试终止...")
			cmdKill := exec.Command("taskkill", "/IM", "portal.exe", "/F")
			outputKill, errKill := cmdKill.CombinedOutput()
			if errKill != nil {
				fmt.Printf("终止 portal.exe 失败: %v\n%s\n", errKill, string(outputKill))
			} else {
				fmt.Println("已成功终止 portal.exe")
				time.Sleep(500 * time.Millisecond)
			}
		}
	} else {
		fmt.Println("portal.exe 不存在，跳过进程检查")
	}

	// 删除任务计划
	cmdDelete := exec.Command("schtasks", "/delete", "/tn", taskName, "/f")
	outputDelete, errDelete := cmdDelete.CombinedOutput()
	if errDelete != nil {
		fmt.Printf("删除任务计划失败: %v\n%s\n", errDelete, string(outputDelete))
		pause()
		clearScreen()
		return
	}

	// 清理安装目录
	maxRetries := 3
	retryDelay := 1 * time.Second
	for i := 0; i < maxRetries; i++ {
		errRemove := os.RemoveAll(portalDir)
		if errRemove == nil {
			fmt.Println("任务计划删除成功，安装目录已清理")
			break
		}
		fmt.Printf("删除安装目录失败 (尝试 %d/%d): %v\n", i+1, maxRetries, errRemove)
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		} else {
			fmt.Printf("无法删除安装目录 %s，请确保没有进程占用目录中的文件后手动删除\n", portalDir)
		}
	}
	pause()
	clearScreen()
}

func showTaskStatus() {
	if !taskExists() {
		fmt.Printf("任务计划 %s 不存在\n", taskName)
		pause()
		clearScreen()
		return
	}

	cmd := exec.Command("cmd", "/c", "schtasks /query /tn "+taskName)
	//cmd := exec.Command("cmd", "/c", "chcp 65001 && schtasks /query /tn "+taskName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("查询任务状态失败: %v\n", err)
		pause()
		clearScreen()
		return
	}

	confPath := filepath.Join(portalDir, "portal.conf")
	fmt.Println("\n配置文件路径:", confPath)
	if _, err := os.Stat(confPath); err == nil {
		fmt.Println("配置文件内容:")
		content, _ := os.ReadFile(confPath)
		fmt.Println(string(content))
	} else {
		fmt.Println("配置文件不存在")
	}
	pause()
	clearScreen()
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}
