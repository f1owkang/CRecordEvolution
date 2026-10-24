#!/system/bin/sh
# 脚本编写感谢 酷安@阿巴酱
SKIPUNZIP=0

# 获取基础环境信息
device=`getprop ro.product.device`
version=`getprop ro.build.version.incremental`
android=`getprop ro.build.version.release`
abi=`getprop ro.product.cpu.abi`
[ -n "$abi" ] || abi=unknown

# 模块元信息直接从 module.prop 读取（SKIPUNZIP=0 时已解压到 MODPATH），
# 兼容 Magisk/KSU/APatch/MMRL 各安装器环境变量注入差异。
modname=`grep '^name=' "$MODPATH/module.prop" | cut -d= -f2`
modver=`grep '^version=' "$MODPATH/module.prop" | cut -d= -f2`
modauth=`grep '^author=' "$MODPATH/module.prop" | cut -d= -f2`

# 安装器文案语言：与 batteryd 的探测口径一致（persist.sys.locale →
# ro.product.locale → persist.sys.language），中文优先、非中文回落英文；
# getprop 不可用或属性全空时按中文处理，避免英文误判。
T=zh
for p in persist.sys.locale ro.product.locale persist.sys.language; do
	v=`getprop "$p" 2>/dev/null`
	if [ -n "$v" ]; then
		case "$v" in
		zh*) T=zh ;;
		*) T=en ;;
		esac
		break
	fi
done

# L <中文> <英文>：按安装环境语言取一条文案
L()
{
	if [ "$T" = en ]; then
		printf '%s' "$2"
	else
		printf '%s' "$1"
	fi
}

# 读取一次音量键：UP / DOWN / 空（超时或无按键事件）。
# 每次 getevent 最多阻塞 3s，limit 次仍未读到音量键就返回空。
read_key()
{
	local limit=${1:-10}
	local n=0
	local key
	while [ "$n" -lt "$limit" ]; do
		key="$(timeout 3 getevent -qlc 1 2>/dev/null | awk '{ print $3 }')"
		case "$key" in
		KEY_VOLUMEUP)   echo UP; return 0 ;;
		KEY_VOLUMEDOWN) echo DOWN; return 0 ;;
		esac
		n=$((n + 1))
	done
	echo ""
}

# 获取音量键状态；应用内安装无实体按键事件时，最多等 30s 后按取消处理
get_choose()
{
	case "$(read_key 10)" in
	UP) echo 0 ;;
	*) echo 1 ;;
	esac
}

# 安装时打印的信息
UiPrint()
{
	echo "$@"
	sleep 0.03
}
# 安装语言选择：先问语言再打印其余提示。音量+ 中文 / 音量- English；
# 超时或无按键事件（应用内安装）时跟随系统语言，与 batteryd 的探测口径一致。
# 显式选择会写入 data/lang，模块描述与 once 输出也跟随该语言；之后可在 WebUI 里改。
UiPrint "****************************"
UiPrint "? 选择安装语言 / Select install language"
UiPrint "  音量+ : 中文 / Volume+ : Chinese"
UiPrint "  音量- : English / Volume- : English"
UiPrint "  超时 : 跟随系统 / timeout : follow system"
lang_chosen=0
case "$(read_key 4)" in
UP)   T=zh; lang_chosen=1 ;;
DOWN) T=en; lang_chosen=1 ;;
esac
if [ "$lang_chosen" = 1 ]; then
	# 吃掉这次按键的抬起事件，避免下一步安装确认把它当成新的「音量+」
	timeout 1 getevent -qlc 1 >/dev/null 2>&1
fi
UiPrint "$(L "- 语言: 中文" "- Language: English")"
UiPrint "****************************"
UiPrint "$(L "- 模块: $modname" "- Module: $modname")"
UiPrint "$(L "- 版本: $modver" "- Version: $modver")"
UiPrint "$(L "- 作者: $modauth" "- Author: $modauth")"
UiPrint "****************************"
UiPrint "$(L "- 设备代号: $device" "- Device: $device")"
UiPrint "$(L "- 安卓版本: Android $android" "- Android: $android")"
UiPrint "$(L "- 系统版本: $version" "- Build: $version")"
UiPrint "$(L "- 架构: $abi" "- ABI: $abi")"
UiPrint "****************************"
UiPrint "$(L "* 功能：管理器描述实时显示电池健康度与容量" "* What it does: real-time battery health and capacity in the module description")"
UiPrint "$(L "* WebUI：估算曲线、充放电记录与特征走势（KernelSU/APatch/MMRL）" "* WebUI: estimate curves, charge/discharge records and feature trends (KernelSU/APatch/MMRL)")"
UiPrint "$(L "* 支持 Magisk v20.4+ / KernelSU / APatch / MMRL，仅 arm64" "* Supports Magisk v20.4+ / KernelSU / APatch / MMRL, arm64 only")"
UiPrint "$(L "* 升级会保留 battery.db 学习记录，无需手动备份" "* Upgrading keeps the battery.db learning history; no manual backup needed")"
UiPrint "****************************"
UiPrint "$(L "* 电池健康数据来自系统读数" "* Battery health data comes from system readings")"
UiPrint "$(L "* 厂商或系统不同，估算准确度不同" "* Estimation accuracy varies by vendor and ROM")"
UiPrint "$(L "* 音量+ 安装 | 音量- 取消 | 超时自动取消" "* Volume Up: install | Volume Down: cancel | timeout cancels")"
UiPrint "$(L "? 确定安装此模块吗？" "? Install this module?")"

if [ "$(get_choose)" = "0" ]; then
	UiPrint "$(L "- 已选择安装 $modname" "- Installing $modname")"
	UiPrint "$(L "* 刷入完成后请重启设备，模块描述将显示实时数据" "* Reboot after flashing; the module description will show live data")"
	UiPrint " "
	unzip -o "$ZIPFILE" '/*' -d "$MODPATH" >&2
	# 保留旧学习数据：Magisk/KSU 升级走 staged update，重启时旧模块目录（含
	# data/battery.db）被整体删除替换；此处把旧 data 复制进新 MODPATH，避免
	# 学习记录随旧目录丢失。MODPATH 目录名即模块 id，不依赖安装器注入变量。
	oldmod="/data/adb/modules/`basename "$MODPATH"`"
	if [ -n "$MODPATH" ] && [ "$oldmod" != "$MODPATH" ] && [ -d "$oldmod/data" ]; then
		mkdir -p "$MODPATH/data"
		cp -a "$oldmod"/data/. "$MODPATH"/data/ 2>/dev/null
	fi
	# 显式选了语言才落偏好文件（超时=跟随系统，不写，系统语言变化仍会跟随）；
	# 文件在旧数据复制之后写，保证本次选择覆盖升级带过来的旧偏好。
	if [ "$lang_chosen" = 1 ]; then
		mkdir -p "$MODPATH/data"
		printf '%s\n' "$T" > "$MODPATH/data/lang"
		UiPrint "$(L "* 语言设置已保存，之后可在 WebUI 中修改" "* Language preference saved; you can change it in the WebUI")"
	fi
	set_perm "$MODPATH/bin/batteryd" 0 0 0755
	UiPrint "$(L "* 模块已写入，重启后生效" "* Module installed; reboot to activate")"
else
	abort "$(L "* 已取消安装" "* Installation cancelled")"
fi
