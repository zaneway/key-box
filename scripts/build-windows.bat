@echo off
REM Key-Box Windows 打包脚本 (在 Windows 上运行)
REM 生成 .exe 安装文件

setlocal enabledelayedexpansion

REM 项目根目录
set "PROJECT_ROOT=%~dp0.."
set "APP_NAME=Key-Box"
set "APP_ID=com.keybox.app"
set "ICON_FILE=%PROJECT_ROOT%\key-box.png"
set "BUILD_DIR=%PROJECT_ROOT%\dist\windows"

echo ======================================
echo   Key-Box Windows 打包脚本
echo ======================================

REM 检查 icon 文件是否存在
if not exist "%ICON_FILE%" (
    echo 错误: 图标文件不存在: %ICON_FILE%
    exit /b 1
)

REM 检查 fyne 命令是否可用
where fyne >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo 正在安装 fyne 命令行工具...
    go install fyne.io/fyne/v2/cmd/fyne@latest
)

REM 清理并创建构建目录
echo 清理构建目录...
if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
mkdir "%BUILD_DIR%"

REM 使用 fyne package 打包 Windows 应用
echo 正在打包 Windows 应用...
cd /d "%PROJECT_ROOT%"

fyne package --target windows --src cmd/gui --name "%APP_NAME%" --icon "%ICON_FILE%" --app-id "%APP_ID%" --tags sqlite_unlock_notify --release

if %ERRORLEVEL% NEQ 0 (
    echo 打包失败！
    exit /b 1
)

REM 移动生成的文件
if exist "%APP_NAME%.exe" (
    move "%APP_NAME%.exe" "%BUILD_DIR%\"
    echo 已生成: %BUILD_DIR%\%APP_NAME%.exe

    REM 创建安装包目录
    set "INSTALL_DIR=%BUILD_DIR%\installer"
    mkdir "%INSTALL_DIR%"

    REM 复制 exe 到安装目录
    copy "%BUILD_DIR%\%APP_NAME%.exe" "%INSTALL_DIR%\"

    REM 创建安装说明文件
    (
        echo =======================================
        echo       Key-Box 安装说明
        echo =======================================
        echo.
        echo 1. 双击运行 Key-Box.exe 即可启动应用
        echo.
        echo 2. 如需创建桌面快捷方式:
        echo    - 右键点击 Key-Box.exe
        echo    - 选择"发送到"^>"桌面快捷方式"
        echo.
        echo 3. 如需添加到开始菜单:
        echo    - 将快捷方式复制到:
        echo      C:\Users\%%USERNAME%%\AppData\Roaming\Microsoft\Windows\Start Menu\Programs
        echo.
        echo =======================================
    ) > "%INSTALL_DIR%\README.txt"

    REM 创建自解压安装脚本
    set "INSTALLER_SCRIPT=%BUILD_DIR%\create_installer.bat"
    (
        echo @echo off
        echo setlocal
        echo set "APP_NAME=%APP_NAME%"
        echo set "INSTALL_DIR=%%PROGRAMFILES%%\%%APP_NAME%%"
        echo echo 正在安装 %%APP_NAME%% 到 %%INSTALL_DIR%%...
        echo if not exist "%%INSTALL_DIR%%" mkdir "%%INSTALL_DIR%%"
        echo copy /Y "%APP_NAME%.exe" "%%INSTALL_DIR%%\" ^>nul
        echo.
        echo echo 是否创建桌面快捷方式？
        echo set /p CREATE_SHORTCUT="输入 Y 创建，其他键跳过: "
        echo if /i "%%CREATE_SHORTCRIPT%%"=="Y" (
        echo     powershell -command "$s=(New-Object -COM WScript.Shell).CreateShortcut('%%USERPROFILE%%\Desktop\%%APP_NAME%%.lnk');$s.TargetPath='%%INSTALL_DIR%%\%%APP_NAME%%.exe';$s.Save()"
        echo     echo 桌面快捷方式已创建
        echo ^)
        echo.
        echo echo 安装完成！
        echo pause
    ) > "%INSTALLER_SCRIPT%"

    echo.
    echo ======================================
    echo   打包完成！
    echo ======================================
    echo EXE 文件: %BUILD_DIR%\%APP_NAME%.exe
    echo 安装目录: %INSTALL_DIR%
    echo.
    echo 安装说明: 运行 %APP_NAME%.exe 即可使用
    echo           或运行 create_installer.bat 安装到 Program Files
    echo.

) else (
    echo 错误: 未找到生成的 .exe 文件
    exit /b 1
)

endlocal
