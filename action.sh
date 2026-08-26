#!/system/bin/sh
DIR=${0%/*}
if "$DIR/bin/batteryd" once; then
	echo "电池健康数据已刷新"
else
	echo "刷新失败，请查看模块 data/battery.db 的 events 表"
fi
