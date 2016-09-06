
Originally cloned from https://code.google.com/p/ftp4go/
Forked because needs patching.

Changes:

	1. Works with other unix ftp daemons - correctly returns
		path informaiton.
	2. Checks for local existence of files before attempting
		to upload them.  This prevents going boom.

Tested With:

	1. Linux 14.10 vsftpd
	2. Windows 8 IIS/ftp


