package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/xuri/excelize/v2"
)

// VideoCreateTask 上传任务结构体
type VideoCreateTask struct {
	Description  string
	Location     string
	Collection   string
	Link         string
	Activity     string
	Schedule     bool
	ScheduleTime string
	ShortTitle   string
	Action       string
	VideoPath    string
	RowIndex     int
	Page         *playwright.Page
	ChannelName  string
	Success      bool
	Error        string
}

// ValidateExcelFile 验证Excel文件并解析任务
func ValidateExcelFile(filePath string) ([]VideoCreateTask, error) {
	log.Println("🔍 验证Excel文件格式...")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("文件不存在: %s", filePath)
	}

	// 打开Excel文件
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开Excel文件失败: %v", err)
	}
	defer f.Close()

	// 获取Sheet1
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return nil, fmt.Errorf("读取Sheet1失败: %v", err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("Excel文件没有数据行")
	}

	// 检查表头
	headers := rows[0]
	requiredColumns := map[string]string{
		"保存方式": "I",
		"视频位置": "J",
	}
	// 创建表头映射
	headerMap := make(map[string]int)
	for i, header := range headers {
		headerMap[strings.TrimSpace(header)] = i
	}

	var missingColumns []string
	for name, col := range requiredColumns {
		if _, exists := headerMap[name]; !exists {
			missingColumns = append(missingColumns, fmt.Sprintf("%s列(%s)", col, name))
		}
	}

	if len(missingColumns) > 0 {
		return nil, fmt.Errorf("缺少必要的列: %v", missingColumns)
	}

	log.Printf("✅ 表头验证成功，开始检查数据行...")

	// 解析数据行
	var tasks []VideoCreateTask
	var errors []string

	for i, row := range rows[1:] {
		rowIndex := i + 2 // Excel行号从1开始，表头占1行
		task, err := parseTaskFromRow(row)
		if err != nil {
			errors = append(errors, fmt.Sprintf("第%d行: %v", rowIndex, err))
			continue
		}
		task.RowIndex = rowIndex
		tasks = append(tasks, task)
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("数据行错误:\n%s", strings.Join(errors, "\n"))
	}

	log.Printf("✅ Excel文件验证成功，共 %d 个上传任务", len(tasks))
	return tasks, nil
}

// parseTaskFromRow 从Excel行解析任务
func parseTaskFromRow(row []string) (VideoCreateTask, error) {
	task := VideoCreateTask{}

	// 视频描述 (A列)
	if len(row) > 0 {
		task.Description = strings.TrimSpace(row[0])
	}

	// 位置 (B列)
	if len(row) > 1 {
		task.Location = strings.TrimSpace(row[1])
	}

	// 添加到合集 (C列)
	if len(row) > 2 {
		task.Collection = strings.TrimSpace(row[2])
	}

	// 链接 (D列)
	if len(row) > 3 {
		task.Link = strings.TrimSpace(row[3])
	}

	// 活动 (E列)
	if len(row) > 4 {
		task.Activity = strings.TrimSpace(row[4])
	}

	// 定时发表 (F列)
	if len(row) > 5 {
		schedule := strings.TrimSpace(row[5])
		task.Schedule = schedule == "定时"
	}

	// 定时时间 (G列)
	if row[5] == "定时" && row[6] == "" {
		return task, fmt.Errorf("定时发表时定时时间不能为空")
	}
	if len(row) > 6 {
		task.ScheduleTime = strings.TrimSpace(row[6])
		if row[5] == "定时" && task.ScheduleTime != "" {
			// 直接解析并校验
			targetTime, err := time.Parse("2006/01/2 15:04", task.ScheduleTime)
			if err != nil {
				return task, fmt.Errorf("时间格式错误")
			}

			now := time.Now()
			if targetTime.Before(now) || targetTime.After(now.Add(30*24*time.Hour)) {
				return task, fmt.Errorf("定时时间需要大于当前时间且在一个月内")
			}
		}
	}

	// 短标题 (H列)
	if len(row) > 7 {
		task.ShortTitle = strings.TrimSpace(row[7])
	}

	// 保存方式 (I列) - 必需
	if len(row) > 8 {
		action := strings.TrimSpace(row[8])
		if row[5] == "定时" && row[8] == "保存草稿" {
			return task, fmt.Errorf("定时发表方式必须以发表方式保存")
		}
		switch action {
		case "保存草稿":
			task.Action = "save_draft"
		case "手机预览":
			task.Action = "preview"
		case "发表":
			task.Action = "publish"
		default:
			return task, fmt.Errorf("不支持的保存方式: %s", action)
		}
	} else {
		return task, fmt.Errorf("缺少保存方式")
	}

	// 视频位置 (J列) - 必需
	if len(row) > 9 {
		videoPath := strings.TrimSpace(row[9])
		if videoPath == "" {
			return task, fmt.Errorf("视频位置不能为空")
		}
		// 检查视频文件是否存在
		if exists, err := checkFileExists(videoPath, ""); !exists {
			return task, fmt.Errorf("视频文件不存在: %s, %s", videoPath, err)
		}
		task.VideoPath = videoPath
	} else {
		return task, fmt.Errorf("缺少视频位置")
	}

	return task, nil
}

// 检查文件是否存在（支持相对路径和绝对路径）
func checkFileExists(filename string, extension string) (bool, error) {
	// filepath.Abs 会自动处理相对路径和绝对路径
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return false, fmt.Errorf("无法解析文件路径 %s: %v", filename, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("文件不存在: %s", filename)
		}
		return false, fmt.Errorf("无法访问文件 %s: %v", filename, err)
	}

	// 检查是否是目录
	if info.IsDir() {
		return false, fmt.Errorf("路径是目录而不是文件: %s", filename)
	}

	// 检查文件扩展名
	if extension == "xls" || extension == "xlsx" {
		ext := strings.ToLower(filepath.Ext(absPath))
		if ext != ".xls" && ext != ".xlsx" {
			return false, fmt.Errorf("文件扩展名不是 .xls 或 .xlsx: %s", filename)
		}
	}
	return true, nil
}

// PrintVideoCreateResults 打印上传结果
func PrintVideoCreateResults(results []VideoCreateTask) {
	log.Println("\n📊 ===== 上传结果统计 =====")

	successCount := 0
	failCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
			log.Printf("✅ 第%d行: %s - 成功",
				result.RowIndex, filepath.Base(result.VideoPath))
		} else {
			failCount++
			log.Printf("❌ 第%d行: %s - 失败: %s",
				result.RowIndex, filepath.Base(result.VideoPath), result.Error)
		}
	}

	log.Printf("📈 总计: %d 成功, %d 失败", successCount, failCount)

	if failCount > 0 {
		log.Printf("⚠️ 有 %d 个文件上传失败，详情请查看: wechat_channel_uploader.log", failCount)
	}
}
