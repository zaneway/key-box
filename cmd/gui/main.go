package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"

	"key-box/internal/auth"
	"key-box/internal/config"
	"key-box/internal/crypto"
	"key-box/internal/db"
	"key-box/internal/vault"
)

var (
	myApp        fyne.App
	myWindow     fyne.Window
	authService  *auth.Service
	vaultManager *vault.Manager

	// State
	currentUser string
	currentKeyC []byte

	// 标志：登录后是否自动打开恢复对话框
	shouldShowRestoreAfterLogin bool
	autoLockTimer               *time.Timer
)

func main() {
	myApp = app.New()
	myApp.SetIcon(theme.InfoIcon())
	myWindow = myApp.NewWindow("Key-Box - 密码管理器")
	myWindow.Resize(fyne.NewSize(600, 500))

	// 1. Init Config & DB
	checkEnvAndInit()

	// 2. Show Main Menu (Login/Register)
	showMainMenu()

	myWindow.ShowAndRun()
}

func checkEnvAndInit() {
	autoSalt := ""
	salt, err := config.GetSalt()
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取配置失败: %v", err), myWindow)
		return
	}

	database, err := db.InitDB()
	if err != nil {
		dialog.ShowError(fmt.Errorf("数据库初始化失败: %v", err), myWindow)
		return
	}

	if salt == "" {
		userCount, err := database.UserCount()
		if err != nil {
			dialog.ShowError(fmt.Errorf("检查账户数据失败: %v", err), myWindow)
			return
		}

		if userCount > 0 {
			fyne.CurrentApp().Lifecycle().SetOnStarted(func() {
				showMissingSaltForExistingUsersDialog()
			})
		} else {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				dialog.ShowError(fmt.Errorf("随机数生成失败: %v", err), myWindow)
				return
			}
			autoSalt = hex.EncodeToString(b)
			if err := config.SaveSalt(autoSalt); err != nil {
				dialog.ShowError(fmt.Errorf("保存配置失败: %v", err), myWindow)
				return
			}
		}
	}

	authService = auth.NewService(database)
	vaultManager = vault.NewManager(database)

	if autoSalt != "" {
		// Salt 已自动保存到配置文件 ~/.key-box.config
		msg := "已生成加密 Salt 并自动保存到配置文件。\n\n配置文件路径: ~/.key-box.config\n\n首次使用完成。"
		content := container.NewVBox(
			widget.NewLabel(msg),
		)

		fyne.CurrentApp().Lifecycle().SetOnStarted(func() {
			dialog.ShowCustom("初始化完成", "我知道了", content, myWindow)
		})
	}
}

func showMissingSaltForExistingUsersDialog() {
	content := container.NewVBox(
		widget.NewLabel("检测到本机已有 Key-Box 账户数据，但未找到 SEC_APP_SALT。"),
		widget.NewLabel("为避免覆盖原密钥，程序没有自动生成新的 Salt。"),
		widget.NewSeparator(),
		widget.NewLabel("请配置注册或备份时使用的原 Salt，否则原账号无法解密系统密钥。"),
	)

	dialog.ShowCustomConfirm("需要配置原 Salt", "配置 Salt", "稍后处理", content, func(ok bool) {
		if ok {
			showSaltConfigDialog(nil)
		}
	}, myWindow)
}

func showSaltConfigDialog(onSaved func()) {
	currentSalt, _ := config.GetSalt()
	configPath, err := config.ConfigPath()
	if err != nil {
		configPath = "~/.key-box.config"
	}

	entrySalt := widget.NewEntry()
	entrySalt.SetPlaceHolder("粘贴注册或备份时使用的 SEC_APP_SALT")
	entrySalt.SetText(currentSalt)

	content := container.NewVBox(
		widget.NewLabel("RootKey 由 SEC_APP_SALT 派生，用于解密系统密钥 Key B。"),
		widget.NewLabel("如果是恢复原账号，必须填写注册或备份时的原 Salt。"),
		widget.NewSeparator(),
		widget.NewLabel("配置文件: "+configPath),
		entrySalt,
	)

	dialog.ShowCustomConfirm("配置 SEC_APP_SALT", "保存", "取消", content, func(ok bool) {
		if !ok {
			return
		}

		salt := strings.TrimSpace(entrySalt.Text)
		if salt == "" {
			dialog.ShowError(fmt.Errorf("SEC_APP_SALT 不能为空"), myWindow)
			return
		}
		saveSaltWithOverwriteConfirm(salt, configPath, onSaved)
	}, myWindow)
}

func saveSaltWithOverwriteConfirm(newSalt, configPath string, onSaved func()) {
	oldSalt, exists, err := config.SavedSalt()
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取本地 Salt 失败: %v", err), myWindow)
		return
	}

	if exists && oldSalt != "" && oldSalt != newSalt {
		oldEntry := widget.NewEntry()
		oldEntry.SetText(oldSalt)
		newEntry := widget.NewEntry()
		newEntry.SetText(newSalt)

		content := container.NewVBox(
			widget.NewLabel("本地已存在 SEC_APP_SALT。"),
			widget.NewLabel("保存新值会覆盖现有配置。请先备份旧值，避免旧账户无法解密。"),
			widget.NewSeparator(),
			widget.NewLabel("当前旧值（可复制备份）:"),
			oldEntry,
			widget.NewLabel("即将写入的新值:"),
			newEntry,
			widget.NewLabel("配置文件: "+configPath),
		)

		dialog.ShowCustomConfirm("覆盖 SEC_APP_SALT", "我已备份，覆盖", "取消", content, func(ok bool) {
			if ok {
				saveSaltAndNotify(newSalt, configPath, onSaved)
			}
		}, myWindow)
		return
	}

	saveSaltAndNotify(newSalt, configPath, onSaved)
}

func saveSaltAndNotify(salt, configPath string, onSaved func()) {
	if err := config.SaveSalt(salt); err != nil {
		dialog.ShowError(fmt.Errorf("保存 Salt 失败: %v", err), myWindow)
		return
	}

	if onSaved != nil {
		onSaved()
		return
	}
	dialog.ShowInformation("配置已保存", "已写入: "+configPath+"\n请使用原账号重新登录。", myWindow)
}

func showSystemKeyMismatchDialog(retry func()) {
	content := container.NewVBox(
		widget.NewLabel("系统密钥解密失败，通常表示当前 SEC_APP_SALT 与注册时不一致。"),
		widget.NewLabel("重新打包或重装不会迁移 Shell 环境变量；双击 macOS App 也不会继承终端里的 export。"),
		widget.NewSeparator(),
		widget.NewLabel("请粘贴原 Salt。保存后会立即重试登录。"),
	)

	dialog.ShowCustomConfirm("RootKey 不匹配", "配置 Salt", "取消", content, func(ok bool) {
		if ok {
			showSaltConfigDialog(retry)
		}
	}, myWindow)
}

func startAutoLock() {
	if autoLockTimer != nil {
		autoLockTimer.Stop()
	}
	autoLockTimer = time.AfterFunc(10*time.Minute, func() {
		if currentUser == "" {
			return
		}
		currentUser = ""
		currentKeyC = nil
		myWindow.Resize(fyne.NewSize(600, 500))
		showMainMenu()
		dialog.ShowInformation("已自动锁定", "应用空闲超过 10 分钟，已返回登录页。", myWindow)
	})
}

func copyPasswordToClipboard(password string) {
	myWindow.Clipboard().SetContent(password)
	dialog.ShowInformation("已复制", "密码已复制到剪贴板，将在 30 秒后自动清理。", myWindow)
	time.AfterFunc(30*time.Second, func() {
		if myWindow.Clipboard().Content() == password {
			myWindow.Clipboard().SetContent("")
		}
	})
}

func generatePasswordInto(entry *widget.Entry) {
	password, err := crypto.GeneratePassword(crypto.PasswordGeneratorOptions{
		Length:      20,
		Lowercase:   true,
		Uppercase:   true,
		Digits:      true,
		Symbols:     true,
		NoAmbiguous: true,
	})
	if err != nil {
		dialog.ShowError(fmt.Errorf("生成密码失败: %v", err), myWindow)
		return
	}
	entry.SetText(password)
}

func showSecurityCenterDialog() {
	salt, _ := config.GetSalt()
	saltStatus := "已配置"
	if strings.TrimSpace(salt) == "" {
		saltStatus = "未配置"
	}

	entryPassword := widget.NewPasswordEntry()
	entryPassword.PlaceHolder = "新登录密码"
	entryConfirm := widget.NewPasswordEntry()
	entryConfirm.PlaceHolder = "确认新登录密码"

	content := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("安全状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("当前用户: "+currentUser),
		widget.NewLabel("SEC_APP_SALT: "+saltStatus),
		widget.NewLabel("自动锁定: 10 分钟"),
		widget.NewLabel("剪贴板保护: 复制密码 30 秒后自动清理"),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("修改登录密码", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entryPassword,
		entryConfirm,
		widget.NewButton("保存新登录密码", func() {
			if entryPassword.Text == "" {
				dialog.ShowError(fmt.Errorf("新登录密码不能为空"), myWindow)
				return
			}
			if entryPassword.Text != entryConfirm.Text {
				dialog.ShowError(fmt.Errorf("两次输入的登录密码不一致"), myWindow)
				return
			}
			if err := authService.SetLoginPassword(currentUser, entryPassword.Text); err != nil {
				dialog.ShowError(fmt.Errorf("修改登录密码失败: %v", err), myWindow)
				return
			}
			dialog.ShowInformation("成功", "登录密码已更新", myWindow)
		}),
		widget.NewSeparator(),
		widget.NewLabel("建议：修改重要密码后及时备份，并妥善保管 ~/.key-box.config。"),
	))
	content.SetMinSize(fyne.NewSize(480, 420))

	d := dialog.NewCustom("安全中心", "关闭", content, myWindow)
	d.Resize(fyne.NewSize(560, 500))
	d.Show()
}

func showMainMenu() {
	// 标题区域
	titleLabel := widget.NewLabelWithStyle("🔐 Key-Box", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	//subtitleLabel := widget.NewLabelWithStyle("安全本地密码管理器", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	url, _ := url.Parse("https://github.com/zaneway/key-box")
	githubLink := widget.NewHyperlink("🌟 访问全球最大的同性交友网站（项目主页）", url)

	titleContainer := container.NewVBox(
		layout.NewSpacer(),
		titleLabel,
		//subtitleLabel,
		githubLink,
		layout.NewSpacer(),
	)

	// Login is the main content
	loginContent := createLoginContent()

	// 主布局：上方标题，下方居中登录表单
	mainContent := container.NewBorder(
		titleContainer, // top
		nil,            // bottom
		nil,            // left
		nil,            // right
		container.NewVBox(
			layout.NewSpacer(),
			loginContent,
			layout.NewSpacer(),
		),
	)

	myWindow.SetContent(mainContent)
}

func createLoginContent() fyne.CanvasObject {
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "👤 用户名"
	entryUser.Resize(fyne.NewSize(250, 40))

	entryPassword := widget.NewPasswordEntry()
	entryPassword.PlaceHolder = "🔑 登录密码（老账号首次迁移可留空）"
	entryPassword.Resize(fyne.NewSize(250, 40))

	entryOTP := widget.NewEntry()
	entryOTP.PlaceHolder = "🔢 6位 OTP 验证码"
	entryOTP.Resize(fyne.NewSize(250, 40))

	// 登录处理函数
	var performLogin func()
	var openVaultAfterLogin func()
	performLogin = func() {
		user := entryUser.Text
		password := entryPassword.Text
		otp := entryOTP.Text

		if user == "" || otp == "" {
			dialog.ShowError(fmt.Errorf("请输入用户名和验证码"), myWindow)
			return
		}

		requiresSetup, _ := authService.RequiresPasswordSetup(user)
		if !requiresSetup && password == "" {
			dialog.ShowError(fmt.Errorf("请输入登录密码"), myWindow)
			return
		}

		var (
			keyC []byte
			err  error
		)
		if requiresSetup {
			keyC, err = authService.Login(user, otp)
		} else {
			keyC, err = authService.LoginWithPassword(user, password, otp)
		}
		if err != nil {
			if errors.Is(err, auth.ErrSystemKeyMismatch) {
				showSystemKeyMismatchDialog(performLogin)
				return
			}
			dialog.ShowError(fmt.Errorf("登录失败: %v", err), myWindow)
			return
		}

		// Login Success
		currentUser = user
		currentKeyC = keyC

		if requiresSetup {
			showForceSetPasswordDialog(func() {
				openVaultAfterLogin()
			})
			return
		}

		openVaultAfterLogin()
	}

	openVaultAfterLogin = func() {
		// 检查是否需要自动打开恢复对话框
		if shouldShowRestoreAfterLogin {
			shouldShowRestoreAfterLogin = false
			// 先显示密码管理界面
			showVaultScreen()
			// 稍后打开恢复对话框
			time.AfterFunc(500*time.Millisecond, func() {
				showRestoreDialog()
			})
		} else {
			showVaultScreen()
		}
	}

	// 验证码输入框回车事件 - 触发登录
	entryOTP.OnSubmitted = func(string) {
		performLogin()
	}

	btnLogin := widget.NewButton("登录", performLogin)
	btnLogin.Importance = widget.HighImportance

	btnRegister := widget.NewButtonWithIcon("注册", theme.InfoIcon(), func() {
		showRegisterDialog()
	})

	btnRestore := widget.NewButtonWithIcon("恢复", theme.DownloadIcon(), func() {
		showRestoreDialogBeforeLogin()
	})

	btnForgot := widget.NewButtonWithIcon("重置", theme.MailSendIcon(), func() {
		showResetDialog()
	})

	btnSalt := widget.NewButton("Salt", func() {
		showSaltConfigDialog(nil)
	})

	// 使用 Grid 让输入框更宽
	form := container.NewVBox(
		widget.NewLabelWithStyle("账户登录", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		entryUser,
		entryPassword,
		entryOTP,
		btnLogin,
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), btnRegister, btnRestore, btnForgot, btnSalt, layout.NewSpacer()),
	)

	// 设置最小宽度，让表单更宽更居中
	formContainer := container.NewPadded(form)
	formContainer.Resize(fyne.NewSize(350, 300))

	return formContainer
}

func showForceSetPasswordDialog(onDone func()) {
	entryPassword := widget.NewPasswordEntry()
	entryPassword.PlaceHolder = "新登录密码"
	entryConfirm := widget.NewPasswordEntry()
	entryConfirm.PlaceHolder = "确认登录密码"

	var d dialog.Dialog
	btnSave := widget.NewButton("设置登录密码", func() {
		if entryPassword.Text == "" {
			dialog.ShowError(fmt.Errorf("登录密码不能为空"), myWindow)
			return
		}
		if entryPassword.Text != entryConfirm.Text {
			dialog.ShowError(fmt.Errorf("两次输入的登录密码不一致"), myWindow)
			return
		}
		if err := authService.SetLoginPassword(currentUser, entryPassword.Text); err != nil {
			dialog.ShowError(fmt.Errorf("设置登录密码失败: %v", err), myWindow)
			return
		}
		d.Hide()
		onDone()
	})
	btnSave.Importance = widget.HighImportance

	content := container.NewVBox(
		widget.NewLabel("检测到该账号尚未设置登录密码。"),
		widget.NewLabel("为升级到密码 + OTP 登录，请先设置登录密码。"),
		widget.NewSeparator(),
		entryPassword,
		entryConfirm,
		btnSave,
	)

	d = dialog.NewCustom("设置登录密码", "", content, myWindow)
	d.Resize(fyne.NewSize(420, 260))
	d.Show()
}

func showRegisterDialog() {
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名"

	entryPassword := widget.NewPasswordEntry()
	entryPassword.PlaceHolder = "登录密码"
	entryConfirmPassword := widget.NewPasswordEntry()
	entryConfirmPassword.PlaceHolder = "确认登录密码"

	entryQ1 := widget.NewEntry()
	entryQ1.PlaceHolder = "密保问题 1"
	entryA1 := widget.NewEntry()
	entryA1.PlaceHolder = "答案 1"

	entryQ2 := widget.NewEntry()
	entryQ2.PlaceHolder = "密保问题 2"
	entryA2 := widget.NewEntry()
	entryA2.PlaceHolder = "答案 2"

	entryQ3 := widget.NewEntry()
	entryQ3.PlaceHolder = "密保问题 3"
	entryA3 := widget.NewEntry()
	entryA3.PlaceHolder = "答案 3"

	// Create a dialog window manually or use ShowCustom
	// Since we need to handle "Register" click inside, ShowCustom is good.
	// But standard ShowCustom doesn't have a specific "Register" button unless we make it part of content
	// or use ShowCustomConfirm with "Register" as label.

	// Let's use a window or a custom container in dialog.

	var d dialog.Dialog

	btnReg := widget.NewButton("提交注册", func() {
		if entryUser.Text == "" {
			dialog.ShowError(fmt.Errorf("用户名不能为空"), myWindow)
			return
		}
		if entryPassword.Text == "" {
			dialog.ShowError(fmt.Errorf("登录密码不能为空"), myWindow)
			return
		}
		if entryPassword.Text != entryConfirmPassword.Text {
			dialog.ShowError(fmt.Errorf("两次输入的登录密码不一致"), myWindow)
			return
		}

		res, err := authService.RegisterWithPassword(
			entryUser.Text,
			entryPassword.Text,
			entryQ1.Text, entryQ2.Text, entryQ3.Text,
			entryA1.Text, entryA2.Text, entryA3.Text,
		)
		if err != nil {
			dialog.ShowError(fmt.Errorf("注册失败: %v", err), myWindow)
			return
		}

		// Close register dialog
		d.Hide()

		// Success Dialog
		keyBEntry := widget.NewEntryWithData(bindingString(res.SecretKeyBBase32))

		btnCopy := widget.NewButton("复制到剪贴板", func() {
			myWindow.Clipboard().SetContent(res.SecretKeyBBase32)
			dialog.ShowInformation("已复制", "Key B 已复制到剪贴板", myWindow)
		})
		btnCopy.Importance = widget.HighImportance

		instructionText := widget.NewMultiLineEntry()
		instructionText.SetText(
			"如何使用 Key B 登录：\n" +
				"\n" +
				"1. 使用 TOTP 应用扫描或手动输入上方的 Key B\n" +
				"   推荐应用：Google Authenticator、Microsoft Authenticator\n" +
				"   1Password、Authy 等\n" +
				"\n" +
				"2. TOTP 应用会生成 6 位验证码（每 30 秒刷新）\n" +
				"\n" +
				"3. 登录时输入用户名和当前 6 位验证码即可\n" +
				"\n" +
				"⚠️ 重要：请务必保存 Key B！\n" +
				"   丢失后无法找回，只能通过密保问题重置",
		)
		instructionText.Wrapping = fyne.TextWrapWord

		dSuccess := dialog.NewCustom("注册成功", "关闭",
			container.NewVBox(
				widget.NewLabelWithStyle("🎉 账户创建成功！", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
				widget.NewLabel("您的登录凭证 (Key B):"),
				keyBEntry,
				btnCopy,
				widget.NewSeparator(),
				instructionText,
			), myWindow)
		dSuccess.Resize(fyne.NewSize(500, 450))
		dSuccess.Show()
	})
	btnReg.Importance = widget.HighImportance

	form := container.NewVBox(
		entryUser,
		entryPassword,
		entryConfirmPassword,
		widget.NewSeparator(),
		entryQ1, entryA1,
		widget.NewSeparator(),
		entryQ2, entryA2,
		widget.NewSeparator(),
		entryQ3, entryA3,
		layout.NewSpacer(),
		btnReg,
	)

	// Scrollable content for smaller screens
	content := container.NewVScroll(form)
	content.SetMinSize(fyne.NewSize(400, 400))

	d = dialog.NewCustom("新用户注册", "取消", content, myWindow)
	d.Resize(fyne.NewSize(500, 500))
	d.Show()
}

func showResetDialog() {
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名"

	entryA1 := widget.NewEntry()
	entryA1.PlaceHolder = "答案 1"
	entryA2 := widget.NewEntry()
	entryA2.PlaceHolder = "答案 2"
	entryA3 := widget.NewEntry()
	entryA3.PlaceHolder = "答案 3"

	entryPassword := widget.NewPasswordEntry()
	entryPassword.PlaceHolder = "新登录密码"
	entryConfirmPassword := widget.NewPasswordEntry()
	entryConfirmPassword.PlaceHolder = "确认新登录密码"

	labelQ1 := widget.NewLabel("问题 1: (输入用户名后加载)")
	labelQ2 := widget.NewLabel("问题 2: (输入用户名后加载)")
	labelQ3 := widget.NewLabel("问题 3: (输入用户名后加载)")

	btnLoad := widget.NewButton("加载密保问题", func() {
		if entryUser.Text == "" {
			return
		}
		qs, err := authService.GetSecurityQuestions(entryUser.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("查询失败: %v", err), myWindow)
			return
		}
		labelQ1.SetText("问题 1: " + qs[0])
		labelQ2.SetText("问题 2: " + qs[1])
		labelQ3.SetText("问题 3: " + qs[2])
	})

	var d dialog.Dialog

	btnReset := widget.NewButton("重置密码", func() {
		if entryPassword.Text == "" {
			dialog.ShowError(fmt.Errorf("新登录密码不能为空"), myWindow)
			return
		}
		if entryPassword.Text != entryConfirmPassword.Text {
			dialog.ShowError(fmt.Errorf("两次输入的登录密码不一致"), myWindow)
			return
		}

		res, err := authService.ResetPasswordWithLoginPassword(entryUser.Text, entryA1.Text, entryA2.Text, entryA3.Text, entryPassword.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("重置失败: %v", err), myWindow)
			return
		}

		d.Hide()

		// 使用自定义对话框，包含可选中复制的 Entry
		keyBEntry := widget.NewEntryWithData(bindingString(res.SecretKeyBBase32))

		btnCopy := widget.NewButton("复制到剪贴板", func() {
			myWindow.Clipboard().SetContent(res.SecretKeyBBase32)
			dialog.ShowInformation("已复制", "Key B 已复制到剪贴板", myWindow)
		})
		btnCopy.Importance = widget.HighImportance

		instructionText := widget.NewMultiLineEntry()
		instructionText.SetText(
			"如何使用新的 Key B 登录：\n" +
				"\n" +
				"1. 在 TOTP 应用中删除旧的凭证\n" +
				"\n" +
				"2. 添加新的 Key B 到 TOTP 应用\n" +
				"   推荐应用：Google Authenticator、Microsoft Authenticator\n" +
				"   1Password、Authy 等\n" +
				"\n" +
				"3. TOTP 应用会生成新的 6 位验证码（每 30 秒刷新）\n" +
				"\n" +
				"4. 使用新的验证码登录\n" +
				"\n" +
				"⚠️ 重要：旧的 Key B 已失效！\n" +
				"   请务必保存新的 Key B，丢失后只能再次重置",
		)
		instructionText.Wrapping = fyne.TextWrapWord

		dSuccess := dialog.NewCustom("重置成功", "关闭",
			container.NewVBox(
				widget.NewLabelWithStyle("✅ 密码重置成功！", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
				widget.NewLabel("您的新登录凭证 (Key B):"),
				keyBEntry,
				btnCopy,
				widget.NewSeparator(),
				instructionText,
			), myWindow)
		dSuccess.Resize(fyne.NewSize(500, 450))
		dSuccess.Show()
	})
	btnReset.Importance = widget.HighImportance

	content := container.NewVScroll(container.NewVBox(
		entryUser,
		btnLoad,
		widget.NewSeparator(),
		labelQ1, entryA1,
		labelQ2, entryA2,
		labelQ3, entryA3,
		widget.NewSeparator(),
		entryPassword,
		entryConfirmPassword,
		layout.NewSpacer(),
		btnReset,
	))
	content.SetMinSize(fyne.NewSize(400, 400))

	d = dialog.NewCustom("密码重置 / 找回", "取消", content, myWindow)
	d.Resize(fyne.NewSize(500, 500))
	d.Show()
}

// truncateText 截断文本，如果超出最大长度则添加省略号
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

// createFixedWidthTextCell 创建固定宽度的文本单元格，支持点击查看完整内容
func createFixedWidthTextCell(fullText string, maxChars int, width float32, style fyne.TextStyle) fyne.CanvasObject {
	displayText := truncateText(fullText, maxChars)
	isTruncated := len([]rune(fullText)) > maxChars

	// 如果文本被截断，在末尾添加点击提示
	if isTruncated {
		displayText = truncateText(fullText, maxChars-2) + "…🔍" // 使用省略号和放大镜图标
	}

	label := widget.NewLabelWithStyle("  "+displayText, fyne.TextAlignLeading, style)
	label.Truncation = fyne.TextTruncateEllipsis

	// 创建固定宽度容器 - 使用 Max 容器限制最大宽度
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(width, 1))

	// 创建一个固定大小的容器来确保不会被撑大
	fixedContainer := container.NewMax(spacer, label)

	// 如果文本被截断，添加点击查看功能
	if isTruncated {
		// 创建带悬停效果的可点击区域
		clickableArea := &tappableContainer{
			content:   fixedContainer,
			fullText:  fullText,
			label:     label,
			baseText:  displayText,
			isHovered: false,
		}
		clickableArea.ExtendBaseWidget(clickableArea)
		return clickableArea
	}

	return fixedContainer
}

// tappableContainer 自定义可点击容器，带悬停效果
type tappableContainer struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	fullText  string
	label     *widget.Label
	baseText  string
	isHovered bool
}

func (t *tappableContainer) CreateRenderer() fyne.WidgetRenderer {
	return &tappableRenderer{
		container: t,
		objects:   []fyne.CanvasObject{t.content},
	}
}

func (t *tappableContainer) Tapped(*fyne.PointEvent) {
	dialog.ShowInformation("完整内容", t.fullText, myWindow)
}

func (t *tappableContainer) MouseIn(*fyne.PointEvent) {
	t.isHovered = true
	// 可以在这里添加视觉反馈，比如改变标签颜色
	t.Refresh()
}

func (t *tappableContainer) MouseOut() {
	t.isHovered = false
	t.Refresh()
}

func (t *tappableContainer) MouseMoved(*fyne.PointEvent) {
	// 可选：处理鼠标移动事件
}

func (t *tappableContainer) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

type tappableRenderer struct {
	container *tappableContainer
	objects   []fyne.CanvasObject
}

func (r *tappableRenderer) Layout(size fyne.Size) {
	r.objects[0].Resize(size)
}

func (r *tappableRenderer) MinSize() fyne.Size {
	return r.objects[0].MinSize()
}

func (r *tappableRenderer) Refresh() {
	r.objects[0].Refresh()
}

func (r *tappableRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *tappableRenderer) Destroy() {}

func showVaultScreen() {
	// 调整窗口大小
	myWindow.Resize(fyne.NewSize(800, 600))
	startAutoLock()

	// Vault Toolbar
	btnAdd := widget.NewButtonWithIcon("添加", theme.ContentAddIcon(), func() {
		showAddVaultItemDialog()
	})

	btnBackup := widget.NewButtonWithIcon("备份", theme.DocumentSaveIcon(), func() {
		showBackupDialog()
	})

	btnRestore := widget.NewButtonWithIcon("恢复", theme.DownloadIcon(), func() {
		showRestoreDialog()
	})

	btnCategory := widget.NewButtonWithIcon("分类", theme.ListIcon(), func() {
		showCategoryDialog()
	})

	btnSecurity := widget.NewButtonWithIcon("安全中心", theme.SettingsIcon(), func() {
		showSecurityCenterDialog()
	})

	btnLogout := widget.NewButtonWithIcon("退出", theme.LogoutIcon(), func() {
		if autoLockTimer != nil {
			autoLockTimer.Stop()
		}
		currentUser = ""
		currentKeyC = nil
		myWindow.Resize(fyne.NewSize(600, 500))
		showMainMenu()
	})

	// 搜索框
	searchEntry := widget.NewEntry()
	searchEntry.PlaceHolder = "🔍 搜索标题、网站、URL 或分类..."

	categoryEntry := widget.NewEntry()
	categoryEntry.PlaceHolder = "分类筛选（留空或输入“全部”显示全部）"

	// Content List
	listContainer := container.NewVBox()

	var refreshList func()

	refreshList = func() {
		searchText := searchEntry.Text
		categoryText := categoryEntry.Text
		listContainer.Objects = nil

		// 标题区域
		titleContainer := container.NewBorder(
			nil, nil, nil, nil,
			container.NewVBox(
				widget.NewLabelWithStyle("🔐 密码库", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabel(fmt.Sprintf("当前用户: %s", currentUser)),
			),
		)
		listContainer.Add(titleContainer)
		listContainer.Add(widget.NewSeparator())

		filteredItems, err := vaultManager.ListItemsFiltered(currentUser, currentKeyC, vault.ItemFilter{
			Search:   searchText,
			Category: categoryText,
		})
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取失败: %v", err), myWindow)
			return
		}

		// 空状态
		if len(filteredItems) == 0 {
			emptyText := "暂无密码记录"
			if searchText != "" || categoryText != "" {
				emptyText = "未找到匹配的密码记录"
			}
			listContainer.Add(container.NewCenter(
				container.NewVBox(
					widget.NewLabelWithStyle(emptyText, fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
					widget.NewLabelWithStyle("点击「添加」按钮开始使用", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
				),
			))
		} else {
			// 添加表头 - 使用透明占位符控制列宽
			headerBg := canvas.NewRectangle(color.RGBA{R: 200, G: 200, B: 200, A: 255})

			// 定义列宽
			col1Width := float32(220) // 条目信息列
			col2Width := float32(160) // 账号列
			col3Width := float32(320) // 密码列（加宽）

			// 创建各列标题，使用透明背景矩形控制宽度
			col1Spacer := canvas.NewRectangle(color.Transparent)
			col1Spacer.SetMinSize(fyne.NewSize(col1Width, 1))
			col1Label := widget.NewLabelWithStyle("  标题 / 网站 / 分类", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			col1Box := container.NewStack(col1Spacer, col1Label)

			col2Spacer := canvas.NewRectangle(color.Transparent)
			col2Spacer.SetMinSize(fyne.NewSize(col2Width, 1))
			col2Label := widget.NewLabelWithStyle("  账号", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			col2Box := container.NewStack(col2Spacer, col2Label)

			col3Spacer := canvas.NewRectangle(color.Transparent)
			col3Spacer.SetMinSize(fyne.NewSize(col3Width, 1))
			col3Label := widget.NewLabelWithStyle("  密码", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			col3Box := container.NewStack(col3Spacer, col3Label)

			col4Label := widget.NewLabelWithStyle("  操作", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			headerContent := container.NewStack(
				headerBg,
				container.NewHBox(
					col1Box,
					col2Box,
					col3Box,
					col4Label,
				),
			)
			listContainer.Add(headerContent)
			listContainer.Add(widget.NewSeparator())

			// 密码卡片列表
			for _, item := range filteredItems {
				item := item // Capture for closure

				// 密码显示/隐藏状态
				passwordVisible := false
				passEntry := widget.NewPasswordEntry()
				passEntry.SetText(item.Password)
				passEntry.Disable()

				// 密码显示切换按钮
				var btnTogglePass *widget.Button
				btnTogglePass = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
					if passwordVisible {
						passEntry.Password = true
						btnTogglePass.SetIcon(theme.VisibilityIcon())
					} else {
						passEntry.Password = false
						btnTogglePass.SetIcon(theme.VisibilityOffIcon())
					}
					passwordVisible = !passwordVisible
					passEntry.Refresh()
				})

				// 给密码框添加深色背景和固定宽度
				passBg := canvas.NewRectangle(color.RGBA{R: 35, G: 35, B: 35, A: 255})
				passBg.CornerRadius = 4

				// 使用透明占位符控制密码框最小宽度
				passSpacer := canvas.NewRectangle(color.Transparent)
				passSpacer.SetMinSize(fyne.NewSize(250, 1)) // 密码框宽度设为 250px
				passWithBg := container.NewStack(passSpacer, passBg, passEntry)

				// 密码框和切换按钮组合
				passColumn := container.NewHBox(
					widget.NewLabel("  "), // 与表头对齐
					passWithBg,
					btnTogglePass,
				)

				// 操作按钮组 - 单独放在一个 HBox 中
				actionButtons := container.NewHBox(
					widget.NewButtonWithIcon("复制", theme.ContentCopyIcon(), func() {
						copyPasswordToClipboard(item.Password)
					}),
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
						showEditVaultItemDialog(item, refreshList)
					}),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
						dialog.ShowCustomConfirm("确认删除", "删除", "取消",
							widget.NewLabel(fmt.Sprintf("确定要删除「%s」的密码吗？", item.Site)),
							func(confirm bool) {
								if confirm {
									err := vaultManager.DeleteItem(item.ID)
									if err != nil {
										dialog.ShowError(fmt.Errorf("删除失败: %v", err), myWindow)
									} else {
										dialog.ShowInformation("成功", "密码已删除", myWindow)
										refreshList()
									}
								}
							}, myWindow)
					}),
				)

				// 使用与表头完全相同的列宽和方法
				col1Width := float32(220) // 条目信息列
				col2Width := float32(160) // 账号列
				col3Width := float32(320) // 密码列（加宽）

				metaText := fmt.Sprintf("%s | %s | %s", item.Title, item.Site, item.Category)
				if item.Favorite {
					metaText = "★ " + metaText
				}
				if item.URL != "" {
					metaText = metaText + " | " + item.URL
				}
				if item.UpdatedAt != "" {
					metaText = metaText + " | 更新: " + item.UpdatedAt
				}

				// 第一列：标题/网站/分类 - 固定宽度，超长文本可点击查看
				siteCell := createFixedWidthTextCell(metaText, 22, col1Width, fyne.TextStyle{Bold: true})

				// 第二列：账号 - 固定宽度，超长文本可点击查看
				usernameCell := createFixedWidthTextCell(item.Username, 12, col2Width, fyne.TextStyle{})

				// 第三列：密码 - 固定宽度容器
				col3Spacer := canvas.NewRectangle(color.Transparent)
				col3Spacer.SetMinSize(fyne.NewSize(col3Width, 1))
				passBox := container.NewStack(col3Spacer, passColumn)

				cardContent := container.NewHBox(
					siteCell,
					usernameCell,
					passBox,
					actionButtons,
				)

				// 添加卡片
				listContainer.Add(cardContent)
				listContainer.Add(widget.NewSeparator())
			}
		}
		listContainer.Refresh()
	}

	// 搜索框实时搜索
	searchEntry.OnChanged = func(string) {
		refreshList()
	}
	categoryEntry.OnChanged = func(string) {
		refreshList()
	}

	// Initial Load
	refreshList()

	// 顶部工具栏
	toolbar := container.NewBorder(
		nil, nil, nil, nil,
		container.NewHBox(
			btnAdd,
			btnBackup,
			btnRestore,
			btnCategory,
			btnSecurity,
			layout.NewSpacer(),
			btnLogout,
		),
	)

	// 主布局
	content := container.NewBorder(
		container.NewVBox(
			toolbar,
			widget.NewSeparator(),
			container.NewGridWithColumns(2, searchEntry, categoryEntry),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		container.NewVScroll(listContainer),
	)

	myWindow.SetContent(content)
}

func showCategoryDialog() {
	categories, err := vaultManager.ListCategories(currentUser)
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取分类失败: %v", err), myWindow)
		return
	}

	categoryList := container.NewVBox()
	if len(categories) == 0 {
		categoryList.Add(widget.NewLabel("暂无分类"))
	} else {
		for _, category := range categories {
			categoryList.Add(widget.NewLabel("• " + category))
		}
	}

	entryOld := widget.NewEntry()
	entryOld.PlaceHolder = "原分类名"
	entryNew := widget.NewEntry()
	entryNew.PlaceHolder = "新分类名"
	entryDelete := widget.NewEntry()
	entryDelete.PlaceHolder = "要删除的分类名（条目会移动到未分类）"

	content := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("当前分类", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		categoryList,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("重命名分类", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entryOld,
		entryNew,
		widget.NewButton("重命名", func() {
			if err := vaultManager.RenameCategory(currentUser, entryOld.Text, entryNew.Text); err != nil {
				dialog.ShowError(fmt.Errorf("重命名失败: %v", err), myWindow)
				return
			}
			dialog.ShowInformation("成功", "分类已重命名", myWindow)
			showVaultScreen()
		}),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("删除分类", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entryDelete,
		widget.NewButton("删除并移动到未分类", func() {
			if err := vaultManager.DeleteCategory(currentUser, entryDelete.Text); err != nil {
				dialog.ShowError(fmt.Errorf("删除分类失败: %v", err), myWindow)
				return
			}
			dialog.ShowInformation("成功", "分类已删除，条目已移动到未分类", myWindow)
			showVaultScreen()
		}),
	))
	content.SetMinSize(fyne.NewSize(460, 420))

	d := dialog.NewCustom("分类管理", "关闭", content, myWindow)
	d.Resize(fyne.NewSize(520, 500))
	d.Show()
}

func showAddVaultItemDialog() {
	entryTitle := widget.NewEntry()
	entryTitle.PlaceHolder = "显示标题（如 GitHub 公司账号）"
	entrySite := widget.NewEntry()
	entrySite.PlaceHolder = "网站/应用"
	entryURL := widget.NewEntry()
	entryURL.PlaceHolder = "URL（可选）"
	entryCategory := widget.NewEntry()
	entryCategory.PlaceHolder = "分类（如 工作、个人、财务）"
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名/邮箱"
	entryPass := widget.NewPasswordEntry()
	entryPass.PlaceHolder = "密码"
	checkFavorite := widget.NewCheck("收藏", nil)
	btnGenerate := widget.NewButton("生成强密码", func() {
		generatePasswordInto(entryPass)
	})

	content := container.NewVScroll(container.NewVBox(
		widget.NewLabel("标题"),
		entryTitle,
		widget.NewLabel("网站/应用"),
		entrySite,
		widget.NewLabel("URL"),
		entryURL,
		widget.NewLabel("分类"),
		entryCategory,
		widget.NewLabel("用户名/邮箱"),
		entryUser,
		widget.NewLabel("密码"),
		entryPass,
		btnGenerate,
		checkFavorite,
	))
	content.SetMinSize(fyne.NewSize(520, 420))

	d := dialog.NewCustomConfirm("添加密码", "保存", "取消", content, func(confirm bool) {
		if confirm {
			if entryTitle.Text == "" && entrySite.Text == "" {
				dialog.ShowError(fmt.Errorf("标题或网站不能为空"), myWindow)
				return
			}
			if entryPass.Text == "" {
				dialog.ShowError(fmt.Errorf("密码不能为空"), myWindow)
				return
			}
			err := vaultManager.AddDetailedItem(currentUser, currentKeyC, vault.ItemInput{
				Title:    entryTitle.Text,
				Site:     entrySite.Text,
				URL:      entryURL.Text,
				Category: entryCategory.Text,
				Username: entryUser.Text,
				Password: entryPass.Text,
				Favorite: checkFavorite.Checked,
			})
			if err != nil {
				dialog.ShowError(fmt.Errorf("添加失败: %v", err), myWindow)
			} else {
				dialog.ShowInformation("成功", "密码已添加", myWindow)
				showVaultScreen() // Rebuilds the UI which refreshes list
			}
		}
	}, myWindow)
	d.Resize(fyne.NewSize(600, 520))
	d.Show()
}

func showEditVaultItemDialog(item vault.VaultItem, refreshCallback func()) {
	entryTitle := widget.NewEntry()
	entryTitle.SetText(item.Title)

	entrySite := widget.NewEntry()
	entrySite.SetText(item.Site)

	entryURL := widget.NewEntry()
	entryURL.SetText(item.URL)

	entryCategory := widget.NewEntry()
	entryCategory.SetText(item.Category)

	entryUser := widget.NewEntry()
	entryUser.SetText(item.Username)

	entryPass := widget.NewPasswordEntry()
	entryPass.SetText(item.Password)

	checkFavorite := widget.NewCheck("收藏", nil)
	checkFavorite.SetChecked(item.Favorite)
	btnGenerate := widget.NewButton("生成强密码", func() {
		generatePasswordInto(entryPass)
	})

	content := container.NewVScroll(container.NewVBox(
		widget.NewLabel("标题:"),
		entryTitle,
		widget.NewLabel("网站/应用:"),
		entrySite,
		widget.NewLabel("URL:"),
		entryURL,
		widget.NewLabel("分类:"),
		entryCategory,
		widget.NewLabel("用户名/邮箱:"),
		entryUser,
		widget.NewLabel("密码:"),
		entryPass,
		btnGenerate,
		checkFavorite,
	))
	content.SetMinSize(fyne.NewSize(520, 420))

	d := dialog.NewCustomConfirm("编辑密码", "保存", "取消", content, func(confirm bool) {
		if confirm {
			if entryTitle.Text == "" && entrySite.Text == "" {
				dialog.ShowError(fmt.Errorf("标题或网站不能为空"), myWindow)
				return
			}
			if entryPass.Text == "" {
				dialog.ShowError(fmt.Errorf("密码不能为空"), myWindow)
				return
			}
			err := vaultManager.UpdateDetailedItem(currentKeyC, item.ID, vault.ItemInput{
				Title:    entryTitle.Text,
				Site:     entrySite.Text,
				URL:      entryURL.Text,
				Category: entryCategory.Text,
				Username: entryUser.Text,
				Password: entryPass.Text,
				Favorite: checkFavorite.Checked,
			})
			if err != nil {
				dialog.ShowError(fmt.Errorf("更新失败: %v", err), myWindow)
			} else {
				dialog.ShowInformation("成功", "密码已更新", myWindow)
				refreshCallback()
			}
		}
	}, myWindow)
	d.Resize(fyne.NewSize(600, 520))
	d.Show()
}

// Helper for data binding simple string
func bindingString(s string) binding.String {
	b := binding.NewString()
	b.Set(s)
	return b
}

// showBackupDialog 显示备份对话框，导出密码数据为加密JSON格式
func showBackupDialog() {
	content := container.NewVBox(
		widget.NewLabel("📦 备份说明"),
		widget.NewSeparator(),
		widget.NewLabel("• 将导出您的账户和所有密码数据"),
		widget.NewLabel("• 包含用户信息和加密的密码数据"),
		widget.NewLabel("• 密码数据保持加密状态（使用 Key C）"),
		widget.NewLabel("• 可用于账户迁移和灾难恢复"),
		widget.NewSeparator(),
		widget.NewLabel("✅ 密码已加密，但备份文件包含完整账户信息"),
		widget.NewLabel("⚠️ 请妥善保管备份文件"),
	)

	dialog.ShowCustomConfirm("备份数据", "确认并导出", "取消", content, func(confirm bool) {
		if confirm {
			performBackup()
		}
	}, myWindow)
}

// BackupUserInfo 备份用户信息
type BackupUserInfo struct {
	Username  string `json:"username"`
	Salt      string `json:"salt"`
	Question1 string `json:"question_1"`
	Question2 string `json:"question_2"`
	Question3 string `json:"question_3"`
	EncM      string `json:"enc_m"`
	EncB      string `json:"enc_b"`
	EncC      string `json:"enc_c"`
}

// BackupData 备份数据结构 - 存储加密的密码数据
type BackupData struct {
	Version  string                `json:"version"`
	ExportAt string                `json:"export_at"`
	Username string                `json:"username"`
	User     BackupUserInfo        `json:"user"`
	Items    []BackupItemEncrypted `json:"items"`
}

// BackupItemEncrypted 备份条目 - 密码保持加密状态
type BackupItemEncrypted struct {
	Site    string `json:"site"`     // 网站名称（明文，用于索引）
	EncData string `json:"enc_data"` // 加密的用户名和密码（base64编码）
}

// performBackup 执行实际的备份操作 - 导出加密的JSON数据（包含用户信息）
func performBackup() {
	// 获取用户信息
	user, err := authService.GetUserInfo(currentUser)
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取用户信息失败: %v", err), myWindow)
		return
	}

	// 直接从数据库获取加密数据
	dbItems, err := vaultManager.GetEncryptedItems(currentUser)
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取数据失败: %v", err), myWindow)
		return
	}

	// 构造备份数据
	backup := BackupData{
		Version:  "2.0", // 版本号升级，包含用户信息
		ExportAt: time.Now().Format("2006-01-02 15:04:05"),
		User: BackupUserInfo{
			Username:  user.Username,
			Salt:      hex.EncodeToString(user.Salt),
			Question1: user.Question1,
			Question2: user.Question2,
			Question3: user.Question3,
			EncM:      hex.EncodeToString(user.EncM),
			EncB:      hex.EncodeToString(user.EncB),
			EncC:      hex.EncodeToString(user.EncC),
		},
		Items: make([]BackupItemEncrypted, 0, len(dbItems)),
	}

	for _, item := range dbItems {
		backup.Items = append(backup.Items, BackupItemEncrypted{
			Site:    item.Site,
			EncData: hex.EncodeToString(item.EncData),
		})
	}

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		dialog.ShowError(fmt.Errorf("数据序列化失败: %v", err), myWindow)
		return
	}

	// 使用文件选择对话框让用户选择保存位置
	saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(fmt.Errorf("保存失败: %v", err), myWindow)
			return
		}
		if writer == nil {
			return // 用户取消
		}
		defer writer.Close()

		// 写入JSON数据
		_, err = writer.Write(jsonData)
		if err != nil {
			dialog.ShowError(fmt.Errorf("写入文件失败: %v", err), myWindow)
			return
		}

		// 获取当前SEC_APP_SALT用于提示
		currentSalt, _ := config.GetSalt()
		showBackupSuccessDialog(len(dbItems), currentSalt)
	}, myWindow)

	// 设置默认文件名
	saveDialog.SetFileName(fmt.Sprintf("key-box-backup-%s.json", time.Now().Format("20060102-150405")))
	saveDialog.Show()
}

func showBackupSuccessDialog(itemCount int, currentSalt string) {
	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("已导出账户和 %d 条密码记录。", itemCount)),
		widget.NewLabel("密码数据保持加密，可用于账户迁移和灾难恢复。"),
		widget.NewSeparator(),
	)

	if currentSalt != "" {
		saltEntry := widget.NewEntry()
		saltEntry.SetText(currentSalt)
		commandEntry := widget.NewEntry()
		commandEntry.SetText(fmt.Sprintf("printf '%%s' %s > ~/.key-box.config", shellQuote(currentSalt)))

		content.Add(widget.NewLabel("跨设备恢复前，新设备必须配置相同的 SEC_APP_SALT。"))
		content.Add(widget.NewLabel("当前 SEC_APP_SALT（可选中复制）:"))
		content.Add(saltEntry)
		content.Add(widget.NewLabel("推荐写入命令（可选中复制，避免 echo 写入换行）:"))
		content.Add(commandEntry)
	} else {
		content.Add(widget.NewLabel("SEC_APP_SALT 未配置。跨设备恢复前需要手动配置原 Salt。"))
	}

	dialog.ShowCustom("备份成功", "我知道了", content, myWindow)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// showRestoreDialogBeforeLogin 登录前显示恢复对话框
func showRestoreDialogBeforeLogin() {
	currentSalt, _ := config.GetSalt()

	var saltInfo *fyne.Container
	if currentSalt == "" {
		saltInfo = container.NewVBox(
			widget.NewLabel("⚠️ SEC_APP_SALT 未配置"),
			widget.NewLabel("跨设备恢复前必须配置 SEC_APP_SALT"),
			widget.NewLabel("配置方法："),
			widget.NewLabel("1. 复制原设备的 ~/.key-box.config 到本设备"),
			widget.NewLabel("2. 或设置环境变量: export SEC_APP_SALT=<原值>"),
		)
	} else {
		saltInfo = container.NewVBox(
			widget.NewLabel("✅ SEC_APP_SALT 已配置"),
			widget.NewLabel("（跨设备恢复需与原设备一致）"),
		)
	}

	content := container.NewVBox(
		widget.NewLabel("📥 恢复数据说明"),
		widget.NewSeparator(),
		widget.NewLabel("• 从备份文件恢复账户和密码数据"),
		widget.NewLabel("• 备份文件包含用户信息和加密的密码"),
		widget.NewLabel("• 将创建或覆盖同名账户"),
		widget.NewLabel("• 恢复后可直接使用原 TOTP 登录"),
		widget.NewSeparator(),
		saltInfo,
		widget.NewSeparator(),
		widget.NewLabel("⚠️ 如果账户已存在，数据将被覆盖！"),
	)

	dialog.ShowCustomConfirm("恢复数据", "选择备份文件", "取消", content, func(ok bool) {
		if ok {
			performRestoreWithoutLogin()
		}
	}, myWindow)
}

// performRestoreWithoutLogin 不需要登录的恢复操作
func performRestoreWithoutLogin() {
	openDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(fmt.Errorf("打开文件失败: %v", err), myWindow)
			return
		}
		if reader == nil {
			return // 用户取消
		}
		defer reader.Close()

		// 读取备份文件
		data, err := io.ReadAll(reader)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取备份文件失败: %v", err), myWindow)
			return
		}

		// 解析JSON
		var backup BackupData
		if err := json.Unmarshal(data, &backup); err != nil {
			dialog.ShowError(fmt.Errorf("备份文件格式错误: %v", err), myWindow)
			return
		}

		// 检查版本
		if backup.Version != "2.0" {
			dialog.ShowError(fmt.Errorf("备份文件版本不支持（需要 v2.0）"), myWindow)
			return
		}

		// 恢复用户信息
		user := &db.User{
			Username:  backup.User.Username,
			Salt:      mustDecodeHex(backup.User.Salt),
			Question1: backup.User.Question1,
			Question2: backup.User.Question2,
			Question3: backup.User.Question3,
			EncM:      mustDecodeHex(backup.User.EncM),
			EncB:      mustDecodeHex(backup.User.EncB),
			EncC:      mustDecodeHex(backup.User.EncC),
		}

		// 检查用户是否存在
		existingUser, _ := authService.GetUserInfo(backup.User.Username)
		if existingUser != nil {
			// 用户已存在，询问是否覆盖
			dialog.ShowCustomConfirm("账户已存在",
				"覆盖", "取消",
				widget.NewLabel(fmt.Sprintf("账户 '%s' 已存在。\n是否覆盖现有账户？\n\n⚠️ 覆盖后原账户数据将丢失！", backup.User.Username)),
				func(confirm bool) {
					if confirm {
						// 删除旧账户数据
						authService.DeleteUser(backup.User.Username)
						vaultManager.DeleteAllItems(backup.User.Username)
						// 继续恢复
						continueRestore(user, backup.Items)
					}
				}, myWindow)
		} else {
			// 用户不存在，直接恢复
			continueRestore(user, backup.Items)
		}
	}, myWindow)

	openDialog.Show()
}

// mustDecodeHex 十六进制字符串转字节数组
func mustDecodeHex(s string) []byte {
	data, _ := hex.DecodeString(s)
	return data
}

// continueRestore 继续恢复流程
func continueRestore(user *db.User, items []BackupItemEncrypted) {
	// 创建用户
	if err := authService.RestoreUser(user); err != nil {
		dialog.ShowError(fmt.Errorf("恢复用户失败: %v", err), myWindow)
		return
	}

	// 恢复密码数据
	successCount := 0
	failCount := 0
	for _, item := range items {
		encData := mustDecodeHex(item.EncData)
		err := vaultManager.RestoreEncryptedItem(user.Username, item.Site, encData)
		if err != nil {
			failCount++
		} else {
			successCount++
		}
	}

	// 显示结果
	currentSalt, _ := config.GetSalt()
	var restoreResult string
	if currentSalt == "" {
		if failCount > 0 {
			restoreResult = fmt.Sprintf("账户: %s\n成功导入: %d 条\n失败: %d 条\n\n⚠️ 重要：SEC_APP_SALT 未配置！\n\n配置步骤：\n1. 复制 ~/.key-box.config 到新设备\n2. 或设置环境变量:\n   export SEC_APP_SALT=<备份时显示的值>\n\n配置完成后再尝试登录", user.Username, successCount, failCount)
		} else {
			restoreResult = fmt.Sprintf("账户 '%s' 恢复成功！\n成功导入 %d 条密码记录\n\n⚠️ 重要：SEC_APP_SALT 未配置！\n\n配置步骤：\n1. 复制 ~/.key-box.config 到新设备\n2. 或设置环境变量:\n   export SEC_APP_SALT=<备份时显示的值>\n\n配置完成后再尝试登录", user.Username, successCount)
		}
	} else {
		if failCount > 0 {
			restoreResult = fmt.Sprintf("账户: %s\n成功导入: %d 条\n失败: %d 条\n\n请使用原 TOTP 登录", user.Username, successCount, failCount)
		} else {
			restoreResult = fmt.Sprintf("账户 '%s' 恢复成功！\n成功导入 %d 条密码记录\n\n请使用原 TOTP 登录", user.Username, successCount)
		}
	}
	showRestoreResultDialog(restoreResult, currentSalt)
}

func showRestoreResultDialog(message, currentSalt string) {
	content := container.NewVBox(
		widget.NewLabel(message),
		widget.NewSeparator(),
	)

	if currentSalt == "" {
		content.Add(widget.NewLabel("检测到当前未配置 SEC_APP_SALT。"))
		content.Add(widget.NewLabel("请设置备份时显示的原 Salt，否则恢复账号无法登录。"))
	} else {
		content.Add(widget.NewLabel("当前已配置 SEC_APP_SALT。"))
		content.Add(widget.NewLabel("如果该值不是备份时的原值，请重新设置。"))
	}

	dialog.ShowCustomConfirm("恢复结果", "设置/更新 Salt", "稍后处理", content, func(ok bool) {
		if ok {
			showSaltConfigDialog(nil)
		}
	}, myWindow)
}

// showRestoreDialog 显示恢复对话框（登录后）
func showRestoreDialog() {
	content := container.NewVBox(
		widget.NewLabel("📥 恢复数据说明"),
		widget.NewSeparator(),
		widget.NewLabel("• 从备份文件恢复密码数据"),
		widget.NewLabel("• 备份文件中的密码已加密"),
		widget.NewLabel("• 数据将追加到当前账户中"),
		widget.NewLabel("• 不会覆盖或删除现有数据"),
		widget.NewSeparator(),
		widget.NewLabel("点击「确认」后选择备份文件进行恢复"),
	)

	dialog.ShowCustomConfirm("恢复数据", "确认", "取消", content, func(confirm bool) {
		if confirm {
			performRestore()
		}
	}, myWindow)
}

// performRestore 执行实际的恢复操作 - 从加密JSON导入
func performRestore() {
	openDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(fmt.Errorf("打开文件失败: %v", err), myWindow)
			return
		}
		if reader == nil {
			return // 用户取消
		}
		defer reader.Close()

		// 读取备份文件
		data, err := io.ReadAll(reader)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取备份文件失败: %v", err), myWindow)
			return
		}

		// 解析JSON
		var backup BackupData
		if err := json.Unmarshal(data, &backup); err != nil {
			dialog.ShowError(fmt.Errorf("备份文件格式错误: %v", err), myWindow)
			return
		}

		// 逐条导入加密数据
		successCount := 0
		failCount := 0
		for _, item := range backup.Items {
			// 将十六进制字符串转回字节数组
			encData, err := hex.DecodeString(item.EncData)
			if err != nil {
				failCount++
				continue
			}

			// 直接插入加密数据
			err = vaultManager.RestoreEncryptedItem(currentUser, item.Site, encData)
			if err != nil {
				failCount++
			} else {
				successCount++
			}
		}

		// 显示结果
		if failCount > 0 {
			dialog.ShowInformation("恢复完成",
				fmt.Sprintf("成功导入: %d 条\n失败: %d 条\n\n请刷新列表查看", successCount, failCount),
				myWindow)
		} else {
			dialog.ShowInformation("恢复成功",
				fmt.Sprintf("成功导入 %d 条密码记录！", successCount),
				myWindow)
		}

		// 如果已登录，刷新界面
		if currentUser != "" && currentKeyC != nil {
			showVaultScreen()
		}
	}, myWindow)

	openDialog.Show()
}
