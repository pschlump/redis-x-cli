Redis Extended Command Line Interface
=====================================
 
## Overview

Redis has a capable CLI that comes with it.  This extends the 
CLI to understand JSON and adds a bunch of commands to the CLI.

This tool includes a templated report writer that can generate
reports in PDF.

The reports can be copied to a destination folder, pushed to a
server with FTP or emailed to a destination.

## License

(C) Copyright Philip Schlump, 2013-2016.

## Command Line

Each of the command line argumets is placed in the global store so that
they can be substiturted in commands.  __0__ is used for the name of the
program, __1__ is the first command line argument.  The first 20 will
be "" if not specified.

Example:

$ redis-cli " the time is ... "
rcli> set a {{.__1__}}

