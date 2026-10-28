#!/system/bin/sh
# 脚本编写感谢 酷安@阿巴酱
SKIPUNZIP=0

# 安装器输出控制在 8~10 行：标题/说明、设备、两步按键、结果、一条提示。
# 管理器提供 ui_print（recovery 控制台可见）时优先使用，否则退回 echo。
say()
{
	if type ui_print >/dev/null 2>&1; then
		ui_print "$*"
	else
		echo "$*"
	fi
}

die()
{
	if type abort >/dev/null 2>&1; then
		abort "$*"
	else
		echo "$*" >&2
		exit 1
	fi
}

[ -n "${MODPATH:-}" ] || die "安装失败：MODPATH 未设置 / MODPATH not set"

# SKIPUNZIP=0 时管理器已解压；仅在缺 module.prop 的异常环境兜底解压一次。
if [ ! -f "$MODPATH/module.prop" ]; then
	unzip -o "$ZIPFILE" -x 'META-INF/*' -d "$MODPATH" >/dev/null 2>&1
fi
[ -f "$MODPATH/module.prop" ] || die "安装失败：安装包不完整 / Corrupt package"

modid=`grep '^id=' "$MODPATH/module.prop" | cut -d= -f2- | tr -d '\r'`
modname=`grep '^name=' "$MODPATH/module.prop" | cut -d= -f2- | tr -d '\r'`
modver=`grep '^version=' "$MODPATH/module.prop" | cut -d= -f2- | tr -d '\r'`
modcode=`grep '^versionCode=' "$MODPATH/module.prop" | cut -d= -f2- | tr -d '\r'`
[ -n "$modid" ] || modid=`basename "$MODPATH"`
[ -n "$modname" ] || modname=CRecordEvolution

# 只发布 arm64；安装器通常注入 ARCH，缺失时跳过检查。
case "${ARCH:-}" in
""|arm64|aarch64) ;;
*) die "仅支持 arm64 设备 / arm64 only" ;;
esac

device=`getprop ro.product.device 2>/dev/null`
android=`getprop ro.build.version.release 2>/dev/null`
[ -n "$android" ] && android="Android $android"
oldlang=`cat "/data/adb/modules/$modid/data/lang" 2>/dev/null | tr -d '\r\n'`

# 文案语言：升级沿用旧 data/lang，否则跟随系统（顺序与 i18n.go 一致），默认中文。
T=zh
case "$oldlang" in
zh*) T=zh ;;
en*) T=en ;;
*)
	for p in persist.sys.locale ro.product.locale persist.sys.language; do
		v=`getprop "$p" 2>/dev/null`
		if [ -n "$v" ]; then
			case "$v" in zh*) T=zh ;; *) T=en ;; esac
			break
		fi
	done
	;;
esac
L()
{
	if [ "$T" = en ]; then printf '%s' "$2"; else printf '%s' "$1"; fi
}
rule()
{
	say "------------------------------------------------"
}

# 读一次音量键：UP / DOWN / 空。只采纳按下事件（DOWN），忽略抬起/同步事件，
# 否则第一次按键的抬起会被下一步确认当成第二次按键。
read_key()
{
	_rk_limit=${1:-10}
	_rk_n=0
	while [ "$_rk_n" -lt "$_rk_limit" ]; do
		_rk_ev=`timeout 3 getevent -qlc 1 2>/dev/null`
		_rk_key=`printf '%s\n' "$_rk_ev" | awk '{ print $3 }'`
		_rk_state=`printf '%s\n' "$_rk_ev" | awk '{ print $4 }'`
		case "$_rk_key" in
		KEY_VOLUMEUP|KEY_VOLUMEDOWN)
			case "$_rk_state" in
			UP|up|0|00000000) ;; # 抬起：继续等真正的按下
			*)
				case "$_rk_key" in
				KEY_VOLUMEUP) echo UP ;;
				*) echo DOWN ;;
				esac
				return 0
				;;
			esac
			;;
		esac
		_rk_n=$((_rk_n + 1))
	done
	echo ""
}

say "$modname  v$modver($modcode) · arm64"
say "$(L "用充电会话估算电池健康与容量" "Battery health & capacity from charge sessions")"
rule
say "$(L "设备：" "Device: ")${device:-unknown}${android:+ · $android}"

# 两步按键：选语言（12s 超时跟随系统）→ 确认安装（30s 超时取消）。
# 读键工具缺失时按安装处理，避免误取消。
lang_chosen=0
if command -v getevent >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
	say "$(L "[1/2] 音量+ 中文 / 音量- English（12 秒后跟随系统）" "[1/2] Volume+ Chinese / Volume- English (12s → system)")"
	case `read_key 4` in
	UP)   T=zh; lang_chosen=1 ;;
	DOWN) T=en; lang_chosen=1 ;;
	esac
	say "$(L "[2/2] 音量+ 安装 / 音量- 取消（30 秒后取消）" "[2/2] Volume+ install / Volume- cancel (30s → cancel)")"
	case `read_key 10` in
	UP) ;;
	*) die "$(L "已取消安装" "Installation cancelled")" ;;
	esac
else
	say "$(L "未检测到按键工具，自动安装" "No key tools detected; installing")"
fi

say "$(L "正在写入模块…" "Installing module...")"

# 保留旧学习数据；失败不阻断安装，也不谎报“已保留”。
kept=0
oldmod="/data/adb/modules/$modid"
if [ "$oldmod" != "$MODPATH" ] && [ -d "$oldmod/data" ]; then
	mkdir -p "$MODPATH/data"
	cp -a "$oldmod/data/." "$MODPATH/data/" 2>/dev/null && kept=1
fi
# 显式选了语言才写偏好，覆盖升级带过来的旧值；未选则沿用旧偏好/系统。
if [ "$lang_chosen" = 1 ]; then
	mkdir -p "$MODPATH/data"
	printf '%s\n' "$T" > "$MODPATH/data/lang"
fi

# 只给二进制设权限；不要给 webroot 加 chmod/set_perm。
if type set_perm >/dev/null 2>&1; then
	set_perm "$MODPATH/bin/batteryd" 0 0 0755
else
	chmod 0755 "$MODPATH/bin/batteryd" 2>/dev/null
fi

rule
if [ "$kept" = 1 ]; then
	say "$(L "安装完成 · 旧数据已保留 · 重启后生效" "Done · previous data kept · reboot to activate")"
else
	say "$(L "安装完成 · 重启后生效" "Done · reboot to activate")"
fi
say "$(L "提示：重启后可在模块 WebUI 查看数据与诊断" "Tip: open the module WebUI after reboot for data and diagnostics")"
