// design 子命令：设置/清除设计容量手动覆盖（<moddir>/data/design_mah）。
//
// 只在自动裁决不可靠时使用（issue #14：sysfs 报标签容量、设备树缺失/不可信的
// 机型）。写入走临时文件 + rename，daemon 正在读也不会看到半截内容；命令只接受
// 数字 mAh（100~30000），不做任何字符串透传，WebUI 用固定模板拼接数字调用。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parseDesignMah 校验手动覆盖值：纯十进制整数且在合理范围内。
func parseDesignMah(text string) (int64, error) {
	s := strings.TrimSpace(text)
	mah, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("设计容量 %q 不是整数", text)
	}
	if mah < designOverrideMinMah || mah > designOverrideMaxMah {
		return 0, fmt.Errorf("设计容量 %d 超出合理范围 [%d, %d] mAh",
			mah, designOverrideMinMah, designOverrideMaxMah)
	}
	return mah, nil
}

// writeDesignOverride 原子写覆盖文件（临时文件 + rename），内容为「数字 + LF」。
func writeDesignOverride(dataDir string, mah int64) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("创建设计容量目录失败：%w", err)
	}
	path := filepath.Join(dataDir, designOverrideFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(mah, 10)+"\n"), 0o644); err != nil {
		return fmt.Errorf("写设计容量临时文件失败：%w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换设计容量文件失败：%w", err)
	}
	return nil
}

// clearDesignOverride 删除覆盖文件；文件本来不存在视为成功。
func clearDesignOverride(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, designOverrideFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除设计容量覆盖失败：%w", err)
	}
	return nil
}

// runDesign 实现 `batteryd design set <mAh>` / `batteryd design clear`。
func runDesign(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: batteryd design set <mAh> | clear")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位可执行文件失败：%w", err)
	}
	dataDir := filepath.Join(deriveModdir(exe), "data")
	switch args[0] {
	case "set":
		if len(args) != 2 {
			return fmt.Errorf("用法: batteryd design set <mAh>")
		}
		mah, err := parseDesignMah(args[1])
		if err != nil {
			return err
		}
		if err := writeDesignOverride(dataDir, mah); err != nil {
			return err
		}
		fmt.Printf("已设置设计容量 %d mAh（运行 once 或重启后生效）\n", mah)
		return nil
	case "clear":
		if len(args) != 1 {
			return fmt.Errorf("用法: batteryd design clear")
		}
		if err := clearDesignOverride(dataDir); err != nil {
			return err
		}
		fmt.Println("已恢复自动裁决设计容量")
		return nil
	default:
		return fmt.Errorf("未知 design 子命令 %q（set|clear）", args[0])
	}
}
