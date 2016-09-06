#!/bin/bash

BUILD_NO=$(git rev-list --count HEAD)
DATE=$(date)

for i in $* ; do

	ed $i <<XXxx
1
/BuildNo:/s/: [0-9][0-9][0-9]*/: 0$BUILD_NO/
1
/Date:/s/:.*/: $DATE/
w
q
XXxx

done


