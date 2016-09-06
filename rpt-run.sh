#!/bin/bash

RM=/bin/rm
DATE=/bin/date

if [ "$(hostname)" = "ub1404001" ] ; then
	PT=/home/pschlump/src/go-sql
else
	PT=/home/pschlump/www/app-t04.qr-today.com/www/sel
fi

cd "$PT"
mkdir -p log

HH=$( $DATE '+%u' )
H1=$(expr $HH + 1)
if [ "$H1" = "8" ] ; then
	H1="1"
fi

$RM -f log/rpt-run.$H1.log

echo "======================================================" >>log/rpt-run.$HH.log
echo "Run Reports at $( $DATE )" >>log/rpt-run.$HH.log
echo "======================================================" >>log/rpt-run.$HH.log


./run-qry >>log/rpt-run.$HH.log 2>&1

# For production - install from .deb
# ./run-qry -g /usr/local/etc/global-cfg.json -r /usr/local/etc/rpt-cfg.json

