package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// isPlaywrightInstalled 确保浏览器环境就绪
func isPlaywrightInstalled() error {
	log.Println("🔍 检查浏览器环境...")

	// 首先检查是否已安装
	if isPlaywrightAlreadyInstalled() {
		log.Println("✅ Playwright 已安装")
		return nil
	}

	// 如果未安装，尝试从 ZIP 文件安装
	log.Println("⚠️ Playwright 未安装，尝试从本地 ZIP 文件安装...")
	if err := installPlaywrightFromZip(); err != nil {
		return fmt.Errorf("自动安装失败: %v", err)
	}

	// 再次验证安装
	if !isPlaywrightAlreadyInstalled() {
		return fmt.Errorf("安装后验证失败")
	}

	log.Println("✅ Playwright 安装完成")
	return nil
}

// isPlaywrightAlreadyInstalled 检查 Playwright 是否已安装
func isPlaywrightAlreadyInstalled() bool {
	// 检查系统默认位置
	playwrightPath := getPlaywrightPath()
	if _, err := os.Stat(playwrightPath); err != nil {
		log.Printf("❌ Playwright 目录不存在: %s", playwrightPath)
		return false
	}

	// 检查是否有 Chromium 浏览器
	chromiumPath := filepath.Join(playwrightPath, "chromium-1169")
	if _, err := os.Stat(chromiumPath); err != nil {
		log.Printf("❌ Chromium 浏览器不存在: %s", chromiumPath)
		return false
	}

	log.Printf("✅ Playwright 已安装: %s", playwrightPath)
	return true
}

// getPlaywrightPath 获取 Playwright 安装路径
func getPlaywrightPath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = os.Getenv("USERPROFILE") + "\\AppData\\Local"
	}
	return filepath.Join(localAppData, "ms-playwright")
}

// installPlaywrightFromZip 从 ZIP 文件安装 Playwright
func installPlaywrightFromZip() error {
	zipPath := getZipFilePath()
	log.Printf("📦 检查 ZIP 文件: %s", zipPath)

	// 检查 ZIP 文件是否存在
	if _, err := os.Stat(zipPath); err != nil {
		return fmt.Errorf("ZIP 文件不存在: %s", zipPath)
	}

	// 创建目标目录
	targetDir := getPlaywrightPath()
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	log.Printf("📁 解压到: %s", targetDir)

	// 解压 ZIP 文件
	if err := unzip(zipPath, targetDir); err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}

	log.Println("✅ ZIP 文件解压完成")
	return nil
}

// getZipFilePath 获取 ZIP 文件路径
func getZipFilePath() string {
	exeDir := getExecutableDir()
	return filepath.Join(exeDir, "ms-playwright.zip")
}

// getExecutableDir 获取可执行文件所在目录
func getExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// unzip 解压 ZIP 文件
func unzip(src, dest string) error {
	log.Printf("🔓 正在解压 %s 到 %s", src, dest)

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// 创建目标目录
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	// 遍历 ZIP 文件中的每个文件/目录
	for _, f := range r.File {
		// 构建目标路径
		fpath := filepath.Join(dest, f.Name)

		// 检查是否是目录
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return err
			}
			continue
		}

		// 创建文件
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		// 关闭文件描述符
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}

		log.Printf("   📄 解压: %s", f.Name)
	}

	return nil
}
