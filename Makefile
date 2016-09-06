#
# Makefile for generating "sel" 
#
# Version: 0.9.0
#

all: 
	go build 

test: t00 t01 t02 t03 t04 t05 t06 t07 t08 t09 t10 t11 t16 t17 t19 t22 t23 t24 t12

test_all:
	( cd go-lib/em ; make test )
	( cd go-lib/gouuid ; make test )
	( cd go-lib/mailbuilder ; make test )
	( cd go-lib/ms ; make test )
	( cd go-lib/picfloat ; make test )
	( cd go-lib/pictime ; make test )
	( cd go-lib/resize ; make test )
	( cd go-lib/strftime ; make test )
	( cd go-lib/tr ; make test )
	( cd go-lib/words ; make test )

setup:
	-rm go-lib
	ln -s $(HOME)/lib/go-lib ./go-lib

# From: https://www.digitalocean.com/company/blog/get-your-development-team-started-with-go/ 
# dpkg -c go-sql_0.9.0-61_amd64.deb  -- To see files in a .deb

VERSION=0.9.0
BUILD=$(shell git rev-list --count HEAD)

go-sql-dpkg:
	mkdir -p deb/go-sql/usr/local/bin deb/go-sql/usr/local/etc/go-sql deb/go-sql/usr/local/doc
	cp ./go-sql  deb/go-sql/usr/local/bin
	cp ./*.json  deb/go-sql/usr/local/etc/go-sql
	cp ./err.doc  deb/go-sql/usr/local/doc
	fpm -s dir -t deb -n go-sql -v $(VERSION)-$(BUILD) -C deb/go-sql .

#run-qry: run-qry.go
#	go build run-qry.go

go-sql: 
	go build 

DIFF=diff
CP=cp

test-setup:
	@./go-sql -i td/set-up.sel

test-tear-down:
	@./go-sql -i td/tear-down.sel


t00: go-sql
	@./go-sql -i td/t00.sel -o to/o00.out
	@$(DIFF) to/o00.out ref/o00.out
	@echo TEST 00: PASS!

ok-t00:
	@$(CP) to/o00.out ref/o00.out

t01: go-sql
	@./go-sql -i td/t01.sel -o to/t01.out
	@$(DIFF) to/t01.out ref/t01.out
	@sed -e /CreationDate/d <to/t01.pdf >to/t01.pdf.mod
	@sed -e /CreationDate/d <ref/t01.pdf >ref/t01.pdf.mod
	@$(DIFF) to/t01.pdf.mod ref/t01.pdf.mod
	@echo TEST 01: PASS!

ok-t01:
	@$(CP) to/o01.out ref/o01.out

# XML output
t02: go-sql
	@./go-sql -i td/t02.sel -o to/o02.out
	@$(DIFF) to/o02.out ref/o02.out
	@echo TEST 02: PASS!

ok-t02:
	@$(CP) to/o02.out ref/o02.out

# xml output
t03: go-sql
	@./go-sql -i td/t03.sel -o to/o03.out
	@$(DIFF) to/o03.out ref/o03.out
	@echo TEST 03: PASS!

ok-t03:
	@$(CP) to/o03.out ref/o03.out

# insert output - go-sql can generate SQL insert statments as its output
# format from a select.  This tests that.
# xyzzy-error - need to have consisten order of output ---------------------------------- error
# xyzzy-error - data not in consistent order, columns are.
t04: go-sql
	@./go-sql -i td/t04.sel -o to/o04.out
	@$(DIFF) to/o04.out ref/o04.out
	@echo TEST 04: PASS!

ok-t04:
	@$(CP) to/o04.out ref/o04.out

# CSV output - with comment/header line
t05: go-sql
	@./go-sql -i td/t05.sel -o to/o05.out
	@$(DIFF) to/o05.out ref/o05.out
	@echo TEST 05: PASS!

# csv output - with out comment header line
t06: go-sql
	@./go-sql -i td/t06.sel -o to/o06.out
	@$(DIFF) to/o06.out ref/o06.out
	@echo TEST 06: PASS!

# csv first, then TEXT output
t07: go-sql
	@./go-sql -i td/t07.sel -o to/o07.out
	@$(DIFF) to/o07.out ref/o07.out
	@echo TEST 07: PASS!

# csv, then text output
t08: go-sql
	@./go-sql -i td/t08.sel -o to/o08.out
	@$(DIFF) to/o08.out ref/o08.out
	@echo TEST 08: PASS!

# json output
t09: go-sql
	@./go-sql -i td/t09.sel -o to/o09.out
	@$(DIFF) to/o09.out ref/o09.out
	@echo TEST 09: PASS!

# JSON output
t10: go-sql
	@./go-sql -i td/t10.sel -o to/o10.out
	@$(DIFF) to/o10.out ref/o10.out
	@echo TEST 10: PASS!

# Test gen. of report using template
# Save and merge data from multiple tables
t11: go-sql
	@./go-sql -i td/t11.sel -o to/o11.out
	@$(DIFF) to/o11.out ref/o11.out
	@$(DIFF) to/t11_1.rpt ref/t11_1.rpt
	@echo TEST 11: PASS!

# Test all of the CRUD commands in one script.  create table, insert, select, update, delete, drop
t16: go-sql
	@./go-sql -i td/t16.sel -o to/o16.out
	@$(DIFF) to/o16.out ref/o16.out
	@echo TEST 16: PASS!

# test template substitution in commands.
t17: go-sql
	@./go-sql -i td/t17.sel -o to/o17.out
	@$(DIFF) to/o17.out ref/o17.out
	@echo TEST 17: PASS!

# Test echo
t19: go-sql
	@./go-sql -i td/t19.sel -o to/o19.out
	@$(DIFF) to/o19.out ref/o19.out
	@echo TEST 19: PASS!

# Test spool of data to file
t22: go-sql
	@./go-sql -i td/t22.sel -o to/o22.out
	@$(DIFF) to/o22.out ref/o22.out
	@$(DIFF) to/o22.rpt ref/o22.rpt
	@echo TEST 22: PASS!

# Generate report and send via FTP to detination
t23: go-sql
	@./go-sql -i td/t23.sel -o to/o23.out
	@$(DIFF) to/o23.out ref/o23.out
	@$(DIFF) to/t23_1.rpt ref/t23_1.rpt
	@echo TEST 23: PASS!

# Send Email test
t24: go-sql
	@./go-sql -i td/t24.sel -o to/o24.out
	@$(DIFF) to/o24.out ref/o24.out
	@$(DIFF) to/o24.rpt ref/o24.rpt
	@echo TEST 24: PASS!

t12: go-sql
	@./go-sql -i td/t12.sel -o to/o12.out
	@$(DIFF) to/o12.out ref/o12.out
	@echo TEST 12: PASS!

t14: go-sql
	@./go-sql -i td/t14.sel -o to/o14.out
	@$(DIFF) to/o14.out ref/o14.out
	@echo TEST 14: PASS!

# not well automated test - but it runs and seems to produce good results for a 1st pass
# Mod this to be an automated test with a loop over known data.   Save the data back
# into the global data store.  Generate a simple report with it. (Post-Join Test)
t28: go-sql
	@./go-sql -i td/t28.sel -o to/o28.out
	@./rm_now.sh to/o28.out >.junk1 2>&1
	@./rm_now.sh ref/o28.out >.junk2 2>&1
	@$(DIFF) to/o28.out ref/o28.out
	@echo TEST 28: PASS!

# Initial test of generation of a PDF report using a Template for SO
t27: go-sql td/t27.sel
	./go-sql -i td/t27.sel -o to/o27.out "" "" " LIMIT 65 " "FromDate" "ToDate" "MineName"
	@./rm_now.sh to/o27.out >.junk1 2>&1
	@./rm_now.sh ref/o27.out >.junk2 2>&1
	@$(DIFF) to/o27.out ref/o27.out
	echo TEST 27: PASS!  Preliminary SO pdf report

# Current (*Sun Aug 17 16:43:56 MDT 2014*) test for the report for SO
# worked!
#  e3539842-5357-46bf-b30f-380b1214a9a4 | error  | {"cmd":["go-sql","-i","rpt-daily.rpt","-c","{\"auth_token\":\"10bd00e5-6258-436e-a581-7cbd5363885b\",\"dest\":\"Email\",\"to\":\"g@h.com\",\"subject\":\" Saftey Observation - 2014-08-17 04:50:36\",\"site\":\"\",\"from\":\"2014-01-05 05:00:00\",\"thru\":\"2014-08-12 05:00:00\"}","",""," Limit 42 ","FromDate","ToDate","MineName"]} |    |          |         |         | 2014-08-17 16:51:01.521996
t29: go-sql rpt-daily.rpt
	./go-sql -i rpt-daily.rpt -c '{"auth_token":"10bd00e5-6258-436e-a581-7cbd5363885b","dest":"Email","to":"pschlump@gmail.com","subject":"test t29","site":"","from":"2014-01-01 05:00:00","thru":"2015-01-01 05:00:00"}' "" "" " LIMIT 45 " "FromDate" "ToDate" "MineName"

t30: go-sql rpt-daily.rpt
	./go-sql -i rpt-daily-screen.rpt -c '{"auth_token":"10bd00e5-6258-436e-a581-7cbd5363885b","dest":"Email","to":"pschlump@gmail.com","subject":"test t29","site":"","from":"2014-01-01 05:00:00","thru":"2015-01-01 05:00:00"}' "" "" " LIMIT 45 " "FromDate" "ToDate" "MineName"

t31: go-sql 
	tsql -S 192.168.0.161 -U sa -P uuqqrm  <note/ms-sql-all-type.sql
	go-sql -i td/t31.sel -o to/o31.out
	@$(DIFF) to/o31.out ref/o31.out
	echo TEST 31: PASS!  Test of all data tyeps for MS-SQL

ok-t31: go-sql
	$(CP)  to/o31.out ref/o31.out

#// BuildNo: 000

upd_BuildNo:
	./updBuildNo.sh go-sql.go note.1
	( cd . ; make )
	git commit -m "Set Build No on who-cares" .
	git push origin master




#######################################################################################################
#
# invoice report
#
# 199 		- "id" from t_rpt_q
# 68... 	- "id" from p_invoice - the invoice id
#
test_invoice:
	./go-sql -i invoice-rpt.sql 199 "68e8a89d-d3f5-48a0-b85b-450b0dbb036a"


#######################################################################################################
#
# ship-rpt.sql
#
# 199 	- "id" from t_rpt_q
# u0 	- User ID
# 2 	- customer_id
#
test_shipping:
	./go-sql -i ship-rpt.sql 199 u0 2


#######################################################################################################
#
# packing slip
#
# 199 		- "id" from t_rpt_q
# 68... 	- "id" from p_invoice - the invoice id
#
packing_slip:
	./go-sql -i packing-slip.sql 199 "68e8a89d-d3f5-48a0-b85b-450b0dbb036a"


