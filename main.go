package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {

	// 定义命令行参数
	var (
		file       string
		concurrent bool
		headless   bool
	)

	flag.StringVar(&file, "file", "", "Excel文件路径 (例如: /abc/def/xxx.xls)")
	flag.BoolVar(&concurrent, "concurrent", false, "是否并发处理(默认false)")
	flag.BoolVar(&headless, "headless", true, "无头模式运行浏览器(默认true")

	flag.Parse()

	// 1. 检查并安装 Playwright
	if err := isPlaywrightInstalled(); err != nil {
		log.Fatalf("❌ 环境初始化失败: %v", err)
	}

	// 2. 校验参数
	if file == "" {
		fmt.Println("错误: 必须指定 file 参数")
		// flag.Usage()
		os.Exit(1)
	}
	// 检查参数文件是否存在
	if exists, err := checkFileExists(file, "xls"); !exists {
		log.Fatalf("错误: %v\n", err)
	}

	// 3. 检查Excel文件记录
	log.Printf("📁 检验Excel文件: %s", file)
	videoCreateTasks, err := ValidateExcelFile(file)
	if err != nil {
		log.Fatalf("❌ Excel文件验证失败: %v", err)
	}

	// 4. 打开网页扫码登录
	log.Println("🚀 第一阶段：扫码登录并保存认证状态...")
	authState, err := processUserLogin()
	if err != nil {
		log.Fatalf("❌ 登录阶段失败: %v", err)
	}

	// 5. 处理EXCEL文件
	log.Println("🚀 第二阶段：处理视频创建任务...")
	videoCreateResults := ProcessVideoCreateTask(videoCreateTasks, authState, concurrent, headless)

	// 6. 打印上传结果
	log.Println("🚀 第三阶段：打印上传结果...")
	PrintVideoCreateResults(videoCreateResults)

	// 7. 程序结束
	log.Println("🎉 所有文件上传完成！")
}
