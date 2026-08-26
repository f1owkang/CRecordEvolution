#!/system/bin/sh
# Charging_Record：读取 sysfs 估算电池健康度，更新模块描述并记录日志
# 用法：service.sh [once]
#   无参  开机守护：等启动完成后首刷，之后充电状态变化或每 24 小时兜底刷新
#   once  单次刷新后退出（供 action.sh 调用）
MODDIR=${0%/*}
LOGFILE=/data/media/0/Documents/电池健康度.log
BASE=/sys/class/power_supply/battery

# 定位节点：固定路径优先，find 兜底（保持原兼容面）
find_node() {
	if [ -r "$BASE/$1" ]; then
		echo "$BASE/$1"
	else
		find /sys/devices -iname "$1" -type f 2>/dev/null | head -n 1
	fi
}

# 非零纯十进制整数校验
is_valid() {
	case "$1" in
	'' | 0) return 1 ;;
	*[!0-9]*) return 1 ;;
	*) return 0 ;;
	esac
}

refresh() {
	NODE_CFD=$(find_node charge_full_design)
	NODE_CF=$(find_node charge_full)
	NODE_CC=$(find_node cycle_count)
	NODE_STATUS=$(find_node status)
	cfd=$(cat "$NODE_CFD" 2>/dev/null)
	cf=$(cat "$NODE_CF" 2>/dev/null)
	cc=$(cat "$NODE_CC" 2>/dev/null)
	if ! is_valid "$cfd"; then
		reason="charge_full_design"
	elif ! is_valid "$cf"; then
		reason="charge_full"
	elif ! is_valid "$cc"; then
		reason="cycle_count"
	else
		reason=""
	fi
	if [ -n "$reason" ]; then
		echo "$(date "+%Y-%m-%d %H:%M:%S") - 获取电池数据失败：$reason" >>"$LOGFILE"
		return 1
	fi
	battery="出厂设计容量为：$((cfd / 1000))mAh，当前电池容量为：$((cf / 1000))mAh，电池循环次数为：$cc次，估算剩余容量百分比为：$(printf "%d" $((cf * 100 / cfd)))%"
	sed -i '/^description=/d' "$MODDIR/module.prop"
	echo "description=$battery" >>"$MODDIR/module.prop"
	echo "$(date "+%Y-%m-%d %H:%M:%S") - 电池健康记录脚本已运行" >>"$LOGFILE"
	echo "$battery" >>"$LOGFILE"
	echo " " >>"$LOGFILE"
	return 0
}

if [ "$1" = "once" ]; then
	refresh
	exit $?
fi

while [ "$(getprop sys.boot_completed)" != "1" ]; do
	sleep 2
done

retry=0
until refresh; do
	retry=$((retry + 1))
	[ "$retry" -ge 10 ] && break
	sleep 30
done

last_status=$(cat "$NODE_STATUS" 2>/dev/null)
count=0
while :; do
	sleep 60
	count=$((count + 1))
	cur_status=$(cat "$NODE_STATUS" 2>/dev/null)
	if [ "$cur_status" != "$last_status" ] || [ "$count" -ge 1440 ]; then
		refresh
		last_status=$cur_status
		count=0
	fi
done
