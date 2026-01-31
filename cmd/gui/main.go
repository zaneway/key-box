package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sec-keys/internal/auth"
	"sec-keys/internal/db"
	"sec-keys/internal/vault"
)

var (
	myApp        fyne.App
	myWindow     fyne.Window
	authService  *auth.Service
	vaultManager *vault.Manager

	// State
	currentUser string
	currentKeyC []byte
)

func main() {
	myApp = app.New()
	myWindow = myApp.NewWindow("本地密码管理器 (Sec-Keys)")
	myWindow.Resize(fyne.NewSize(600, 500))

	// 1. Env Check & Init DB
	checkEnvAndInit()

	// 2. Show Main Menu (Login/Register)
	showMainMenu()

	myWindow.ShowAndRun()
}

func checkEnvAndInit() {
	autoSalt := ""
	if os.Getenv("SEC_APP_SALT") == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			dialog.ShowError(fmt.Errorf("随机数生成失败: %v", err), myWindow)
			return
		}
		autoSalt = hex.EncodeToString(b)
		os.Setenv("SEC_APP_SALT", autoSalt)
	}

	database, err := db.InitDB()
	if err != nil {
		dialog.ShowError(fmt.Errorf("数据库初始化失败: %v", err), myWindow)
		return
	}

	authService = auth.NewService(database)
	vaultManager = vault.NewManager(database)

	if autoSalt != "" {
		// Use a custom dialog with a copyable Entry for the Salt and instructions

		msg := fmt.Sprintf("环境变量 SEC_APP_SALT 未设置。\n已生成临时 Salt (请务必保存！):")
		saltEntry := widget.NewEntry()
		saltEntry.SetText(autoSalt)
		// saltEntry.Disable() // Disable makes it look gray, but usually still copyable?
		// Actually in Fyne, Disable() might prevent interaction.
		// Let's keep it enabled but ReadOnly if possible? Fyne Entry doesn't have ReadOnly prop directly exposed easily in v2.0
		// But in newer Fyne versions, Disable() allows copy? Let's verify.
		// Actually, standard pattern is NewEntry with text.

		instruction := fmt.Sprintf("下次启动需配置环境变量:\nMac/Linux: export SEC_APP_SALT=\"%s\"\nWindows: $env:SEC_APP_SALT=\"%s\"", autoSalt, autoSalt)
		instrEntry := widget.NewMultiLineEntry()
		instrEntry.SetText(instruction)

		content := container.NewVBox(
			widget.NewLabel(msg),
			saltEntry,
			widget.NewLabel("配置命令 (可复制):"),
			instrEntry,
		)

		fyne.CurrentApp().Lifecycle().SetOnStarted(func() {
			dialog.ShowCustom("安全警告", "我知道了", content, myWindow)
		})
	}
}

func showMainMenu() {
	labelTitle := widget.NewLabelWithStyle("欢迎使用 Sec-Keys", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// Login is the main content
	loginContent := createLoginContent()

	myWindow.SetContent(container.NewBorder(labelTitle, nil, nil, nil, loginContent))
}

func createLoginContent() fyne.CanvasObject {
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名"

	entryOTP := widget.NewEntry()
	entryOTP.PlaceHolder = "6位 OTP 验证码"

	btnLogin := widget.NewButton("登录", func() {
		user := entryUser.Text
		otp := entryOTP.Text

		if user == "" || otp == "" {
			dialog.ShowError(fmt.Errorf("请输入用户名和验证码"), myWindow)
			return
		}

		keyC, err := authService.Login(user, otp)
		if err != nil {
			dialog.ShowError(fmt.Errorf("登录失败: %v", err), myWindow)
			return
		}

		// Login Success
		currentUser = user
		currentKeyC = keyC
		showVaultScreen()
	})
	btnLogin.Importance = widget.HighImportance

	btnRegister := widget.NewButton("注册新账号", func() {
		showRegisterDialog()
	})

	btnForgot := widget.NewButton("忘记密码/重置", func() {
		showResetDialog()
	})

	return container.NewVBox(
		layout.NewSpacer(),
		widget.NewLabel("请输入您的账户信息:"),
		entryUser,
		entryOTP,
		btnLogin,
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), btnRegister, btnForgot, layout.NewSpacer()),
		layout.NewSpacer(),
	)
}

func showRegisterDialog() {
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名"

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

		res, err := authService.Register(
			entryUser.Text,
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
		dSuccess := dialog.NewCustom("注册成功", "复制并关闭",
			container.NewVBox(
				widget.NewLabel("请务必保存您的最高权限恢复凭证 (Key B):"),
				widget.NewEntryWithData(bindingString(res.SecretKeyBBase32)),
				widget.NewLabel("建议手动复制或输入到 App 中。"),
			), myWindow)
		dSuccess.Show()
	})
	btnReg.Importance = widget.HighImportance

	form := container.NewVBox(
		entryUser,
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
		res, err := authService.ResetPassword(entryUser.Text, entryA1.Text, entryA2.Text, entryA3.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("重置失败: %v", err), myWindow)
			return
		}

		d.Hide()

		// 使用自定义对话框，包含可选中复制的 Entry
		dSuccess := dialog.NewCustom("重置成功", "复制并关闭",
			container.NewVBox(
				widget.NewLabel("密码重置成功！"),
				widget.NewLabel("请务必保存新的最高权限恢复凭证 (Key B):"),
				widget.NewEntryWithData(bindingString(res.SecretKeyBBase32)),
				widget.NewLabel("建议手动复制或输入到 App 中。"),
			), myWindow)
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
		layout.NewSpacer(),
		btnReset,
	))
	content.SetMinSize(fyne.NewSize(400, 400))

	d = dialog.NewCustom("密码重置 / 找回", "取消", content, myWindow)
	d.Resize(fyne.NewSize(500, 500))
	d.Show()
}

func showVaultScreen() {
	// Vault Toolbar
	btnAdd := widget.NewButtonWithIcon("添加密码", theme.ContentAddIcon(), func() {
		showAddVaultItemDialog()
	})

	btnBackup := widget.NewButtonWithIcon("备份数据", theme.DocumentSaveIcon(), func() {
		showBackupDialog()
	})

	btnRestore := widget.NewButtonWithIcon("恢复数据", theme.UploadIcon(), func() {
		showRestoreDialog()
	})

	btnLogout := widget.NewButtonWithIcon("退出登录", theme.LogoutIcon(), func() {
		currentUser = ""
		currentKeyC = nil
		showMainMenu()
	})

	// Content List
	listContainer := container.NewVBox()

	refreshList := func() {
		listContainer.Objects = nil
		items, err := vaultManager.ListItems(currentUser, currentKeyC)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取失败: %v", err), myWindow)
			return
		}

		// Header
		listContainer.Add(container.NewGridWithColumns(4,
			widget.NewLabelWithStyle("网站", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("账号", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("密码", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("操作", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		))
		listContainer.Add(widget.NewSeparator())

		for _, item := range items {
			item := item // Capture for closure

			// 密码脱敏显示
			passLabel := widget.NewLabel("********")

			// 复制按钮
			btnCopy := widget.NewButtonWithIcon("复制", theme.ContentCopyIcon(), func() {
				myWindow.Clipboard().SetContent(item.Password)

				// Optional: Feedback (change label temporarily)
				passLabel.SetText("已复制!")
				// Revert after 2 seconds (using simple timer here if needed, or just leave it)
				// Since we don't have async timer easy access without blocking or using time.AfterFunc,
				// let's just leave it or use time.AfterFunc if "time" was imported.
				// For now simple feedback is enough.
			})

			listContainer.Add(container.NewGridWithColumns(4,
				widget.NewLabel(item.Site),
				widget.NewLabel(item.Username),
				passLabel,
				btnCopy,
			))
		}
		listContainer.Refresh()
	}

	// Initial Load
	refreshList()

	// Layout
	content := container.NewBorder(
		container.NewHBox(
			widget.NewLabel("当前用户: "+currentUser),
			layout.NewSpacer(),
			btnBackup,
			btnRestore,
			btnAdd,
			btnLogout,
		),
		nil, nil, nil,
		container.NewVScroll(listContainer),
	)

	myWindow.SetContent(content)
}

func showAddVaultItemDialog() {
	entrySite := widget.NewEntry()
	entrySite.PlaceHolder = "网站/应用"
	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "用户名/邮箱"
	entryPass := widget.NewPasswordEntry()
	entryPass.PlaceHolder = "密码"

	dialog.ShowCustomConfirm("添加密码", "保存", "取消", container.NewVBox(
		entrySite, entryUser, entryPass,
	), func(confirm bool) {
		if confirm {
			if entrySite.Text == "" || entryPass.Text == "" {
				return // Simple ignore validation for dialog for now or show error?
			}
			err := vaultManager.AddItem(currentUser, currentKeyC, entrySite.Text, entryUser.Text, entryPass.Text)
			if err != nil {
				dialog.ShowError(fmt.Errorf("添加失败: %v", err), myWindow)
			} else {
				// Refresh main screen?
				// To do this cleanly, showVaultScreen() should be idempotent or refreshable.
				showVaultScreen() // Rebuilds the UI which refreshes list
			}
		}
	}, myWindow)
}

// Helper for data binding simple string
func bindingString(s string) binding.String {
	b := binding.NewString()
	b.Set(s)
	return b
}

// showBackupDialog 显示备份对话框，导出数据库并提示环境变量
func showBackupDialog() {
	// 获取当前环境变量
	currentSalt := os.Getenv("SEC_APP_SALT")
	if currentSalt == "" {
		dialog.ShowError(fmt.Errorf("环境变量 SEC_APP_SALT 未设置，无法备份"), myWindow)
		return
	}

	// 提示信息
	saltEntry := widget.NewEntry()
	saltEntry.SetText(currentSalt)

	instruction := widget.NewMultiLineEntry()
	instruction.SetText(fmt.Sprintf("备份说明:\n1. 数据库文件将导出到您选择的位置\n2. 环境变量 SEC_APP_SALT 是解密数据的关键\n3. 请务必保存以下 Salt 值:\n\nMac/Linux: export SEC_APP_SALT=\"%s\"\nWindows: $env:SEC_APP_SALT=\"%s\"\n\n⚠️ 警告: 没有此 Salt 值，备份数据将无法恢复！", currentSalt, currentSalt))

	content := container.NewVBox(
		widget.NewLabel("⚠️ 重要提示：备份数据需要配合环境变量使用"),
		widget.NewSeparator(),
		widget.NewLabel("当前 SEC_APP_SALT 值 (可复制):"),
		saltEntry,
		widget.NewSeparator(),
		instruction,
	)

	dialog.ShowCustomConfirm("备份数据", "确认并导出", "取消", content, func(confirm bool) {
		if confirm {
			performBackup()
		}
	}, myWindow)
}

// performBackup 执行实际的备份操作
func performBackup() {
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

		// 获取数据库路径
		home, err := os.UserHomeDir()
		if err != nil {
			dialog.ShowError(fmt.Errorf("获取主目录失败: %v", err), myWindow)
			return
		}
		dbPath := filepath.Join(home, ".sec-keys.db")

		// 读取数据库文件
		data, err := os.ReadFile(dbPath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取数据库失败: %v", err), myWindow)
			return
		}

		// 写入到用户选择的位置
		_, err = writer.Write(data)
		if err != nil {
			dialog.ShowError(fmt.Errorf("写入文件失败: %v", err), myWindow)
			return
		}

		dialog.ShowInformation("备份成功", "数据库已成功导出！\n请妥善保管备份文件和 SEC_APP_SALT 值。", myWindow)
	}, myWindow)

	// 设置默认文件名
	saveDialog.SetFileName(fmt.Sprintf("sec-keys-backup-%s.db", time.Now().Format("20060102-150405")))
	saveDialog.Show()
}

// showRestoreDialog 显示恢复对话框
func showRestoreDialog() {
	content := container.NewVBox(
		widget.NewLabel("⚠️ 恢复数据将覆盖当前数据库"),
		widget.NewSeparator(),
		widget.NewLabel("请确保:"),
		widget.NewLabel("1. 已正确设置 SEC_APP_SALT 环境变量"),
		widget.NewLabel("2. 环境变量与备份时的值一致"),
		widget.NewLabel("3. 已备份当前数据（如需保留）"),
		widget.NewSeparator(),
		widget.NewLabel("点击「确认」后选择备份文件进行恢复"),
	)

	dialog.ShowCustomConfirm("恢复数据", "确认", "取消", content, func(confirm bool) {
		if confirm {
			performRestore()
		}
	}, myWindow)
}

// performRestore 执行实际的恢复操作
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

		// 获取数据库路径
		home, err := os.UserHomeDir()
		if err != nil {
			dialog.ShowError(fmt.Errorf("获取主目录失败: %v", err), myWindow)
			return
		}
		dbPath := filepath.Join(home, ".sec-keys.db")

		// 备份当前数据库（防止误操作）
		backupPath := dbPath + ".before-restore"
		if _, err := os.Stat(dbPath); err == nil {
			os.Rename(dbPath, backupPath)
		}

		// 写入恢复的数据
		err = os.WriteFile(dbPath, data, 0600)
		if err != nil {
			dialog.ShowError(fmt.Errorf("写入数据库失败: %v", err), myWindow)
			// 尝试恢复备份
			if _, err := os.Stat(backupPath); err == nil {
				os.Rename(backupPath, dbPath)
			}
			return
		}

		// 删除临时备份
		os.Remove(backupPath)

		dialog.ShowInformation("恢复成功", "数据已成功恢复！\n请重新启动应用以加载新数据。", myWindow)
	}, myWindow)

	openDialog.Show()
}
