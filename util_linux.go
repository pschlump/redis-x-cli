// code for linux

package main

import (
	ms "github.com/pschlump/templatestrings" // "../ms"
	sizlib "www.2c-why.com/vsizlib"          // "../go-lib/sizlib" // "github.com/pschlump/Go-FTL/server/sizlib" // "../go-lib/sizlib"
)

var IsWindows = false
var IsBSD = false
var IsLinux = true
var IsUnix = true

var defaultSearchPath = "~/cfg;./cfg;."

//var opts struct {
//	GlobalCfgFN string `short:"g" long:"globaCfgFile"    description:"Full path to global config" default:"global-cfg.json"`
//	EmailCfgFN  string `short:"e" long:"emailCfgFile"    description:"Path to email config" default:"~/.email/email-config.json"` // Relative path to $HOME
//	FTPCfgFN    string `short:"f" long:"ftpCfgFile"      description:"Path to ftp config" default:"~/.ftp/ftp-config.json"`       // Relative path to $HOME
//	InputFile   string `short:"i" long:"input"           description:"Input file name" default:"--stdin--"`
//	OutputFile  string `short:"o" long:"output"          description:"Ouptut file name" default:"--stdout--"`
//	CliCfg      string `short:"c" long:"config"          description:"JSON string for config of data" default:""`
//	Search      string `short:"S" long:"searchPath"      description:"SearchPath to use for config files" default:"~/cfg:./cfg:."`
//	AppName     string `short:"A" long:"application"     description:"Application to run"                  default:"go-sql"`
//}

func ExistsPath(cmd string, path []string) (string, bool) {
	var fn string
	// funcMap = map[string]func(cmd string, raw string) (t string){
	if _, ok := funcMap[cmd]; ok {
		return cmd, false
	}
	if IfQuit(1, cmd) {
		return cmd, false
	}
	if cmd[0:1] == g_file_sep {
		fn = cmd
		if sizlib.Exists(fn) {
			return fn, true
		}
	}
	if len(cmd) > 2 && cmd[0:2] == "~"+g_file_sep { /* ~/name */
		fn = ms.HomeDir() + g_file_sep + cmd[2:]
		if sizlib.Exists(fn) {
			return fn, true
		}
	}
	for _, v := range g_path {
		if v == "." {
			fn = cmd
		} else {
			fn = v + g_file_sep + cmd
		}
		if sizlib.Exists(fn) {
			return fn, true
		}
	}
	return cmd, false
}
