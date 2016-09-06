
create table "r_queue" (
	  "id"					char varying (40) DEFAULT uuid_generate_v4() not null primary key
	, "customer_id"			char varying (40) not null
	, "state"				char varying (50)
	, "email_addr"			char varying (250)
	, "params"				text
	, "updated" 			timestamp 									 						-- Project update timestamp (YYYYMMDDHHMMSS timestamp).
	, "created" 			timestamp default current_timestamp not null 						-- Project creation timestamp (YYYYMMDDHHMMSS timestamp).
);

insert into "r_queue" ( "customer_id", "state", "email_addr", "params" ) values
	( '1', 'please-run', 'a@b.c', '' )
;

