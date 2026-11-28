#!/system/bin/sh
# 脚本编写感谢 酷安@阿巴酱
SKIPUNZIP=0

# 安装器输出控制在 8~14 行：标题/说明、设备、结果、一条提示；只有降级或
# 跨通道（stable ↔ ML）时才追加「已安装 → 本次」对比与音量键确认。
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
[ -n "$modname" ] || modname="CRecord Evolution"

# 只发布 arm64；安装器通常注入 ARCH，缺失时跳过检查。
case "${ARCH:-}" in
""|arm64|aarch64) ;;
*) die "仅支持 arm64 设备 / arm64 only" ;;
esac

device=`getprop ro.product.device 2>/dev/null`
android=`getprop ro.build.version.release 2>/dev/null`
[ -n "$android" ] && android="Android $android"
oldmod="/data/adb/modules/$modid"
oldprop="$oldmod/module.prop"
oldlang=`cat "$oldmod/data/lang" 2>/dev/null | tr -d '\r\n'`

# 恢复模式（原位安装）：Magisk/KernelSU 的 installer 在 source customize.sh 之前
# 先 rm -rf 安装目录再解压，此时 $oldmod == $MODPATH、$oldprop 已是本次包，
# 旧版本/通道与旧 data/ 全都读不到。降级/跨通道判定与数据保留在这种模式下
# 不成立，必须明确告知，不能假装已经检查过。
inplace=0
if [ "$oldmod" = "$MODPATH" ]; then
	inplace=1
fi

# 文案语言：升级沿用旧 data/lang，否则跟随系统（顺序与 i18n.go 一致），
# 探测不到时回退英文（与 i18n.go / boot.js 的兜底一致）。
T=en
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
	if [ -z "$v" ]; then
		v=`cat /data/property/persist.sys.locale 2>/dev/null | tr -d '\r\n'`
		if [ -n "$v" ]; then
			case "$v" in zh*) T=zh ;; *) T=en ;; esac
		fi
	fi
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

# 读一次音量键：UP / DOWN / 空。只采纳按下事件（DOWN），忽略抬起/同步事件。
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

# 安装确认按风险分级：普通升级、同版本重刷、首次安装直接进行（管理器自己
# 已有一次安装确认）；只有「降级」或「stable ↔ ML 跨通道」才要求音量键确认，
# 并显示已安装版本/通道与本次 zip 的对比。
# 应用内安装与恢复模式仍用同一套按键，不按 BOOTMODE/管理器类型跳过。
oldver=""
oldcode=""
oldch=""
if [ -f "$oldprop" ]; then
	oldver=`grep '^version=' "$oldprop" | cut -d= -f2- | tr -d '\r'`
	oldcode=`grep '^versionCode=' "$oldprop" | cut -d= -f2- | tr -d '\r'`
	# stable 包带 updateJson，ML 包在 CI 里删掉该行：这是安装态的可靠通道差异。
	if grep -q '^updateJson=' "$oldprop"; then oldch=stable; else oldch=ml; fi
fi
newch=stable
grep -q '^updateJson=' "$MODPATH/module.prop" || newch=ml

ch_name()
{
	case "$1" in
	ml) L "ML实验版" "ML experimental" ;;
	*) printf 'stable' ;;
	esac
}

need_confirm=0
risk_zh=""
risk_en=""
old_num=0
new_num=0
case "$oldcode" in ""|*[!0-9]*) ;; *) old_num=1 ;; esac
case "$modcode" in ""|*[!0-9]*) ;; *) new_num=1 ;; esac
if [ "$old_num" = 1 ] && [ "$new_num" = 1 ] && [ "$modcode" -lt "$oldcode" ]; then
	need_confirm=1
	risk_zh="降级"
	risk_en="a downgrade"
fi
if [ -n "$oldch" ] && [ "$oldch" != "$newch" ]; then
	if [ "$need_confirm" = 1 ]; then
		risk_zh="降级并切换通道"
		risk_en="a downgrade + channel switch"
	else
		risk_zh="切换通道"
		risk_en="a channel switch"
	fi
	need_confirm=1
fi

if [ "$need_confirm" = 1 ]; then
	say "$(L "已安装：v${oldver:-?}(${oldcode:-?}) $(ch_name "$oldch")" "Installed: v${oldver:-?}(${oldcode:-?}) $(ch_name "$oldch")")"
	say "$(L "本次刷入：v$modver($modcode) $(ch_name "$newch")" "This package: v$modver($modcode) $(ch_name "$newch")")"
	if command -v getevent >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
		say "$(L "检测到$risk_zh（30 秒后取消）" "Detected $risk_en (30s → cancel)")"
		say "$(L "    音量+ 继续安装 / 音量- 取消" "    Volume+ continue / Volume- cancel")"
		case `read_key 10` in
		UP) ;;
		*) die "$(L "已取消安装" "Installation cancelled")" ;;
		esac
	else
		say "$(L "未检测到按键工具，仍继续安装" "No key tools detected; continuing")"
	fi
fi

say "$(L "正在写入模块…" "Installing module...")"

if [ "$inplace" = 1 ]; then
	say "$(L "恢复模式安装：无法判断降级/跨通道，旧数据不会保留" "In-place install: cannot detect downgrade/channel; previous data will not be kept")"
fi

# 保留旧学习数据；失败不阻断安装，也不谎报“已保留”。
# 旧 daemon 仍在跑并写 WAL 库：先按锁文件里的 pid 尽力优雅结束（必须确认是
# 本模块 batteryd），避免 cp 到「主库 + 半截 WAL」的撕裂状态；复制先落到
# 临时目录、成功后再整体替换，失败清理，绝不让半份 data 变成正式库。
kept=0
if [ "$inplace" = 0 ] && [ -d "$oldmod/data" ]; then
	oldpid=`cat "$oldmod/data/batteryd.lock" 2>/dev/null | tr -d '\r\n'`
	case "$oldpid" in
	''|*[!0-9]*) ;;
	*)
		if [ -r "/proc/$oldpid/cmdline" ] && grep -q 'batteryd' "/proc/$oldpid/cmdline" 2>/dev/null; then
			kill -TERM "$oldpid" 2>/dev/null
			sleep 1
		fi
		;;
	esac
	tmpdata="$MODPATH/data.new.$$"
	if mkdir -p "$tmpdata" 2>/dev/null && cp -a "$oldmod/data/." "$tmpdata/" 2>/dev/null; then
		# -shm 是进程共享内存文件，按新库重建即可；-wal 必须保留（含已提交数据）。
		rm -f "$tmpdata"/*-shm 2>/dev/null
		if mv "$tmpdata" "$MODPATH/data" 2>/dev/null; then
			kept=1
		else
			rm -rf "$MODPATH/data" 2>/dev/null
			if mv "$tmpdata" "$MODPATH/data" 2>/dev/null; then
				kept=1
			else
				rm -rf "$tmpdata" 2>/dev/null
			fi
		fi
	else
		rm -rf "$tmpdata" 2>/dev/null
	fi
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
