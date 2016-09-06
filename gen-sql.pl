#!/usr/bin/perl

$db_0 = 0;
$db_1 = 0;
$st = 0;
$type = "s";

while ( <> ) {
	chomp;
	print "st:$st line:$_\n" if ( $db_0 );
	if ( $st == 0 && /^create table/ ) {
		$st = 1;
		($j1,$j2,$table_name,$j3) = split /[ \t]+/;
		$table_name =~ s/"//g;
		print "table_name=->$table_name<-\n" if ( $db_0 );
	} elsif ( $st >= 1 && /^\)/ ) {
		$st = 0;
		print <<XXxx;
			]
		}
XXxx
	} elsif ( $st == 1 ) {
		($j0,$col,$t1,$t2,$t3,$jX) = split /[ \t]+/;
		$col =~ s/"//g;
		print "col=->$col<- t1=$t1\n" if ( $db_0 );
		$st = 2;
		$type = ty_to_code($t1);
		print <<XXxx;
	,"/api/table/$table_name": { "crud": [ "select", "insert", "update", "delete" ]
		, "TableName": "$table_name"
		, "LineNo":"1000"
		, "nokey":false
		, "Method":["GET","POST","PUT","DELETE","HEAD"]
		, "CustomerIdPart": { "colType":"s", "colName":"customer_id" }
		, "cols": [
				  { "colName": "$col" 				, "colType": "$type" 			, "autoGen": true , "isPk": true }
XXxx
	} elsif ( $st == 2 ) {
		($j0,$col,$t1,$t2,$t3,$jX) = split /[ \t]+/;
		if ( $col eq "," ) {
			($j0,$j2,$col,$t1,$t2,$t3,$jX) = split /[ \t]+/;
		}
		print "col=->$col<- t1=$t1\n" if ( $db_0 );
		$col =~ s/,"//;
		$col =~ s/"//;
		$type = ty_to_code($t1);
		print "col=->$col<- t1=$t1\n" if ( $db_0 );
		if ( $col eq "dateUpdated" || $col eq "dateEntered" ) {
			$u = "";
		} else {
			$u = "\"update\":true,";
		}
		if ( $col ne ",primary" ) {
			print <<XXxx;
				, { "colName": "$col"				, "colType": "$type",	$u "insert":true		}
XXxx
		}
	}
}


sub ty_to_code {
	my ($ty) = @_;
	print "ty=>$ty<\n" if ( $db_1 );
	if ( $ty eq "float" || $ty eq "real" ) {
		return "f";
	} elsif ( $ty eq "int" || $ty eq "bigint" ) {
		return "i";
	} elsif ( $ty eq "timestamp" ) {
		return "d";
	}
	return "s";
}

__END__

	,"/api/table/tblDepartment": { "crud": [ "select", "insert", "update", "delete" ]
		, "TableName": "tblDepartment"
		, "LineNo":"1018"
		, "nokey":false
		, "Method":["GET","POST","PUT","DELETE","HEAD"]
		, "CustomerIdPart": { "colType":"s", "colName":"customer_id" }
		, "cols": [
				  { "colName": "id" 				, "colType": "s" 			, "autoGen": true , "isPk": true }
				, { "colName": "name"				, "colType": "s",	"update":true, "insert":true		}
				, { "colName": "description"		, "colType": "s",	"update":true, "insert":true		}
				, { "colName": "isDeleted"			, "colType": "i",	"update":true, "insert":true		}
			]
		}
