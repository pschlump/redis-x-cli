#!/bin/bash

#
# Cleanup old reports more than 7 days old - can change this with +7's below.
#
# Should set this up to run once a day with cron.
#

if [ "$(hostname)" = "ub1404001" ] ; then

	cd /home/pschlump/src/go-sql/to

else

	cd /home/pschlump/www/app-t04.qr-today.com/www/go-sql/to

fi

find . -mtime +7 -name 'rpt-daily-[0-9][0-9]*.pdf' -type f
# -exec rm -- \{\} \; 
find . -mtime +7 -name 'rpt-daily-[0-9][0-9]*.html' -type f
# -exec rm -- \{\} \; 

