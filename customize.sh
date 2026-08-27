#!/system/bin/sh
# 脚本编写感谢 酷安@阿巴酱
SKIPUNZIP=0

# 获取基础环境信息
device=`getprop ro.product.device`
version=`getprop ro.build.version.incremental`
android=`getprop ro.build.version.release`

# 模块元信息直接从 module.prop 读取（SKIPUNZIP=0 时已解压到 MODPATH），
# 兼容 Magisk/KSU/APatch/MMRL 各安装器环境变量注入差异。
modname=`grep '^name=' "$MODPATH/module.prop" | cut -d= -f2`
modver=`grep '^version=' "$MODPATH/module.prop" | cut -d= -f2`
modauth=`grep '^author=' "$MODPATH/module.prop" | cut -d= -f2`

# 获取音量键状态；应用内安装无实体按键事件时，最多等 30s 后按取消处理
get_choose()
{
	local choose
	local n=0
	while [ "$n" -lt 10 ]; do
		choose="$(timeout 3 getevent -qlc 1 2>/dev/null | awk '{ print $3 }')"
		case "$choose" in
		KEY_VOLUMEUP)   echo 0; return 0 ;;
		KEY_VOLUMEDOWN) echo 1; return 0 ;;
		esac
		n=$((n + 1))
	done
	echo 1
}

# 安装时打印的信息
UiPrint()
{
	echo "$@"
	sleep 0.03
}
UiPrint "****************************"
UiPrint "- 模块: $modname"
UiPrint "- 版本: $modver"
UiPrint "- 作者: $modauth"
UiPrint "****************************"
UiPrint "- 设备代号: $device"
UiPrint "- 安卓版本: Android $android"
UiPrint "- 系统版本: $version"
UiPrint "****************************"
UiPrint "* 电池健康数据来自系统数据"
UiPrint "* 厂商或系统不同，估算准确度不同"
UiPrint "* 音量+ 安装 | 音量- 取消 | 超时自动取消"
UiPrint "? 确定安装此模块吗？"

if [ "$(get_choose)" = "0" ]; then
	UiPrint "- 已选择安装 $modname"
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
	set_perm "$MODPATH/bin/batteryd" 0 0 0755
else
	abort "* 已取消安装"
fi
