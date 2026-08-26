# 脚本编写感谢 酷安@阿巴酱
SKIPUNZIP=0

# 获取基础环境信息
MODversion=`grep_prop version $TMPDIR/module.prop`
MODdescription=`grep_prop description $TMPDIR/module.prop`
device=`getprop ro.product.device`
Version=`getprop ro.build.version.incremental`
Android=`getprop ro.build.version.release`
Sdk=`getprop ro.build.version.sdk`

# 获取音量键状态
get_choose()
{
	local choose
	local branch
	while :; do
		choose="$(getevent -qlc 1 | awk '{ print $3 }')"
		case "$choose" in
		KEY_VOLUMEUP)  branch="0" ;;
		KEY_VOLUMEDOWN)  branch="1" ;;
		*)  continue ;;
		esac
		echo "$branch"
		break
	done
}

# 安装模块时打印的信息
UiPrint() 
{
	echo "$@"
	sleep 0.03
}
UiPrint "****************************"
UiPrint "- 模块: $MODNAME"
UiPrint "- 作者: $MODAUTH"
UiPrint "****************************"
UiPrint "- 设备代号: $device"
UiPrint "- 安卓版本: Android $Android"
UiPrint "- MIUI版本: $Version"
UiPrint "****************************"
UiPrint "* 电池健康数据来自系统数据"
UiPrint "* 厂商或系统不同，估算准确度不同！"
UiPrint "? 确定安装此模块吗？(请选择)" 
UiPrint "- 按音量键+ : 安装"
UiPrint "- 按音量键- : 取消"
 
if [[ $(get_choose) == 0 ]]; then
	UiPrint "- 已选择安装 $MODNAME"
	UiPrint " "
	unzip -o "$ZIPFILE" '/*' -d $MODPATH >&2
	set_perm "$MODPATH/bin/batteryd" 0 0 0755
else
    abort "* 已经选择退出安装"
fi
