功能说明：自动上传视频小工具

处理流程：执行命令 -> 打开微信视频号URL -> 等待用户扫码登录 -> 自动上传视频
注：扫码登录完后，默认浏览器关闭（操作用户与视频号拥有者权限隔离）

使用说明：
1. 将wechan-channel-uploader.zip解压到某个目录 

2. 进入到此目录，将会看到三个文件和目录 
  video_20251023_demo目录：数据样例，包括：
	channel-video-uploader.xlsx - 需要上传视频的设置信息，如视频描述，定时/不定时发表，短标题，视频位置等
              *.mp4文件 - 需要上传的视频样例，视频位置中指定
              *** 后续如有新上传的视频可复制channel-video-uploader.xlsx并修改其内容，在上传时指定此文件即可
  channel_video_uploader.exe：主程序，必须在DOS下运行
  ms-playwright.zip：运行依赖环境

3. 执行命令：
    在当前目录下执行命令：例：channel_video_uploader.exe -file="video_20251023_demo\channel-video-uploader.xlsx"
    命令行解释：
         channel_video_uploader.exe - 上传视频程序
        -file="video_20251023_demo\channel-video-uploader.xlsx" - 指定上传视频配置信息，其中video_20251023_demo\为目录，channel-video-uploader.xlsx中保存需要上传的文件信息
        -concurrent=false - 指定串行处理上传视频
                    true - 指定并行处理上传视频，大于50个视频，分5个任务；当大于100视频, 分10个任务
        -headless=true - 浏览器无头模式运行，即打开视频号扫码完成后会关闭浏览器，后台运行
        例：E:\tools\wechat-channel-uploader>channel_video_uploader.exe -file="video_20251023_demo\channel-video-uploader.xlsx"，	

5. 执行结果会在log的目录中以.log文件按日期和时间.log文件保存

6. 注意：扫码上传期间，不要再另开浏览器登录扫码登录，否则会挤掉此程序上传视频！！！
