#!/system/bin/sh
DIR=${0%/*}

# Action 提示文案语言：优先模块语言偏好 data/lang（安装时选择 / WebUI 切换写入），
# 没有偏好时回退系统语言探测（与 batteryd 同口径）；探测不到按中文处理。
T=zh
pref=`cat "$DIR/data/lang" 2>/dev/null`
case "$pref" in
zh*) T=zh ;;
en*) T=en ;;
*)
	# 系统语言兜底探测：顺序须与 cmd/batteryd/i18n.go 的 localeProps 保持
	# 一致；customize.sh 的安装器探测也有一份相同的顺序，改这里要同步那边。
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
	;;
esac

if "$DIR/bin/batteryd" once; then
	if [ "$T" = en ]; then
		echo "Battery health data refreshed"
	else
		echo "电池健康数据已刷新"
	fi
else
	if [ "$T" = en ]; then
		echo "Refresh failed; check the events table in the module's data/battery.db"
	else
		echo "刷新失败，请查看模块 data/battery.db 的 events 表"
	fi
fi
