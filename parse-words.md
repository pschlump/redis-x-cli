Parse Words Specification
=========================

Delimieters " " and "\t"

Quoting
	'...' - string
	"..." - string
	`...
	...` - Multi Line String
	<<XXxx - token based multi line string

XXxx
	<<"XX xx" - token based multi line string

XX xx


Examples

	> x.upd srp:U:* `data["xyz"] = "y"`

	> x.upd srp:U:* `where:data["xyz"]==="N"` `set:data["xyz"]="n"`

	> x.upd srp:U:* `where:data["email"]==="pschlump@gmail.com"` `set:data["UserName"] = data["email"]`


