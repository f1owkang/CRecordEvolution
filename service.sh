#!/system/bin/sh
# 等 Android 启动完成后把控制权交给 batteryd
MODDIR=${0%/*}
while [ "$(getprop sys.boot_completed)" != "1" ]; do
	sleep 2
done
exec "$MODDIR/bin/batteryd" daemon
