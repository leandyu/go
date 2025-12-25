package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// processUserLogin 用户扫码登录并保存认证状态
func processUserLogin() (*PageState, error) {
	// 生成浏览器
	pw, browser, context, err := GenerateBrowser(false)
	if err != nil {
		log.Printf("❌ 启动 Playwright 失败: %v", err)
		return nil, fmt.Errorf("启动 Playwright 失败: %v", err)
	}
	defer pw.Stop()
	defer (*browser).Close()
	defer (*context).Close()

	// 生成扫码登录页面
	page, _, err := GeneratePage(context, true)
	// 等待用户扫码登录
	log.Println("⏰ 页面已打开, 您有10分钟时间完成扫码...")
	if err != nil {
		return nil, fmt.Errorf("登录失败: %v", err)
	}
	defer (*page).Close()

	// 保证登录认证信息
	log.Println("✅ 登录成功！正在保存认证状态...")
	pageState, err := SaveAuthState(*page, *context)
	if err != nil {
		return nil, fmt.Errorf("保存认证状态失败: %v", err)
	}

	log.Println("✅ 认证状态已保存，关闭浏览器...")
	return pageState, nil
}

// ProcessVideoCreateTask 处理视频创建任务
func ProcessVideoCreateTask(videoCreateTasks []VideoCreateTask, authState *PageState, concurrent bool, headless bool) []VideoCreateTask {
	log.Printf("🚀 开始处理视频上传任务，共 %d 个任务", len(videoCreateTasks))

	// 创建日志文件
	logFile, err := createLogFile()
	if err != nil {
		log.Printf("❌ 创建日志文件失败: %v", err)
		return nil
	}
	defer logFile.Close()

	// 创建共享pw, 浏览器、上下文
	pw, browser, context, err := GenerateBrowser(headless)
	if err != nil {
		log.Printf("❌ 创建浏览器失败: %v", err)
		return nil
	}
	// 恢复从扫码登录获取的授权信息
	restoreAuthState(*context, authState)
	// 延迟关闭
	defer pw.Stop()
	defer (*browser).Close()
	defer (*context).Close()

	if !concurrent {
		// 处理顺序上传
		log.Printf("🚀 开始顺序处理视频上传任务")
		videoCreateTasks = processTaskSequential(context, videoCreateTasks, logFile)
	} else {
		// 并发上传
		log.Printf("🚀 开始并行处理视频上传任务")
		videoCreateTasks = processTaskConcurrent(context, videoCreateTasks, logFile)
	}
	return videoCreateTasks
}

// processTaskSequential 处理顺序上传
func processTaskSequential(context *playwright.BrowserContext, videoCreateTasks []VideoCreateTask, logFile *os.File) []VideoCreateTask {
	// 生成视频上传页面
	page, channelName, pageError := GeneratePage(context, false)
	if pageError != nil {
		log.Printf("❌ 创建上传页面失败或登录失效: %v", pageError)
		// 保存上传处理结果
		videoCreateTasks[1].Success = false
		videoCreateTasks[1].Error = pageError.Error()
		writeLogFile(logFile, videoCreateTasks[1], channelName)
		return videoCreateTasks
	}

	defer (*page).Close()
	for i := range videoCreateTasks {
		// 上传视频和填充值表单并保存
		videoCreateTasks[i] = createVideo(page, videoCreateTasks[i])
		// 保存上传处理结果
		writeLogFile(logFile, videoCreateTasks[i], channelName)
		// 刷新页面重试
		(*page).Reload()
		time.Sleep(3 * time.Second)
	}
	return videoCreateTasks
}

// processTaskConcurrent 视频并发上传
func processTaskConcurrent(context *playwright.BrowserContext, videoCreateTasks []VideoCreateTask, logFile *os.File) []VideoCreateTask {
	// 并发数
	maxConcurrency := 3
	if len(videoCreateTasks) > 50 && len(videoCreateTasks) < 100 {
		maxConcurrency = 5
	}
	if len(videoCreateTasks) > 100 {
		maxConcurrency = 10
	}
	// 并发处理
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i := range videoCreateTasks {
		wg.Add(1)
		semaphore <- struct{}{}

		videoCreateTask := videoCreateTasks[i]
		index := i

		go func(videoCreateTask VideoCreateTask, index int) {
			defer wg.Done()
			defer func() { <-semaphore }()
			log.Printf("🚀 开始执行第 %d 个任务: %s", index+1, filepath.Base(videoCreateTask.VideoPath))
			// 生成上传视频页面 - 每一个协和生成一个页面
			page, channelName, pageError := GeneratePage(context, false)
			videoCreateTask.Page = page
			videoCreateTask.ChannelName = channelName
			defer (*page).Close()
			if pageError == nil {
				// 上传视频和填充值表单并保存
				videoCreateTask = createVideo(page, videoCreateTasks[i])
			} else {
				videoCreateTask.Success = false
				videoCreateTask.Error = pageError.Error()
			}
			// 保存上传处理结果
			writeLogFile(logFile, videoCreateTask, channelName)
			videoCreateTasks[index] = videoCreateTask
		}(videoCreateTask, index)
	}
	wg.Wait()
	log.Println("✅ 所有上传任务完成")
	return videoCreateTasks
}

func createVideo(page *playwright.Page, videoCreateTask VideoCreateTask) VideoCreateTask {

	// 1. 上传视频文件
	err := uploadVideo(*page, videoCreateTask.VideoPath)

	// 2. 填充页面其他字段, 包括点击保存
	if err == nil {
		uploadOptions := VideoUploadOptions{
			Description:  videoCreateTask.Description,
			Location:     videoCreateTask.Location,
			Collection:   videoCreateTask.Collection,
			Link:         videoCreateTask.Link,
			Activity:     videoCreateTask.Activity,
			Schedule:     videoCreateTask.Schedule,
			ScheduleTime: videoCreateTask.ScheduleTime,
			ShortTitle:   videoCreateTask.ShortTitle,
			Action:       videoCreateTask.Action,
		}
		err = completeVideoUploadForm(*page, uploadOptions)
	}
	if err != nil {
		videoCreateTask.Success = false
		videoCreateTask.Error = err.Error()
	} else {
		videoCreateTask.Success = true
	}
	return videoCreateTask
}

// createLogFile 创建日志文件
// createLogFile 创建日志文件
func createLogFile() (*os.File, error) {
	// 确保log目录存在
	logDir := "log"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 创建日志文件
	logFilename := filepath.Join(logDir, fmt.Sprintf("wechat_channel_uploader_%s.log",
		time.Now().Format("20060102_150405")))

	logFile, err := os.Create(logFilename)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %v", err)
	}

	log.Printf("✅ 日志文件创建成功: %s", logFilename)
	return logFile, nil
}

// writeLogFile 写日志文件
func writeLogFile(logFile *os.File, videoCreateTask VideoCreateTask, channelName string) {
	// 记录到日志文件
	logMessage := ""
	if videoCreateTask.Success == false {
		logMessage = fmt.Sprintf("❌ %s: 视频号：%s, 第%d行上传失败: %s - 错误: %v\n",
			time.Now().Format("20060102_150405"), channelName, videoCreateTask.RowIndex, videoCreateTask.VideoPath, videoCreateTask.Error)
	} else {
		logMessage = fmt.Sprintf("✅ %s: 视频号：%s, 第%d行上传成功: %s\n",
			time.Now().Format("20060102_150405"), channelName, videoCreateTask.RowIndex, videoCreateTask.VideoPath)
	}
	logFile.WriteString(logMessage)
}
