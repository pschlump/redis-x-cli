# Configuration in glboal-cfg.json file

## File Location 

By default the file will be searched for using the search path of "~/cfg", "./cfg", and then in the current directory.
This is on Linux and Mac.  You can set the search path with the -S command line option.

On Windows the search path is "C:\cfg" and then current directory.

"~username" in a file name will be substituted for the home directory of the specified user.

"~" is for the home directory of the current user.


## File Name

By defulat the system will search the path for "global-cfg-<HOSTNAME>.json" first, then for "global-cfg.josn".
This allows for host-specific configuation files.

## Configuration Items


### Database connection and type

For the relational database there are 3 conection items: "connectToDbType", "connectToDbName" and
"connectToAuth".

	Database	Config Item			Example Value													Comment
	--------	-----------			-------------													-------------
	PostgreSQL	connectToDbType		postgres														Database Type
	PostgreSQL	connectToDbName		postgres(192.168.0.10:5432)										Any value - it is a comment
	PostgreSQL	connectToAuth		"user=UserName password=pw dbname=test host=192.168.0.151"		Connection String
	SQL Server	connectToDbType		odbc
	SQL Server	connectToDbName		"MS SQL/SqlServer on 192.168.0.11 via FreeTDS Odbc"
	SQL Server	connectToAuth		"DSN=T2; UID=sa; PWD=uuqqrm"
	Oracle		connectToDbType		oracle
	Oracle		connectToDbName		"Oracle 12.2 Database"
	Oracle 		connectToAuth		"scott/tiger@//192.168.0.101:1521/orcl"

For Redis there are 2 items: "redis_host" and "redis_port".  "redis_host" can be "" (empty string) for 
connecing to redis on the current host or an IP address.  "redis_port" is "6379" for the default
redis port.

xyzzy - need un/pw auth for redis?

### connectToDatabaseName

For ODBC/MS-SQL/SqlServer you also need to set:
"connectToDatabaseName":"Test" to the database you want to conect to as
default when you start up.

### sql-help

"sql-help" is the directory (usually a full path to it) where the help files for go-sql can be found.
Example "sql-help":"/Users/corwin/Projects/go-sql/sql-help"


### port

The port where tab-server1.go will listen on.  You need to set this to match "ip_port", and "ip_addr"

### ip_port

The IP address and prot where tab-server1.go will listen.  For example
"ip_port":"192.168.0.151:8090".

### ip_addr

The IP address where tab-server1.go will listen.  For example
"ip_addr":"192.168.0.151".

### 4LetterBadWords

A list of 4 letter words that you want to disalow in all passwords.  Look in one of the global-cfg.json
files if you can not illustrate this for yourself.  The words are blank seperated.

### JSON_Prefix

A string that all JSON responses will be prefixed with.  For AngularJS this is ")]}',\n".

### monitor_url

The url that is used to access the who-cares monetering server.
Example: "monitor_url":"http://localhost:8090".

### ToPdf

This is the locaiton of the HTML to PDF converter.
For Windows: "ToPdf": "C:/Program Files/wkhtmltopdf/bin/wkhtmltopdf.exe"
For MacOS: "ToPdf": "/usr/local/bin/wkhtmltopdf"
For Linux: "ToPdf": "/usr/local/bin/wkhtmltopdf"


### LimitPostJoinRows

tab-server1.go allows for post-joins on any /api/table/<name> set of data that is
fetched.  A post join will take the set of data and a colum and perform a secondary
search for a joined table - taking the results of the join and returning that as
an array on a new column that you specifiy.

This is a limit on how many rows it will post-join before quiting.  Your page size
should be smaller than this value.  Setting this value to "-1" will enable unlimited
post joins.  

Example: "LimitPostJoinRows": "200"

This has been added as a performance item and is still considered experimental.

### static_dir

The directory that tab-server1.go serves files from.  By default if this is not
set it will be "./static".  Other likely exampes are "./app", "./www" or you can
set it to a hard path.

### go-sql-search-path

This sets the search path for files that go-sql will execute.  The
default is ".;.." for the current directory and for the parent 
direcotry.


### ChdirTo

This is the path where reprots will be generated.  This is the file-system path.

### ChdirToURL

This is the URL that matches with ChdirTo for reports to be returned as URLs
rather than the content of the report.   Both .html and .pdf should be 
generated in this locaiton.

For urls to work in a mult-server environemtn the path will need to be shared
by all the servers.

#### Example:

	"ChdirTo":"/Users/corwin/Projects/so-m/www/to"

	"ChdirToURL":"/to"

### go-sql-[name]

This allows for the configuration of othe commands to be run by go-sql - if this is found 
in the configuration file then it will be used.  Otherwise the only command that will
be run is for Linux/Mac-OS X, "./bin/go-sql", for Windows "C:\Cfg\go-sql"

