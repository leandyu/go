package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const WechatChannelsUploadPage string = "https://channels.weixin.qq.com/platform/post/create"

// PageState 保存页面状态的结构体
type PageState struct {
	Cookies      []playwright.Cookie    `json:"cookies"`
	LocalStorage map[string]interface{} `json:"local_storage"`
	URL          string                 `json:"url"`
}

// VideoUploadOptions 视频上传选项
type VideoUploadOptions struct {
	Description  string
	Location     string
	Collection   string
	Link         string
	Activity     string
	Schedule     bool
	ScheduleTime string
	ShortTitle   string
	Action       string
}

// userLogin 等待用户扫码登录
func waitUserLogin(page playwright.Page) error {
	log.Println("⏳ 等待用户扫码登录...")
	startTime := time.Now()
	maxWait := 600 // 10分钟超时

	for i := 0; i < maxWait; i++ {
		time.Sleep(2 * time.Second)
		elapsed := time.Since(startTime)

		if strings.Contains(page.URL(), "https://channels.weixin.qq.com/platform/") {
			log.Println("✅ 登录跳转至上传页面，登录成功")
			return nil
		}

		remaining := maxWait - int(elapsed.Seconds())
		if remaining > 0 && i%2 == 0 {
			log.Printf("⏰ 剩余扫码时间: %d秒", remaining)
		}

		if elapsed > 600*time.Second {
			return fmt.Errorf("扫码超时，请在5分钟内完成扫码")
		}
	}
	return fmt.Errorf("登录超时")
}

// SaveAuthState 保存认证状态
func SaveAuthState(page playwright.Page, context playwright.BrowserContext) (*PageState, error) {
	cookies, err := context.Cookies()
	if err != nil {
		return nil, fmt.Errorf("获取cookies失败: %v", err)
	}

	authStorage, err := page.Evaluate(`() => {
		const authData = {};
		const authKeys = ['token', 'auth', 'session', 'user', 'login'];
		for (let i = 0; i < localStorage.length; i++) {
			const key = localStorage.key(i);
			for (const authKey of authKeys) {
				if (key.toLowerCase().includes(authKey)) {
					authData[key] = localStorage.getItem(key);
					break;
				}
			}
		}
		return authData;
	}`)
	if err != nil {
		log.Printf("警告: 获取认证存储失败: %v", err)
	}

	pageState := &PageState{
		Cookies:      cookies,
		LocalStorage: convertToMap(authStorage),
		URL:          page.URL(),
	}

	log.Printf("✅ 认证状态保存完成: Cookies=%d个", len(pageState.Cookies))
	return pageState, nil
}

func restoreAuthState(context playwright.BrowserContext, authState *PageState) {
	// 恢复cookies
	if len(authState.Cookies) > 0 {
		optionalCookies := ConvertToOptionalCookies(authState.Cookies)
		if err := context.AddCookies(optionalCookies); err != nil {
			log.Printf("警告: 恢复cookies失败: %v", err)
		}
	}
}

// convertToMap 转换存储数据为map
func convertToMap(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	if dataMap, ok := data.(map[string]interface{}); ok {
		return dataMap
	}
	return result
}

// ConvertToOptionalCookies 转换cookies
func ConvertToOptionalCookies(cookies []playwright.Cookie) []playwright.OptionalCookie {
	optionalCookies := make([]playwright.OptionalCookie, len(cookies))
	for i, cookie := range cookies {
		optionalCookies[i] = playwright.OptionalCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   &cookie.Domain,
			Path:     &cookie.Path,
			Expires:  &cookie.Expires,
			HttpOnly: &cookie.HttpOnly,
			Secure:   &cookie.Secure,
			SameSite: cookie.SameSite,
		}
	}
	return optionalCookies
}

// GenerateBrowser 生成浏览器信息
func GenerateBrowser(headlessMode bool) (*playwright.Playwright, *playwright.Browser, *playwright.BrowserContext, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("启动Playwright失败: %v", err)
	}

	// 启动无头浏览器
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Channel:  playwright.String("chrome"),
		Headless: playwright.Bool(headlessMode),
		Args: []string{
			"--window-size=1920,1080",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("启动浏览器失败: %v", err)
	}

	// 创建上下文
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:  &playwright.Size{Width: 1920, Height: 1080},
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("创建上下文失败: %v", err)
	}

	// 反自动化脚本
	scriptContent := `Object.defineProperty(navigator, 'webdriver', { get: () => false });`
	err = context.AddInitScript(playwright.Script{Content: &scriptContent})

	return pw, &browser, &context, nil
}

// GeneratePage 生成页面信息
func GeneratePage(context *playwright.BrowserContext, isLogin bool) (*playwright.Page, string, error) {
	page, err := (*context).NewPage()
	if err != nil {
		return nil, "", fmt.Errorf("创建页面失败: %v", err)
	}

	// 防止超时
	for i := 0; i < 3; i++ {
		log.Printf("🌐 导航尝试 %d/%d: %s", i+1, 3, WechatChannelsUploadPage)
		_, err = page.Goto(WechatChannelsUploadPage, playwright.PageGotoOptions{
			Timeout:   playwright.Float(60000),                   // 减少超时到60秒
			WaitUntil: playwright.WaitUntilStateDomcontentloaded, // 改为DOMContentLoaded，不等待所有资源
		})
		if err == nil {
			log.Println("✅ 页面导航成功")
			break
		}
		log.Printf("⚠️ 导航失败 (尝试 %d): %v", i+1, err)
		if i <= 3 {
			waitTime := time.Duration(i+1) * 10 * time.Second
			log.Printf("⏳ 等待 %v 后重试...", waitTime)
			time.Sleep(waitTime)
			// 刷新页面重试
			page.Reload()
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("页面创建失败: %v", err)
	}

	// 用户扫码时需要等待扫码
	if isLogin {
		if err = waitUserLogin(page); err != nil {
			return nil, "", fmt.Errorf("登录失败: %v", err)
		}
	} else {
		// 上传视频时需要检查页面是否就绪
		time.Sleep(5 * time.Second)
		if err := waitForPageReady(page); err != nil {
			return nil, "", fmt.Errorf("页面加载失败: %v", err)
		}
		if !isLoggedIn(page) {
			return &page, "", fmt.Errorf("登录信息失效: %v", err)
		}
		// 获取视频号名称
		channnelName := getCurrentChannelName(page)
		return &page, channnelName, nil
	}
	return &page, "", nil
}

// waitForPageReady 等待页面完全就绪
func waitForPageReady(page playwright.Page) error {
	log.Println("🔍 检查页面状态...")

	maxWait := 30
	for i := 0; i < maxWait; i++ {
		currentURL := page.URL()
		title, _ := page.Title()
		log.Printf("🌐 当前URL: %s", currentURL)
		log.Printf("📄 页面标题: %s", title)

		// 检查是否在正确的页面
		if !isCorrectPage(page) {
			return fmt.Errorf("不在正确的上传页面")
		}

		// 检查页面关键元素
		if isUploadPageReady(page) {
			log.Println("✅ 页面已就绪")
			return nil
		}

		log.Printf("⏳ 等待页面元素加载... (%d/%d)", i+1, maxWait)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("页面加载超时")
}

// 获取当前登录的视频号名称
func getCurrentChannelName(page playwright.Page) string {
	// 专门针对您提供的HTML结构的选择器
	specificSelectors := []string{
		".common-menu-item.account-info .account-info .name",
		".account-info .name",
		"[class*='account-info'] .name",
		".left-part .name",
		// 直接使用数据属性
		"[data-v-5271c1f2] .name",
		"[data-v-02b98fb1] .name",
	}

	for _, selector := range specificSelectors {
		if name, found := getTextFromSelector(page, selector); found {
			log.Printf("✅ 通过选择器找到视频号名称: %s -> %s", selector, name)
			return name
		}
	}

	// 如果上面的选择器不行，尝试更精确的定位
	exactSelectors := []string{
		".common-menu-item.default .account-info .name",
		"[class*='common-menu-item'][class*='account-info'] .name",
	}

	for _, selector := range exactSelectors {
		if name, found := getTextFromSelector(page, selector); found {
			log.Printf("✅ 通过精确选择器找到视频号名称: %s -> %s", selector, name)
			return name
		}
	}

	// 调试：打印页面相关HTML来帮助诊断
	log.Println("🔍 尝试调试模式...")
	debugSelectors := []string{
		".common-menu-item",
		".account-info",
		".left-part",
	}

	for _, selector := range debugSelectors {
		element, err := page.QuerySelector(selector)
		if err == nil && element != nil {
			html, _ := element.InnerHTML()
			log.Printf("🔍 选择器 %s 的内容: %s", selector, html)
		}
	}

	return ""
}

// 辅助函数：从选择器获取文本
func getTextFromSelector(page playwright.Page, selector string) (string, bool) {
	element, err := page.QuerySelector(selector)
	if err != nil || element == nil {
		return "", false
	}

	text, err := element.TextContent()
	if err != nil || strings.TrimSpace(text) == "" {
		return "", false
	}

	return strings.TrimSpace(text), true
}

// completeVideoUploadForm 完整的表单填写方法
func completeVideoUploadForm(page playwright.Page, options VideoUploadOptions) error {
	log.Println("=== 开始自动填写视频上传表单 ===")

	// 1. 填写视频描述
	if options.Description != "" {
		log.Println("📝 填写视频描述...")
		descSelector := ".input-editor[contenteditable][data-placeholder='添加描述']"
		if err := page.Locator(descSelector).First().Click(); err != nil {
			return fmt.Errorf("点击描述输入框失败: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		if err := page.Locator(descSelector).First().Fill(options.Description); err != nil {
			return fmt.Errorf("填写描述失败: %v", err)
		}
		log.Println("✅ 视频描述填写成功")
	}

	// 2. 选择位置
	if options.Location != "" {
		log.Printf("📍 选择位置: %s", options.Location)
		if err := selectLocation(page, options.Location); err != nil {
			log.Printf("⚠️ 选择位置失败: %v", err)
		}
	}

	// 3. 选择或创建合集
	if options.Collection != "" {
		log.Printf("📚 处理合集: %s", options.Collection)
		if err := handleCollection(page, options.Collection); err != nil {
			log.Printf("⚠️ 处理合集失败: %v", err)
		}
	}

	// 4. 选择链接
	if options.Link != "" {
		log.Printf("🔗 选择链接类型: %s", options.Link)
		if err := selectLink(page, options.Link); err != nil {
			log.Printf("⚠️ 选择链接失败: %v", err)
		}
	}

	// 5. 选择活动
	if options.Activity != "" {
		log.Printf("🎯 选择活动: %s", options.Activity)
		if err := selectActivity(page, options.Activity); err != nil {
			log.Printf("⚠️ 选择活动失败: %v", err)
		}
	}

	// 6. 设置定时发表
	if options.Schedule {
		log.Println("⏰ 设置定时发表...")
		if err := setScheduledPublish(page, options.ScheduleTime); err != nil {
			return fmt.Errorf("设置定时发表失败: %v", err)
		}
		log.Println("✅ 定时发表设置成功")
	}

	// 7. 填写短标题
	if options.ShortTitle != "" {
		log.Println("🏷️ 填写短标题...")
		if err := fillShortTitle(page, options.ShortTitle); err != nil {
			return fmt.Errorf("填写短标题失败: %v", err)
		}
		log.Println("✅ 短标题填写成功")
	}

	// 8. 执行最终操作
	if options.Action != "" {
		log.Printf("🚀 执行最终操作: %s", options.Action)
		if err := performFinalAction(page, options.Action, options.Schedule); err != nil {
			return fmt.Errorf("执行最终操作失败: %v", err)
		}
		log.Printf("✅ %s 操作成功", getActionName(options.Action))
	}

	log.Println("🎉 表单自动填写完成！")
	return nil
}

// selectLocation 选择位置
func selectLocation(page playwright.Page, location string) error {
	// 点击位置选择器
	locationSelector := ".post-position-wrap .position-display"
	if err := page.Locator(locationSelector).First().Click(); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	if location == "不显示位置" {
		// 选择"不显示位置"
		if err := page.Locator(".location-filter-wrap .option-item.active").First().Click(); err != nil {
			return err
		}
	} else {
		// 搜索并选择具体位置
		searchInput := ".location-filter-wrap input[placeholder='搜索附近位置']"
		if err := page.Locator(searchInput).First().Fill(location); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)

		// 选择第一个匹配的位置
		locationItem := fmt.Sprintf(".location-item:has-text('%s')", location)
		if count, _ := page.Locator(locationItem).Count(); count > 0 {
			if err := page.Locator(locationItem).First().Click(); err != nil {
				return err
			}
		} else {
			// 如果没有精确匹配，选择第一个结果
			if err := page.Locator(".location-filter-wrap .option-item:not(.active)").First().Click(); err != nil {
				return err
			}
		}
	}

	time.Sleep(1 * time.Second)
	return nil
}

// handleCollection 处理合集
func handleCollection(page playwright.Page, collection string) error {
	// 点击合集选择器
	collectionSelector := ".post-album-display"
	if err := page.Locator(collectionSelector).First().Click(); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	if collection == "创建新合集" {
		// 点击创建新合集
		if err := page.Locator(".filter-wrap .create a").First().Click(); err != nil {
			return err
		}
		time.Sleep(1 * time.Second)

		// 填写合集标题
		titleInput := ".weui-desktop-dialog__wrp input[placeholder='有趣的合集标题更容易吸引粉丝']"
		if err := page.Locator(titleInput).First().Fill("我的视频合集"); err != nil {
			return err
		}

		// 点击创建按钮（等待按钮可用）
		createBtn := ".weui-desktop-dialog__ft .weui-desktop-btn_primary:not(.weui-desktop-btn_disabled)"
		if err := page.Locator(createBtn).First().WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		}); err != nil {
			return err
		}

		if err := page.Locator(createBtn).First().Click(); err != nil {
			return err
		}

		// 等待创建成功并关闭对话框
		time.Sleep(2 * time.Second)
		confirmBtn := ".create-success-dialog .weui-desktop-btn_primary"
		if err := page.Locator(confirmBtn).First().Click(); err != nil {
			return err
		}
	} else {
		// 选择现有合集（这里需要根据实际合集列表调整）
		log.Printf("⚠️ 选择现有合集: %s (需要根据实际页面调整)", collection)
		// 这里可以添加选择现有合集的逻辑
	}

	time.Sleep(1 * time.Second)
	return nil
}

// selectLink 选择链接类型
func selectLink(page playwright.Page, linkType string) error {
	// 点击链接选择器
	linkSelector := ".post-link-wrap .link-display-wrap"
	if err := page.Locator(linkSelector).First().Click(); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	// 选择链接类型
	var linkOption string
	switch linkType {
	case "公众号文章":
		linkOption = ".link-option-item:has-text('公众号文章')"
	case "红包封面":
		linkOption = ".link-option-item:has-text('红包封面')"
	default:
		return fmt.Errorf("不支持的链接类型: %s", linkType)
	}

	if err := page.Locator(linkOption).First().Click(); err != nil {
		return err
	}

	time.Sleep(1 * time.Second)
	return nil
}

// selectActivity 选择活动
func selectActivity(page playwright.Page, activity string) error {
	// 点击活动选择器
	activitySelector := ".post-activity-wrap .activity-display"
	if err := page.Locator(activitySelector).First().Click(); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	if activity == "不参与活动" {
		// 选择"不参与活动"
		if err := page.Locator(".activity-filter-wrap .option-item.active").First().Click(); err != nil {
			return err
		}
	} else {
		// 搜索并选择活动
		searchInput := ".activity-filter-wrap input[placeholder='搜索活动']"
		if err := page.Locator(searchInput).First().Fill(activity); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)

		// 选择活动（这里需要根据实际搜索结果调整）
		activityItem := fmt.Sprintf(".activity-item:has-text('%s')", activity)
		if count, _ := page.Locator(activityItem).Count(); count > 0 {
			if err := page.Locator(activityItem).First().Click(); err != nil {
				return err
			}
		}
	}

	time.Sleep(1 * time.Second)
	return nil
}

// fillShortTitle 填写短标题
func fillShortTitle(page playwright.Page, title string) error {
	shortTitleSelectors := []string{
		".short-title-wrap input.weui-desktop-form__input",
		"input[placeholder*='概括视频主要内容']",
		".post-short-title-wrap input",
	}

	for _, selector := range shortTitleSelectors {
		if count, _ := page.Locator(selector).Count(); count > 0 {
			// 等待元素可见
			if err := page.Locator(selector).First().WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			}); err != nil {
				continue
			}

			// 点击确保焦点
			if err := page.Locator(selector).First().Click(); err != nil {
				continue
			}
			time.Sleep(500 * time.Millisecond)

			// 清空并填写
			if err := page.Locator(selector).First().Fill(""); err != nil {
				continue
			}
			time.Sleep(300 * time.Millisecond)

			if err := page.Locator(selector).First().Fill(title); err != nil {
				continue
			}

			// 验证填写成功
			time.Sleep(500 * time.Millisecond)
			value, err := page.Locator(selector).First().InputValue()
			if err == nil && value == title {
				return nil
			}
		}
	}

	return fmt.Errorf("无法填写短标题")
}

// getActionName 获取操作名称
func getActionName(action string) string {
	switch action {
	case "save_draft":
		return "保存草稿"
	case "preview":
		return "手机预览"
	case "publish":
		return "发表"
	default:
		return action
	}
}

// isCorrectPage 检查是否在正确的页面
func isCorrectPage(page playwright.Page) bool {
	currentURL := page.URL()

	// 检查URL是否包含上传页面的特征
	if strings.Contains(currentURL, "channels.weixin.qq.com") &&
		(strings.Contains(currentURL, "platform/post/create") || strings.Contains(currentURL, "create")) {
		return true
	}

	// 检查页面内容
	bodyLocator := page.Locator("body")
	bodyText, err := bodyLocator.TextContent()
	if err == nil {
		// 检查页面是否包含上传相关文本
		uploadTexts := []string{"上传视频", "保存草稿", "发表", "创作"}
		for _, text := range uploadTexts {
			if strings.Contains(bodyText, text) {
				log.Printf("✅ 页面包含上传文本: %s", text)
				return true
			}
		}
	}

	log.Printf("❌ 不在正确的上传页面，当前URL: %s", currentURL)
	return false
}

// isUploadPageReady 检查上传页面是否就绪
func isUploadPageReady(page playwright.Page) bool {
	// 放宽检查条件，只要找到任何上传相关元素即可
	uploadSelectors := []string{
		"input[type='file']",
		".ant-upload",
		"button:has-text('保存草稿')",
		"button:has-text('发表')",
		"text=上传视频",
		"[class*='upload']",
	}

	for _, selector := range uploadSelectors {
		count, _ := page.Locator(selector).Count()
		if count > 0 {
			log.Printf("✅ 找到上传元素: %s (数量: %d)", selector, count)
			return true
		}
	}

	log.Println("❌ 未找到上传相关元素")
	return false
}

// isLoggedIn 检查是否已登录
func isLoggedIn(page playwright.Page) bool {
	// 检查登录状态指示器
	loggedInSelectors := []string{
		".ant-upload", // 上传组件
		"text=保存草稿",   // 保存按钮
		"text=发表",     // 发表按钮
		"text=上传视频",   // 上传文字
	}

	for _, selector := range loggedInSelectors {
		locator := page.Locator(selector)
		if visible, _ := locator.First().IsVisible(); visible {
			return true
		}
	}

	// 检查是否在登录页面
	loginSelectors := []string{
		".qrcode",        // 二维码
		"text=扫码登录",      // 登录文字
		"text=请使用微信扫码登录", // 登录提示
	}

	for _, selector := range loginSelectors {
		locator := page.Locator(selector)
		if visible, _ := locator.First().IsVisible(); visible {
			return false
		}
	}

	// 默认认为已登录（避免误判）
	return true
}

// 🔥 优化：uploadVideo 方法，添加重试机制
func uploadVideo(page playwright.Page, videoPath string) error {
	log.Println("=== 开始上传文件 ===")

	// 直接设置文件上传（带重试）
	maxRetries := 1
	for i := 0; i < maxRetries; i++ {
		log.Printf("🔄 上传尝试 %d/%d", i+1, maxRetries)

		err := uploadVideoBySelector(page, videoPath)
		if err == nil {
			return nil
		}
		if err != nil && i >= maxRetries-1 {
			return err
		}

		if i < maxRetries-1 {
			log.Println("⏳ 上传失败，等待后重试...")
			time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("所有上传方法都失败")
}

// 🔥 优化：uploadVideoBySelector 方法，通过选择器进行上传视频
func uploadVideoBySelector(page playwright.Page, videoPath string) error {

	log.Println("直接设置文件输入框...")
	log.Printf("文件路径：%s", videoPath)

	// 等待页面稳定
	time.Sleep(5 * time.Second)

	// 尝试让隐藏的文件输入框可见
	_, err := page.Evaluate(`() => {
        const fileInputs = document.querySelectorAll('input[type="file"]');
        fileInputs.forEach(input => {
            // 移除可能阻止操作的样式
            input.style.display = 'block';
            input.style.visibility = 'visible';
            input.style.opacity = '1';
            input.style.position = 'static';
            input.style.width = '100px';
            input.style.height = '30px';
        });
        return fileInputs.length;
    }`)
	if err != nil {
		log.Printf("⚠️ 调整文件输入框样式失败: %v", err)
	}

	// 更多选择器尝试
	selectors := []string{
		"input[type='file']",
		"input[accept*='video']",
		"input[accept*='mp4']",
		"input[name='file']",
		".ant-upload input",
		"input.ant-upload",
		"[class*='upload'] input[type='file']",
		"input[type='file'][accept*='video']",
	}

	var fileInput playwright.Locator
	foundSelector := ""

	for _, selector := range selectors {
		fileInput = page.Locator(selector)
		if count, _ := fileInput.Count(); count > 0 {
			foundSelector = selector
			log.Printf("✅ 找到文件输入框: %s (数量: %d)", selector, count)
			break
		}
	}

	if foundSelector == "" {
		return fmt.Errorf("未找到任何文件输入框")
	}

	// 设置文件
	log.Printf("📁 设置文件: %s", videoPath)
	if err := fileInput.SetInputFiles([]string{videoPath}); err != nil {
		return fmt.Errorf("设置文件失败: %v", err)
	}

	log.Println("✅ 文件设置成功，等待上传开始...")

	// 检查上传状态
	return checkVideoUploadStatus(page)

}

// 检查上传状态
func checkVideoUploadStatus(page playwright.Page) error {
	log.Println("=== 监控上传状态 ===")

	// 方法1: 等待删除按钮出现（最可靠）
	if err := waitForDeleteButton(page); err == nil {
		log.Println("✅ 基于删除按钮检测，上传完成")
		return nil
	} else {
		log.Printf("⚠️ 删除按钮检测失败: %v", err)
		return err
	}

	return fmt.Errorf("上传超时")
}

// waitForDeleteButton 等待删除按钮出现
func waitForDeleteButton(page playwright.Page) error {
	log.Println("⏳ 等待删除按钮出现...")

	startTime := time.Now()
	maxWait := 120 // 2分钟

	for i := 0; i < maxWait; i++ {
		time.Sleep(2 * time.Second)

		// 检查删除按钮
		if hasDeleteButton(page) {
			elapsed := time.Since(startTime)
			log.Printf("✅ 删除按钮出现！等待时间: %v", elapsed)
			return nil
		}

		// 检查上传错误
		if hasUploadError(page) {
			return fmt.Errorf("上传过程中出现错误")
		}

		/*		// 定期报告状态
				if (i+1)%5 == 0 {
					progress := getCurrentProgress(page)
					elapsed := time.Since(startTime)
					log.Printf("⏳ 等待上传完成... 进度: %s, 已等待: %v", progress, elapsed)
				}*/

		// 检查超时
		if time.Since(startTime) > 5*time.Minute {
			return fmt.Errorf("等待删除按钮超时（5分钟）")
		}
	}

	return fmt.Errorf("等待删除按钮超时")
}

// hasDeleteButton 检查是否有删除按钮
func hasDeleteButton(page playwright.Page) bool {
	deleteSelectors := []string{
		".finder-tag-wrap .tag-inner:has-text('删除')",
		".ant-upload-list-item .anticon-delete",
		"button:has-text('删除')",
		"[title*='删除']",
		"[class*='delete'][class*='btn']",
	}

	for _, selector := range deleteSelectors {
		if count, _ := page.Locator(selector).Count(); count > 0 {
			if visible, _ := page.Locator(selector).First().IsVisible(); visible {
				// 额外验证：删除按钮应该是可点击的
				if enabled, _ := page.Locator(selector).First().IsEnabled(); enabled {
					log.Printf("✅ 检测到可用的删除按钮: %s", selector)
					return true
				}
			}
		}
	}
	return false
}

// 🔥 新增：检查上传错误
func hasUploadError(page playwright.Page) bool {
	errorSelectors := []string{
		".ant-upload-list-item-error",
		".ant-alert-error",
		".upload-error",
		"text=上传失败",
		"text=格式不支持",
		"text=文件过大",
		"text=网络错误",
		"[class*='error']",
	}

	for _, selector := range errorSelectors {
		locator := page.Locator(selector)
		if visible, _ := locator.First().IsVisible(); visible {
			// 如果URL不对，检查页面内容 - 🔥 使用 Locator
			bodyLocator := page.Locator("body")
			_, errorText := bodyLocator.TextContent()
			if errorText != nil {
				log.Printf("🚨 检测到上传错误: %s - %s", selector, errorText)
				return true
			}
		}
	}
	return false
}

// performFinalAction 执行最终操作 - 修复版本
func performFinalAction(page playwright.Page, action string, isScheduled bool) error {
	var buttonSelector string
	var actionName string

	switch action {
	case "save_draft":
		buttonSelector = ".form-btns button:has-text('保存草稿')"
		actionName = "保存草稿"
		// 检查定时发表时的限制
		if isScheduled {
			log.Println("⚠️ 定时发表时无法保存草稿，将尝试取消定时发表")
			// 取消定时发表
			if err := cancelScheduledPublish(page); err != nil {
				return fmt.Errorf("取消定时发表失败: %v", err)
			}
			time.Sleep(2 * time.Second)
		}
	case "preview":
		buttonSelector = ".form-btns button:has-text('手机预览')"
		actionName = "手机预览"
	case "publish":
		buttonSelector = ".form-btns button:has-text('发表')"
		actionName = "发表"
	default:
		return fmt.Errorf("不支持的操作类型: %s", action)
	}

	log.Printf("🎯 准备执行操作: %s", actionName)

	// 方法1: 等待按钮可用并点击
	if err := waitAndClickButton(page, buttonSelector, actionName); err == nil {
		return waitForActionCompletion(page, action, actionName)
	}

	return waitForActionCompletion(page, action, actionName)
}

// cancelScheduledPublish 取消定时发表
func cancelScheduledPublish(page playwright.Page) error {
	// 尝试点击"不定时"单选按钮
	if err := page.Locator("input.weui-desktop-form__radio[value='0']").First().Click(); err != nil {
		return fmt.Errorf("点击不定时单选按钮失败: %v", err)
	}

	// 等待状态更新
	time.Sleep(2 * time.Second)

	// 验证是否取消成功
	isChecked, err := page.Locator("input.weui-desktop-form__radio[value='0']").First().IsChecked()
	if err != nil {
		return fmt.Errorf("检查单选按钮状态失败: %v", err)
	}

	if !isChecked {
		return fmt.Errorf("取消定时发表失败，按钮状态未更新")
	}

	log.Println("✅ 定时发表已取消")
	return nil
}

// waitAndClickButton 等待按钮可用并点击
func waitAndClickButton(page playwright.Page, selector string, actionName string) error {
	log.Printf("⏳ 等待 %s 按钮可用...", actionName)

	// 等待按钮可见
	if err := page.Locator(selector).First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(15000), // 15秒超时
	}); err != nil {
		return fmt.Errorf("等待 %s 按钮可见失败: %v", actionName, err)
	}

	// 检查按钮是否启用
	isEnabled, err := page.Locator(selector).First().IsEnabled()
	if err != nil {
		return fmt.Errorf("检查 %s 按钮状态失败: %v", actionName, err)
	}

	if !isEnabled {
		return fmt.Errorf("%s 按钮处于禁用状态", actionName)
	}

	// 检查是否包含禁用类
	hasDisabledClass, err := page.Locator(selector).First().GetAttribute("class")
	if err == nil && strings.Contains(hasDisabledClass, "weui-desktop-btn_disabled") {
		return fmt.Errorf("%s 按钮有禁用样式", actionName)
	}

	log.Printf("🖱️ 点击 %s 按钮...", actionName)

	// 使用JavaScript点击，更可靠
	clicked, err := page.Locator(selector).First().Evaluate(`(button) => {
        try {
            button.scrollIntoView({ behavior: 'smooth', block: 'center' });
            button.click();
            return true;
        } catch (e) {
            console.error('点击失败:', e);
            return false;
        }
    }`, nil)

	if err != nil || !clicked.(bool) {
		return fmt.Errorf("JavaScript点击 %s 按钮失败: %v", actionName, err)
	}

	log.Printf("✅ %s 按钮点击成功", actionName)
	return nil
}

// waitForActionCompletion 等待操作完成
func waitForActionCompletion(page playwright.Page, action string, actionName string) error {
	log.Printf("⏳ 等待 %s 操作完成...", actionName)

	maxWait := 300 // 30秒超时
	for i := 0; i < maxWait; i++ {
		time.Sleep(1 * time.Second)

		// 检查操作成功
		if isActionSuccessful(page, action) {
			log.Printf("✅ %s 操作成功完成", actionName)
			return nil
		}

		// 检查操作失败
		if hasActionFailed(page, action) {
			return fmt.Errorf("%s 操作失败", actionName)
		}

		if (i+1)%5 == 0 {
			log.Printf("⏳ 等待 %s 操作完成... (%d/%d)", actionName, i+1, maxWait)
		}
	}
	// 最后再检查一次，避免在最后一次sleep时完成
	if isActionSuccessful(page, action) {
		log.Printf("✅ %s 操作成功完成", actionName)
		return nil
	}
	return fmt.Errorf("%s 操作超时", actionName)
}

// isActionSuccessful 检查操作是否成功
func isActionSuccessful(page playwright.Page, action string) bool {
	switch action {
	case "save_draft":
		// 保存草稿成功的指示器
		successIndicators := []string{
			"text=已保存",
			"text=保存成功",
			"text=保存完成",
			"text=草稿保存成功",
			".ant-message-success",
			".weui-desktop-message--success",
			"[class*='success']",
		}

		for _, selector := range successIndicators {
			// 快速检查，不等待
			if count, _ := page.Locator(selector).Count(); count > 0 {
				if visible, _ := page.Locator(selector).First().IsVisible(); visible {
					log.Printf("✅ 检测到保存成功提示: %s", selector)
					return true
				}
			}
		}

	case "publish":
		// 发表成功的指示器
		successIndicators := []string{
			"text=已发表",
			"text=发表成功",
			"text=发布成功",
			"text=视频已发布",
			".ant-message-success",
			".weui-desktop-message--success",
		}

		for _, selector := range successIndicators {
			if count, _ := page.Locator(selector).Count(); count > 0 {
				if visible, _ := page.Locator(selector).First().IsVisible(); visible {
					log.Printf("✅ 检测到发表成功提示: %s", selector)
					return true
				}
			}
		}

	case "preview":
		// 预览成功的指示器（通常是弹窗或新页面）
		if visible, _ := page.Locator(".weui-desktop-dialog, [role='dialog'], .preview-dialog").First().IsVisible(); visible {
			log.Println("✅ 检测到预览弹窗")
			return true
		}
	}

	return false
}

// hasActionFailed 检查操作是否失败
func hasActionFailed(page playwright.Page, action string) bool {
	errorIndicators := []string{
		"text=保存失败",
		"text=发表失败",
		"text=操作失败",
		"text=网络错误",
		".ant-message-error",
		".weui-desktop-message--error",
		"[class*='error']",
	}

	for _, selector := range errorIndicators {
		if count, _ := page.Locator(selector).Count(); count > 0 {
			if visible, _ := page.Locator(selector).First().IsVisible(); visible {
				errorText, _ := page.Locator(selector).First().TextContent()
				log.Printf("🚨 检测到操作失败: %s - %s", selector, errorText)
				return true
			}
		}
	}
	return false
}

// setScheduledPublish 设置定时发表
func setScheduledPublish(page playwright.Page, scheduleTime string) error {
	log.Println("⏰ 开始设置定时发表...")

	// 方法1: 点击包含radio的label（正确方法）
	timingSelectors := []string{
		"//label[.//span[contains(text(), '定时')]]",
		"//label[.//input[@value='1']]",
		"label:has(input[value='1'])",
		".weui-desktop-form__check-label:has(input[value='1'])",
	}

	var timingLocator playwright.Locator
	var found bool

	// 尝试多种选择器
	for _, selector := range timingSelectors {
		if strings.HasPrefix(selector, "//") {
			timingLocator = page.Locator(fmt.Sprintf("xpath=%s", selector))
		} else {
			timingLocator = page.Locator(selector)
		}

		if count, err := timingLocator.Count(); err == nil && count > 0 {
			log.Printf("✅ 找到定时按钮label，选择器: %s", selector)
			found = true
			break
		}
	}

	if !found {
		// 方法2: 直接点击radio input
		radioSelector := "input.weui-desktop-form__radio[value='1']"
		radioLocator := page.Locator(radioSelector)
		if count, err := radioLocator.Count(); err == nil && count > 0 {
			log.Printf("✅ 找到radio按钮，直接点击")
			timingLocator = radioLocator
			found = true
		}
	}

	if !found {
		log.Println("❌ 未找到定时发表相关元素")
		debugScheduledPublishElements(page)
		return fmt.Errorf("未找到定时发表按钮")
	}

	// 确保元素可见
	if err := timingLocator.First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(30000),
	}); err != nil {
		log.Printf("❌ 等待元素可见失败: %v", err)
		return fmt.Errorf("定时发表按钮不可见: %v", err)
	}

	// 滚动到元素可见
	if err := timingLocator.First().ScrollIntoViewIfNeeded(); err != nil {
		log.Printf("⚠️ 滚动失败: %v", err)
	}
	time.Sleep(1 * time.Second)

	// 获取元素信息用于调试
	bbox, err := timingLocator.First().BoundingBox()
	if err == nil {
		log.Printf("📊 元素位置: x=%.0f, y=%.0f, width=%.0f, height=%.0f",
			bbox.X, bbox.Y, bbox.Width, bbox.Height)
	}

	// 点击前先检查当前状态
	radioLocator := page.Locator("input.weui-desktop-form__radio[value='1']")
	isCheckedBefore, _ := radioLocator.First().IsChecked()
	log.Printf("🔍 点击前radio状态: %t", isCheckedBefore)

	// 点击操作
	log.Println("🖱️ 点击定时发表按钮...")
	if err := timingLocator.First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
		Force:   playwright.Bool(true),
	}); err != nil {
		log.Printf("❌ 点击失败: %v", err)
		return fmt.Errorf("点击定时发表按钮失败: %v", err)
	}

	log.Println("✅ 点击完成，等待页面响应...")
	time.Sleep(3 * time.Second)

	// 验证是否选中
	isCheckedAfter, err := radioLocator.First().IsChecked()
	if err != nil {
		log.Printf("⚠️ 检查radio状态失败: %v", err)
	} else {
		log.Printf("🔍 点击后radio状态: %t", isCheckedAfter)
		if !isCheckedAfter {
			log.Println("❌ radio未选中，尝试其他方法...")
			// 尝试直接设置radio的checked属性
			if _, err := radioLocator.First().Evaluate(`element => {
					element.checked = true;
					const event = new Event('change', { bubbles: true });
					element.dispatchEvent(event);
				}`, nil); err != nil {
				log.Printf("⚠️ JavaScript设置失败: %v", err)
			} else {
				log.Println("✅ 通过JavaScript设置radio选中")
				time.Sleep(2 * time.Second)
			}
		} else {
			log.Println("✅ 定时发表已成功选中")
		}
	}

	// 检查时间选择器是否出现
	timePickerSelector := "input[placeholder='请选择发表时间']"
	timePickerLocator := page.Locator(timePickerSelector)
	if count, _ := timePickerLocator.Count(); count > 0 {
		log.Println("✅ 时间选择器输入框已出现")
		isVisible, _ := timePickerLocator.First().IsVisible()
		log.Printf("🔍 时间选择器可见性: %t", isVisible)
	} else {
		log.Println("⚠️ 时间选择器输入框未出现")
	}

	// 如果有定时时间，设置具体时间
	if scheduleTime != "" {
		log.Printf("⏰ 设置定时时间: %s", scheduleTime)
		if err := setScheduleTime(page, scheduleTime); err != nil {
			return fmt.Errorf("设置定时时间失败: %v", err)
		}
		log.Println("✅ 定时时间设置成功")
	}

	return nil
}

// debugScheduledPublishElements 调试定时发表相关元素
func debugScheduledPublishElements(page playwright.Page) {
	log.Println("🔍 调试定时发表相关元素...")

	// 查找所有相关元素
	elements := page.Locator(".weui-desktop-form__check-label, input[type='radio'], .weui-desktop-form__check-content")
	if count, err := elements.Count(); err == nil {
		log.Printf("📊 找到 %d 个相关元素", count)

		for i := 0; i < count; i++ {
			element := elements.Nth(i)
			tag, _ := element.Evaluate("el => el.tagName", nil)
			text, _ := element.TextContent()
			html, _ := element.InnerHTML()

			log.Printf("  元素 %d: <%s> 文本: '%s'", i+1, tag, strings.TrimSpace(text))
			log.Printf("    HTML: %s", html)

			// 检查radio相关属性
			if tag == "INPUT" {
				value, _ := element.GetAttribute("value")
				checked, _ := element.IsChecked()
				log.Printf("    Radio属性: value=%s, checked=%t", value, checked)
			}
		}
	}
}

// setScheduleTime 设置具体的定时时间
func setScheduleTime(page playwright.Page, scheduleTime string) error {
	log.Printf("⏰ 设置定时时间: '%s'", scheduleTime)

	// 直接使用正则提取所有数字，然后重新构建
	re := regexp.MustCompile(`\d+`)
	numbers := re.FindAllString(scheduleTime, -1)

	if len(numbers) >= 5 {
		// 假设格式: 年 月 日 时 分
		year := numbers[0]
		month := fmt.Sprintf("%02s", numbers[1])
		day := fmt.Sprintf("%02s", numbers[2])
		hour := fmt.Sprintf("%02s", numbers[3])
		minute := fmt.Sprintf("%02s", numbers[4])

		formattedTime := fmt.Sprintf("%s/%s/%s %s:%s", year, month, day, hour, minute)
		log.Printf("🔧 重新构建的时间: %s", formattedTime)

		targetTime, err := time.Parse("2006/01/02 15:04", formattedTime)
		if err != nil {
			return fmt.Errorf("解析时间失败: %v", err)
		}

		log.Printf("✅ 时间解析成功: %s", targetTime.Format("2006-01-02 15:04:05"))
		return setDateTimePicker(page, targetTime)
	}

	return fmt.Errorf("无法从字符串中提取时间信息: %s", scheduleTime)
}

// setDateTimePicker 设置日期时间选择器
func setDateTimePicker(page playwright.Page, targetTime time.Time) error {
	log.Printf("📅 开始设置日期时间: %s", targetTime.Format("2006-01-02 15:04"))

	// 点击日期时间选择器输入框
	dateTimePickerSelector := "input[placeholder='请选择发表时间']"
	dateTimeLocator := page.Locator(dateTimePickerSelector).First()

	if err := dateTimeLocator.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("点击日期时间选择器失败: %v", err)
	}

	log.Println("✅ 日期时间选择器点击成功")
	time.Sleep(3 * time.Second)

	// 检测当前打开的面板类型并设置日期时间
	if err := detectAndSetDateTime(page, targetTime); err != nil {
		return fmt.Errorf("设置日期时间失败: %v", err)
	}

	// 确认选择
	if err := confirmDateTimeSelection(page); err != nil {
		return fmt.Errorf("确认时间选择失败: %v", err)
	}

	return nil
}

// detectAndSetDateTime 检测面板类型并设置日期时间
func detectAndSetDateTime(page playwright.Page, targetTime time.Time) error {
	// 检测当前显示的面板类型
	panelTypes := []string{
		".weui-desktop-picker__panel_year",  // 年份选择面板
		".weui-desktop-picker__panel_month", // 月份选择面板
		".weui-desktop-picker__panel_day",   // 日期选择面板
	}

	var currentPanel string
	for _, panelType := range panelTypes {
		locator := page.Locator(panelType)
		count, err := locator.Count()
		if err != nil {
			log.Printf("⚠️ 检查面板 %s 失败: %v", panelType, err)
			continue
		}
		if count > 0 {
			currentPanel = panelType
			log.Printf("🔍 检测到当前面板: %s", panelType)
			break
		}
	}

	if currentPanel == "" {
		log.Println("⚠️ 未检测到面板类型，尝试默认日期设置")
		return setFullDateTime(page, targetTime)
	}

	// 根据面板类型进行设置
	switch currentPanel {
	case ".weui-desktop-picker__panel_year":
		log.Println("📅 当前在年份选择面板")
		return setDateTimeFromYearPanel(page, targetTime)
	case ".weui-desktop-picker__panel_month":
		log.Println("📅 当前在月份选择面板")
		return setDateTimeFromMonthPanel(page, targetTime)
	case ".weui-desktop-picker__panel_day":
		log.Println("📅 当前在日期选择面板")
		return setDateTimeFromDayPanel(page, targetTime)
	default:
		return setFullDateTime(page, targetTime)
	}
}

// setDateTimeFromYearPanel 从年份面板开始设置完整日期时间
func setDateTimeFromYearPanel(page playwright.Page, targetTime time.Time) error {
	year := targetTime.Year()

	log.Printf("🗓️ 设置年份: %d", year)

	// 选择目标年份
	yearStr := fmt.Sprintf("%d", year)
	yearLocator := page.Locator(fmt.Sprintf("xpath=//a[text()='%s' and not(contains(@class, 'disabled'))]", yearStr))

	count, err := yearLocator.Count()
	if err != nil {
		return fmt.Errorf("查找年份失败: %v", err)
	}
	if count == 0 {
		return fmt.Errorf("未找到可选择的年份: %d", year)
	}

	if err := yearLocator.First().Click(); err != nil {
		return fmt.Errorf("点击年份 %d 失败: %v", year, err)
	}

	log.Printf("✅ 年份设置完成: %d", year)
	time.Sleep(3 * time.Second) // 等待切换到月份面板

	// 继续设置月份和日期
	return setDateTimeFromMonthPanel(page, targetTime)
}

// setDateTimeFromMonthPanel 从月份面板开始设置日期时间 - 简化版
func setDateTimeFromMonthPanel(page playwright.Page, targetTime time.Time) error {
	targetMonth := int(targetTime.Month())

	log.Printf("🗓️ 设置月份: %d月", targetMonth)

	// 直接使用箭头切换月份
	if err := selectSpecificMonth(page, targetMonth); err != nil {
		return fmt.Errorf("选择月份失败: %v", err)
	}

	log.Printf("✅ 月份设置完成: %d月", targetMonth)
	time.Sleep(2 * time.Second) // 等待日期面板刷新

	// 继续设置日期
	return setDateTimeFromDayPanel(page, targetTime)
}

// selectSpecificMonth 选择具体月份 - 通过点击箭头切换，不重复点击月份标签
func selectSpecificMonth(page playwright.Page, targetMonth int) error {
	log.Printf("📅 选择月份: %d月", targetMonth)

	// 获取当前显示的月份
	currentMonth, err := getCurrentMonth(page)
	if err != nil {
		return fmt.Errorf("获取当前月份失败: %v", err)
	}

	log.Printf("🔍 当前月份: %d, 目标月份: %d", currentMonth, targetMonth)

	if currentMonth == targetMonth {
		log.Printf("✅ 已经是目标月份: %d", targetMonth)
		return nil
	}

	// 计算需要点击的次数
	diff := targetMonth - currentMonth
	if diff < 0 {
		diff += 12 // 处理跨年情况
	}

	log.Printf("🔄 需要点击右箭头 %d 次", diff)

	// 获取右箭头按钮
	rightArrow := page.Locator(".weui-desktop-btn__icon__right").First()
	if count, _ := rightArrow.Count(); count == 0 {
		return fmt.Errorf("未找到右箭头按钮")
	}

	// 点击右箭头切换到目标月份
	for i := 0; i < diff; i++ {
		log.Printf("🖱️ 点击右箭头 (%d/%d)", i+1, diff)
		if err := rightArrow.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(5000),
		}); err != nil {
			return fmt.Errorf("点击右箭头失败: %v", err)
		}
		time.Sleep(1 * time.Second) // 等待月份切换

		// 检查当前月份
		current, err := getCurrentMonth(page)
		if err == nil {
			log.Printf("📅 当前月份: %d", current)
		}
	}

	// 验证最终月份
	finalMonth, err := getCurrentMonth(page)
	if err != nil {
		return fmt.Errorf("验证最终月份失败: %v", err)
	}

	if finalMonth == targetMonth {
		log.Printf("✅ 月份切换成功: %d月", targetMonth)
		return nil
	} else {
		return fmt.Errorf("月份切换失败，当前: %d, 目标: %d", finalMonth, targetMonth)
	}
}

// navigateToMonth 导航到指定月份 - 简化版，只使用箭头切换
func navigateToMonth(page playwright.Page, targetMonth int) error {
	log.Printf("🌍 导航到月份: %d", targetMonth)

	// 直接使用箭头切换月份，不需要切换到月份选择面板
	return selectSpecificMonth(page, targetMonth)
}

// setDateTimeFromDayPanel 从日期面板设置日期和时间 - 修正版
func setDateTimeFromDayPanel(page playwright.Page, targetTime time.Time) error {
	targetDay := targetTime.Day()
	targetMonth := int(targetTime.Month())
	targetYear := targetTime.Year()

	log.Printf("🗓️ 设置日期: %d年%d月%d日", targetYear, targetMonth, targetDay)

	// 首先验证当前显示的月份和年份是否正确
	if err := verifyCurrentYearAndMonth(page, targetTime); err != nil {
		log.Printf("⚠️ 年月验证失败: %v", err)
		// 如果年月不正确，需要重新导航
		if err := navigateToYearAndMonth(page, targetTime); err != nil {
			return fmt.Errorf("修正年月失败: %v", err)
		}
	}

	// 选择目标日期
	if err := selectSpecificDay(page, targetDay); err != nil {
		return fmt.Errorf("选择日期失败: %v", err)
	}

	log.Printf("✅ 日期设置完成: %d日", targetDay)
	time.Sleep(2 * time.Second)

	// 设置时间
	return setTimeSelection(page, targetTime)
}

// verifyCurrentYearAndMonth 验证当前显示的年份和月份
func verifyCurrentYearAndMonth(page playwright.Page, targetTime time.Time) error {
	targetYear := targetTime.Year()
	targetMonth := int(targetTime.Month())

	// 获取当前年份
	currentYear, err := getCurrentYear(page)
	if err != nil {
		return fmt.Errorf("获取当前年份失败: %v", err)
	}

	// 获取当前月份
	currentMonth, err := getCurrentMonth(page)
	if err != nil {
		return fmt.Errorf("获取当前月份失败: %v", err)
	}

	log.Printf("🔍 当前显示: %d年%d月, 目标: %d年%d月",
		currentYear, currentMonth, targetYear, targetMonth)

	if currentYear == targetYear && currentMonth == targetMonth {
		log.Printf("✅ 年月正确: %d年%d月", targetYear, targetMonth)
		return nil
	} else {
		return fmt.Errorf("年月不匹配")
	}
}

// selectSpecificDay 选择具体日期 - 使用数字查找版本
func selectSpecificDay(page playwright.Page, day int) error {
	log.Printf("📅 选择日期: %d", day)

	// 方法1: 直接使用数字查找 - 遍历所有日期元素
	allDates := page.Locator(".weui-desktop-picker__table a")
	count, err := allDates.Count()
	if err != nil {
		return fmt.Errorf("获取日期元素失败: %v", err)
	}

	log.Printf("🔍 总共找到 %d 个日期元素", count)

	// 遍历所有日期元素，查找目标数字
	for i := 0; i < count; i++ {
		dateElement := allDates.Nth(i)

		// 获取日期文本并转换为数字
		text, err := dateElement.TextContent()
		if err != nil {
			continue
		}

		// 清理文本并转换为数字
		text = strings.TrimSpace(text)
		currentDay, err := strconv.Atoi(text)
		if err != nil {
			log.Printf("⚠️ 无法解析日期文本: '%s'", text)
			continue
		}

		// 检查是否是目标日期
		if currentDay == day {
			// 检查是否可点击
			classAttr, _ := dateElement.GetAttribute("class")
			if strings.Contains(classAttr, "disabled") {
				continue
				// log.Printf("❌ 日期 %d 被禁用, class: %s", day, classAttr)
				// return fmt.Errorf("日期 %d 不可选择", day)
			}

			log.Printf("✅ 找到目标日期 %d, 元素位置: %d, class: %s", day, i+1, classAttr)

			// 点击日期
			if err := dateElement.Click(playwright.LocatorClickOptions{
				Timeout: playwright.Float(5000),
			}); err != nil {
				return fmt.Errorf("点击日期 %d 失败: %v", day, err)
			}

			log.Printf("✅ 已选择日期: %d", day)
			time.Sleep(1 * time.Second)

			// 验证选择是否成功
			if err := verifyDaySelection(page, strconv.Itoa(day)); err != nil {
				return fmt.Errorf("日期选择验证失败: %v", err)
			}

			return nil
		}
	}

	log.Printf("❌ 未找到日期: %d", day)
	return fmt.Errorf("日期 %d 未找到", day)
}

// selectSpecificDayOptimized 优化版本 - 只遍历可用日期
func selectSpecificDayOptimized(page playwright.Page, day int) error {
	log.Printf("📅 选择日期 (优化版): %d", day)

	// 只获取可用的日期（没有disabled类）
	availableDates := page.Locator(".weui-desktop-picker__table a:not(.weui-desktop-picker__disabled)")
	count, err := availableDates.Count()
	if err != nil {
		return fmt.Errorf("获取可用日期失败: %v", err)
	}

	log.Printf("🔍 找到 %d 个可用日期", count)

	// 遍历可用日期，查找目标数字
	for i := 0; i < count; i++ {
		dateElement := availableDates.Nth(i)

		// 获取日期文本并转换为数字
		text, err := dateElement.TextContent()
		if err != nil {
			continue
		}

		// 清理文本并转换为数字
		text = strings.TrimSpace(text)
		currentDay, err := strconv.Atoi(text)
		if err != nil {
			log.Printf("⚠️ 无法解析日期文本: '%s'", text)
			continue
		}

		// 检查是否是目标日期
		if currentDay == day {
			log.Printf("✅ 找到可用日期 %d, 元素位置: %d", day, i+1)

			// 点击日期
			if err := dateElement.Click(playwright.LocatorClickOptions{
				Timeout: playwright.Float(5000),
			}); err != nil {
				return fmt.Errorf("点击日期 %d 失败: %v", day, err)
			}

			log.Printf("✅ 已选择日期: %d", day)
			time.Sleep(1 * time.Second)

			// 验证选择是否成功
			if err := verifyDaySelectionByNumber(page, day); err != nil {
				return fmt.Errorf("日期选择验证失败: %v", err)
			}

			return nil
		}
	}

	log.Printf("❌ 在可用日期中未找到: %d", day)
	return fmt.Errorf("日期 %d 不可选择", day)
}

// verifyDaySelectionByNumber 使用数字验证日期选择
func verifyDaySelectionByNumber(page playwright.Page, expectedDay int) error {
	log.Printf("🔍 验证日期选择: %d", expectedDay)

	// 检查选中状态
	selectedDate := page.Locator(".weui-desktop-picker__selected")
	count, err := selectedDate.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到选中的日期")
	}

	selectedText, err := selectedDate.First().TextContent()
	if err != nil {
		return fmt.Errorf("获取选中日期文本失败: %v", err)
	}

	selectedText = strings.TrimSpace(selectedText)
	selectedDay, err := strconv.Atoi(selectedText)
	if err != nil {
		return fmt.Errorf("解析选中日期失败: '%s', 错误: %v", selectedText, err)
	}

	if selectedDay == expectedDay {
		log.Printf("✅ 日期选择验证成功: %d", selectedDay)
		return nil
	}

	return fmt.Errorf("日期选择不匹配: 期望=%d, 实际=%d", expectedDay, selectedDay)
}

// 在setFullDateTime中使用优化版本
func setFullDateTime(page playwright.Page, targetTime time.Time) error {
	targetYear := targetTime.Year()
	targetMonth := int(targetTime.Month())
	targetDay := targetTime.Day()
	targetHour := targetTime.Hour()
	targetMinute := targetTime.Minute()

	log.Printf("🎯 目标时间: %d年%d月%d日 %02d:%02d",
		targetYear, targetMonth, targetDay, targetHour, targetMinute)

	// 1. 设置年份和月份
	if err := navigateToYearAndMonth(page, targetTime); err != nil {
		return fmt.Errorf("设置年月失败: %v", err)
	}

	// 2. 选择日期 - 使用数字查找的优化版本
	log.Printf("🗓️ 选择日期: %d日", targetDay)
	if err := selectSpecificDayOptimized(page, targetDay); err != nil {
		log.Printf("⚠️ 优化版本失败，尝试标准版本: %v", err)
		// 回退到标准版本
		if err := selectSpecificDay(page, targetDay); err != nil {
			return fmt.Errorf("选择日期失败: %v", err)
		}
	}

	// 3. 设置时间
	log.Printf("⏱️ 设置时间: %02d:%02d", targetHour, targetMinute)
	if err := setTimeSelection(page, targetTime); err != nil {
		return fmt.Errorf("设置时间失败: %v", err)
	}

	log.Println("✅ 完整日期时间设置完成")
	return nil
}

// verifyDaySelection 验证日期选择是否成功
func verifyDaySelection(page playwright.Page, expectedDay string) error {
	log.Println("🔍 验证日期选择...")

	// 方法1: 检查选中状态
	selectedDay := page.Locator(".weui-desktop-picker__selected")
	count, err := selectedDay.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到选中的日期")
	}

	actualDay, err := selectedDay.First().TextContent()
	if err != nil {
		return fmt.Errorf("获取选中日期文本失败: %v", err)
	}

	actualDay = strings.TrimSpace(actualDay)
	if actualDay == expectedDay {
		log.Printf("✅ 日期选择验证成功: %s", actualDay)
		return nil
	}

	log.Printf("⚠️ 日期选择不匹配: 期望=%s, 实际=%s", expectedDay, actualDay)

	// 方法2: 检查日期元素的选中状态
	dateElement := page.Locator(fmt.Sprintf("xpath=//a[text()='%s']", expectedDay))
	classAttr, _ := dateElement.First().GetAttribute("class")
	if strings.Contains(classAttr, "weui-desktop-picker__selected") {
		log.Printf("✅ 日期元素有选中样式: %s", classAttr)
		return nil
	}

	return fmt.Errorf("日期选择验证失败")
}

// navigateToYear 导航到指定年份
func navigateToYear(page playwright.Page, targetYear int) error {
	log.Printf("🌍 导航到年份: %d", targetYear)

	// 检查当前是否在年份选择面板
	yearPanel := page.Locator(".weui-desktop-picker__panel_year")
	count, _ := yearPanel.Count()
	if count == 0 {
		// 如果不在年份面板，可能需要切换到年份选择
		log.Println("🔄 切换到年份选择面板")
		// 尝试点击年份标签
		yearLabels := page.Locator(".weui-desktop-picker__panel__label")
		labelCount, _ := yearLabels.Count()
		if labelCount > 0 {
			if err := yearLabels.First().Click(); err != nil {
				log.Printf("⚠️ 点击年份标签失败: %v", err)
			}
			time.Sleep(2 * time.Second)
		}
	}

	// 选择目标年份
	yearStr := fmt.Sprintf("%d", targetYear)
	yearLocator := page.Locator(fmt.Sprintf("xpath=//a[text()='%s' and not(contains(@class, 'disabled'))]", yearStr))

	count, err := yearLocator.Count()
	if err != nil {
		return fmt.Errorf("查找年份失败: %v", err)
	}
	if count == 0 {
		return fmt.Errorf("年份 %d 不可选择或未找到", targetYear)
	}

	if err := yearLocator.First().Click(); err != nil {
		return fmt.Errorf("点击年份 %d 失败: %v", targetYear, err)
	}

	log.Printf("✅ 已选择年份: %d", targetYear)
	time.Sleep(2 * time.Second)
	return nil
}

// setTimeSelection 设置时间选择 - 针对这个特定时间控件
func setTimeSelection(page playwright.Page, targetTime time.Time) error {
	hour := targetTime.Hour()
	minute := targetTime.Minute()

	log.Printf("⏱️ 设置时间: %02d:%02d", hour, minute)

	// 1. 点击时间图标打开时间选择器
	if err := openTimePicker(page); err != nil {
		return fmt.Errorf("打开时间选择器失败: %v", err)
	}

	// 2. 设置小时
	if err := setHourWithScroll(page, hour); err != nil {
		return fmt.Errorf("设置小时失败: %v", err)
	}

	// 3. 设置分钟
	if err := setMinuteWithScroll(page, minute); err != nil {
		return fmt.Errorf("设置分钟失败: %v", err)
	}

	// 4. 确认时间选择
	if err := confirmTimeSelection(page); err != nil {
		return fmt.Errorf("确认时间选择失败: %v", err)
	}

	log.Println("✅ 时间设置完成")
	return nil
}

// openTimePicker 点击时间图标打开时间选择器
func openTimePicker(page playwright.Page) error {
	log.Println("🖱️ 点击时间图标打开时间选择器...")

	// 点击时间图标
	timeIcon := page.Locator(".weui-desktop-icon__time").First()

	if err := timeIcon.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("时间图标不可见: %v", err)
	}

	if err := timeIcon.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("点击时间图标失败: %v", err)
	}

	log.Println("✅ 时间图标点击成功")
	time.Sleep(2 * time.Second)

	// 等待时间选择面板出现
	timePanel := page.Locator(".weui-desktop-picker__dd__time")
	if err := timePanel.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("时间选择面板未出现: %v", err)
	}

	log.Println("✅ 时间选择面板已打开")
	return nil
}

// setHourWithScroll 设置小时（支持滚动选择）
func setHourWithScroll(page playwright.Page, hour int) error {
	hourStr := fmt.Sprintf("%02d", hour)

	log.Printf("⏰ 设置小时: %s", hourStr)

	// 查找小时选项
	hourLocator := page.Locator(fmt.Sprintf(".weui-desktop-picker__time__hour li:has-text('%s')", hourStr))

	// 确保元素存在
	count, err := hourLocator.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到小时选项: %s", hourStr)
	}

	// 检查是否已经是选中状态
	classAttr, _ := hourLocator.First().GetAttribute("class")
	if strings.Contains(classAttr, "weui-desktop-picker__selected") {
		log.Printf("✅ 小时已经是选中状态: %s", hourStr)
		return nil
	}

	// 滚动到小时选项可见
	if err := hourLocator.First().ScrollIntoViewIfNeeded(); err != nil {
		log.Printf("⚠️ 滚动到小时选项失败: %v", err)
	}

	// 点击小时选项
	if err := hourLocator.First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		return fmt.Errorf("点击小时 %s 失败: %v", hourStr, err)
	}

	log.Printf("✅ 已设置小时: %s", hourStr)
	time.Sleep(1 * time.Second)

	// 验证小时是否设置成功
	return verifyHourSelection(page, hourStr)
}

// setMinuteWithScroll 设置分钟（支持滚动选择）
func setMinuteWithScroll(page playwright.Page, minute int) error {
	minuteStr := fmt.Sprintf("%02d", minute)

	log.Printf("⏰ 设置分钟: %s", minuteStr)

	// 查找分钟选项
	minuteLocator := page.Locator(fmt.Sprintf(".weui-desktop-picker__time__minute li:has-text('%s')", minuteStr))

	// 确保元素存在
	count, err := minuteLocator.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到分钟选项: %s", minuteStr)
	}

	// 检查是否已经是选中状态
	classAttr, _ := minuteLocator.First().GetAttribute("class")
	if strings.Contains(classAttr, "weui-desktop-picker__selected") {
		log.Printf("✅ 分钟已经是选中状态: %s", minuteStr)
		return nil
	}

	// 滚动到分钟选项可见
	if err := minuteLocator.First().ScrollIntoViewIfNeeded(); err != nil {
		log.Printf("⚠️ 滚动到分钟选项失败: %v", err)
	}

	// 点击分钟选项
	if err := minuteLocator.First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		return fmt.Errorf("点击分钟 %s 失败: %v", minuteStr, err)
	}

	log.Printf("✅ 已设置分钟: %s", minuteStr)
	time.Sleep(1 * time.Second)

	// 验证分钟是否设置成功
	return verifyMinuteSelection(page, minuteStr)
}

// verifyHourSelection 验证小时设置
func verifyHourSelection(page playwright.Page, expectedHour string) error {
	selectedHour := page.Locator(".weui-desktop-picker__time__hour .weui-desktop-picker__selected")

	count, err := selectedHour.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到选中的小时")
	}

	actualHour, err := selectedHour.First().TextContent()
	if err != nil {
		return fmt.Errorf("获取选中小时文本失败: %v", err)
	}

	actualHour = strings.TrimSpace(actualHour)
	if actualHour == expectedHour {
		log.Printf("✅ 小时设置验证成功: %s", actualHour)
		return nil
	} else {
		return fmt.Errorf("小时设置不匹配: 期望=%s, 实际=%s", expectedHour, actualHour)
	}
}

// verifyMinuteSelection 验证分钟设置
func verifyMinuteSelection(page playwright.Page, expectedMinute string) error {
	selectedMinute := page.Locator(".weui-desktop-picker__time__minute .weui-desktop-picker__selected")

	count, err := selectedMinute.Count()
	if err != nil || count == 0 {
		return fmt.Errorf("未找到选中的分钟")
	}

	actualMinute, err := selectedMinute.First().TextContent()
	if err != nil {
		return fmt.Errorf("获取选中分钟文本失败: %v", err)
	}

	actualMinute = strings.TrimSpace(actualMinute)
	if actualMinute == expectedMinute {
		log.Printf("✅ 分钟设置验证成功: %s", actualMinute)
		return nil
	} else {
		return fmt.Errorf("分钟设置不匹配: 期望=%s, 实际=%s", expectedMinute, actualMinute)
	}
}

// confirmTimeSelection 确认时间选择
func confirmTimeSelection(page playwright.Page) error {
	log.Println("🔒 确认时间选择...")

	// 方法1: 点击时间图标关闭时间选择器
	timeIcon := page.Locator(".weui-desktop-icon__time").First()
	if err := timeIcon.Click(); err != nil {
		log.Printf("⚠️ 点击时间图标关闭失败: %v", err)
	}

	time.Sleep(1 * time.Second)

	// 方法2: 如果时间面板仍然打开，点击外部关闭
	timePanel := page.Locator(".weui-desktop-picker__dd__time:visible")
	if count, _ := timePanel.Count(); count > 0 {
		log.Println("⚠️ 时间面板仍然打开，点击外部关闭")
		if err := page.Locator("body").First().Click(); err != nil {
			log.Printf("⚠️ 点击外部关闭失败: %v", err)
		}
	}

	time.Sleep(2 * time.Second)

	// 验证时间输入框的值
	return verifyTimeInputValue(page)
}

// verifyTimeInputValue 验证时间输入框的值
func verifyTimeInputValue(page playwright.Page) error {
	log.Println("🔍 验证时间输入框的值...")

	timeInput := page.Locator("input[placeholder='请选择时间']").First()
	value, err := timeInput.InputValue()
	if err != nil {
		log.Printf("⚠️ 无法获取时间输入框的值: %v", err)
		return nil // 非致命错误
	}

	if value != "" {
		log.Printf("✅ 时间输入框已设置值: %s", value)
	} else {
		log.Printf("⚠️ 时间输入框值为空")
	}

	return nil
}

// confirmDateTimeSelection 确认日期时间选择
func confirmDateTimeSelection(page playwright.Page) error {
	log.Println("🔒 确认日期时间选择...")

	// 简单点击body关闭面板
	if err := page.Locator("body").First().Click(); err != nil {
		log.Printf("⚠️ 点击body失败: %v", err)
		// 非致命错误，继续流程
	}

	time.Sleep(2 * time.Second)
	log.Println("✅ 日期时间选择流程完成")
	return nil
}

// navigateToYearAndMonth 导航到指定年份和月份
func navigateToYearAndMonth(page playwright.Page, targetTime time.Time) error {
	targetYear := targetTime.Year()
	targetMonth := int(targetTime.Month())

	log.Printf("🌍 导航到: %d年%d月", targetYear, targetMonth)

	// 首先检查当前年份，如果需要则设置年份
	currentYear, err := getCurrentYear(page)
	if err != nil {
		log.Printf("⚠️ 获取当前年份失败: %v", err)
	} else if currentYear != targetYear {
		log.Printf("🔄 需要设置年份: 当前 %d年 → 目标 %d年", currentYear, targetYear)
		if err := navigateToYear(page, targetYear); err != nil {
			return fmt.Errorf("设置年份失败: %v", err)
		}
	} else {
		log.Printf("✅ 年份已经是目标年份: %d", targetYear)
	}

	// 然后设置月份
	if err := navigateToMonth(page, targetMonth); err != nil {
		return fmt.Errorf("设置月份失败: %v", err)
	}

	log.Printf("✅ 年月设置完成: %d年%d月", targetYear, targetMonth)
	return nil
}

// getCurrentMonth 获取当前显示的月份
func getCurrentMonth(page playwright.Page) (int, error) {
	// 方法1: 从面板标签获取月份
	monthLabels := page.Locator(".weui-desktop-picker__panel__label")
	labelCount, _ := monthLabels.Count()

	if labelCount >= 2 {
		currentMonthText, err := monthLabels.Nth(1).TextContent()
		if err != nil {
			return 0, fmt.Errorf("获取月份文本失败: %v", err)
		}

		// 清理文本
		currentMonthText = strings.TrimSpace(currentMonthText)
		log.Printf("🔍 原始月份文本: '%s'", currentMonthText)

		// 移除"月"字
		currentMonthText = strings.TrimSuffix(currentMonthText, "月")
		currentMonthText = strings.TrimSpace(currentMonthText)

		// 解析月份
		currentMonth, err := strconv.Atoi(currentMonthText)
		if err != nil {
			log.Printf("⚠️ 解析月份失败，文本: '%s', 错误: %v", currentMonthText, err)
			// 尝试方法2
			return getCurrentMonthFromTable(page)
		}

		log.Printf("🔍 从标签获取月份: %d", currentMonth)
		return currentMonth, nil
	}

	log.Printf("⚠️ 未找到月份标签，尝试从表格获取")
	// 方法2: 从日期表格推断月份
	return getCurrentMonthFromTable(page)
}

// getCurrentMonthFromTable 从日期表格推断当前月份
func getCurrentMonthFromTable(page playwright.Page) (int, error) {
	// 查找当前选中的日期或有特殊样式的日期
	selectedDay := page.Locator(".weui-desktop-picker__selected, .weui-desktop-picker__current")
	if count, _ := selectedDay.Count(); count > 0 {
		// 如果有选中的日期，可以推断月份
		dayText, err := selectedDay.First().TextContent()
		if err == nil {
			dayText = strings.TrimSpace(dayText)
			log.Printf("🔍 选中日期: %s", dayText)
			// 这里可以根据业务逻辑推断月份，或者返回默认值
		}
	}

	// 方法3: 查找所有日期并推断
	allDays := page.Locator(".weui-desktop-picker__table a:not(.weui-desktop-picker__disabled)")
	if count, _ := allDays.Count(); count > 0 {
		// 获取第一个可用日期的文本
		firstDayText, err := allDays.First().TextContent()
		if err == nil {
			firstDayText = strings.TrimSpace(firstDayText)
			log.Printf("🔍 第一个可用日期: %s", firstDayText)
		}
	}

	// 如果无法确定月份，返回错误或默认值
	return 0, fmt.Errorf("无法确定当前月份")
}

// getCurrentYear 获取当前显示的年份
func getCurrentYear(page playwright.Page) (int, error) {
	// 获取年份标签
	yearLabels := page.Locator(".weui-desktop-picker__panel__label")
	labelCount, _ := yearLabels.Count()

	if labelCount >= 1 {
		currentYearText, err := yearLabels.Nth(0).TextContent()
		if err != nil {
			return 0, fmt.Errorf("获取年份文本失败: %v", err)
		}

		// 清理文本
		currentYearText = strings.TrimSpace(currentYearText)
		log.Printf("🔍 原始年份文本: '%s'", currentYearText)

		// 移除"年"字
		currentYearText = strings.TrimSuffix(currentYearText, "年")
		currentYearText = strings.TrimSpace(currentYearText)

		// 处理年份范围（如"2019年-2030年"）
		if strings.Contains(currentYearText, "-") {
			parts := strings.Split(currentYearText, "-")
			if len(parts) > 0 {
				currentYearText = strings.TrimSpace(parts[0])
			}
		}

		currentYear, err := strconv.Atoi(currentYearText)
		if err != nil {
			return 0, fmt.Errorf("解析年份失败: '%s', 错误: %v", currentYearText, err)
		}

		log.Printf("🔍 当前年份: %d", currentYear)
		return currentYear, nil
	}

	return 0, fmt.Errorf("未找到年份标签")
}
