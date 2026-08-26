#!/system/bin/sh
DIR=${0%/*}
if sh "$DIR/service.sh" once; then
	echo "电池健康数据已刷新"
else
	echo "刷新失败，请查看 /sdcard/Documents/电池健康度.log"
fi
