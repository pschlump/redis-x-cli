package main

/*
TODO:
	// xyzzyUpd - update JSON data

	1. Code to convert return value into (array/of/array) into Tree
		- Plus demo to JSON
	2. All the data types for x.get
	3. Setup Redis test environment with Redis, Cluster, Sentinal etc.
	4. Do some LUA
	5. Figure out the JSON stuff
		1. How to load a file to a key
		2. How to dump a key to a file
		3. How to pretty print JSON in a key

		read InMemKey Fn
		save InMemKey Fn
		set RKey {{.InMemKey}}

	6. Test reports directly from JSON in Redis			1h


	1. Create x.get -- Partially complete
		x.keys Pat - uses scan, sorts results on client side

	2. Write up README.md - full						4h
		x.del
		x.keys
		x.get
		x.?

	5. Implement a Redis Pluggin - and work with that.
	0. Test lots of Redis commands

http://stackoverflow.com/questions/4006324/how-to-atomically-delete-keys-matching-a-pattern-using-redis

scan
	https://github.com/andymccurdy/redis-py

Good LUA example
	https://blog.al4.co.nz/2014/08/safely-running-bulk-operations-on-redis-with-lua-scripts/

*/

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/pschlump/Go-FTL/server/lib"          //
	"github.com/pschlump/MiscLib"                    //
	"github.com/pschlump/com"                        // "encoding/json"
	em "github.com/pschlump/emailbuilder"            //
	tr "github.com/pschlump/godebug"                 //
	"github.com/pschlump/json"                       // "encoding/json"
	pw "github.com/pschlump/pw2"                     // parse-words -- new version
	"github.com/pschlump/radix.v2/redis"             //
	"github.com/pschlump/redis-x-cli/ftp4go"         // passed
	"github.com/pschlump/redis-x-cli/gofpdf"         // passed
	"github.com/pschlump/redis-x-cli/myreadline"     //
	"github.com/pschlump/redis-x-cli/terminal"       //
	sizlib "github.com/pschlump/redis-x-cli/vsizlib" //
	ms "github.com/pschlump/templatestrings"         //
	"github.com/robertkrimen/otto"
)

// https://github.com/dop251/goja -- extended/replacmeent 6x as fast as otto -- Looks to be drop in replacement!
// "github.com/robertkrimen/otto"

const (
	Version = "Version: 0.0.1"
)

const ISO8601 = "2006-01-02T15:04:05.99999Z07:00"

var g_quit bool = false
var g_prompt string = "rcli> "
var g_prompt0_x string = "rcli> "
var g_prompt2_x string = "+++> "
var g_prompt3_x string = "%03d> "

// var g_schema string = "public"
var g_path []string
var g_debug bool = false
var g_echo bool = false
var g_termtrunk bool = false
var g_file_sep string = string(os.PathSeparator)
var g_line_sep string = "\n" // Unix line ending
var g_data map[string]interface{}
var g_type map[string]string
var x_data *map[string]interface{}
var x_data_stk []*map[string]interface{}

var g_update_before_insert bool = false
var g_fmt string = "text"
var g_pk_col string = "id"
var g_target string = ""
var g_table string = "theTable"
var GlobalCfg map[string]string

type DispatchFunc struct {
	Fx  func(cmd string, raw string, nth int, words []string) (t string)
	In  []string // Input param types
	Out []string // output param types
}

var funcMap map[string]DispatchFunc

var db_fmt_1 bool = false

// Email
var email_connected bool = false
var email *em.EM

// FTP
var FTPConfig com.FtpUser
var ftp_connected bool = false
var ftpClient *ftp4go.FTP

// Database Quotes - for selects - for output look at g_target
var DbBeginQuote = `"`
var DbEndQuote = `"`

// Display settings - for - output.
var IsTerm bool = false
var TermHeight, TermWidth int

// var xOut io.Writer
var xOut *os.File

func ShowHelp(pth string, cmd string) {
	if cmd == "" {
		cmd = "INDEX"
	}
	fn := pth + com.PathSep + cmd
	s, err := ioutil.ReadFile(fn)
	if err != nil {
		fmt.Fprintf(xOut, "Error(): Unable to open %s - You need to configure \"sql-help\" in \"global-cfg.json\" to point to the help files, Error=%s\n", fn, err)
	} else {
		fmt.Fprintf(xOut, "%s\n", s)
	}
}

func SetPath(s string) {
	if s != "" {
		g_path = strings.Split(s, ";")
	} else {
		g_path = []string{"."}
	}
}

func SetColspec(cmd string, raw string, nth int, words []string) (rv string) {
	plist := ParseLineIntoWords(raw)
	if len(plist) > 1 {
		ReadColspec(plist[1])
	} else {
		fmt.Fprintf(xOut, "Error(): Missing Parameter.")
	}
	return ""
}

func InsurePlist(plist []string, need int) bool {
	if len(plist) >= need {
		return true
	}
	fmt.Fprintf(xOut, "Error(12009): Additional parameters needed\n")
	return false
}

func IfQuit(has_cmd int, cmd string) bool {
	if has_cmd > 0 && sizlib.InArray(cmd, []string{"quit", "\\q", "exit", "bye", ":q", ":q!", ":wq", "logout", "quit;", "exit;", "bye;", "logout;"}) {
		g_quit = true
	}
	return g_quit
}

func IfComment(has_cmd int, cmd string) bool {
	if has_cmd > 0 && sizlib.InArray(cmd, []string{"--", "//", "#"}) {
		return true
	}
	if has_cmd > 0 && (strings.HasPrefix(cmd, "--") || strings.HasPrefix(cmd, "//")) {
		return true
	}
	return false
}

func RunFile(fn string, inputCmdLineRaw string) {
	st := 0
	var multi string
	var cmd string
	var raw string
	var raw2 string
	var m_line_no int
	stack := NewStateStk()

	// parse "raw" into tokens - put tokens into g_data as [_0_], ....
	args := ParseLineIntoWords(raw)
	for i, v := range args { // set Args into global data
		SetValue(fmt.Sprintf("__%d__", i), v)
	}

	runItSaveIt := func(hist, line string) {
		if stack.Depth() == 0 {
			myreadline.AddHistory(hist) //allow user to recall this line
		}

		line = TrimCmd(line)
		// xyzzy72 line = ExecuteATemplate (line, g_data)
		f := ParseLineIntoWords(line)
		cmd := f[0]

		if stack.Depth() == 0 {
			RunCmd(1, cmd, line)
		} else {
			SaveCmd(1, cmd, line)
		}
	}

	// fmt.Printf("fn = ->%s<-, %s\n", fn, tr.LF())
	fileBytes, err := ioutil.ReadFile(fn)
	if err != nil {
		fmt.Fprintf(xOut, "Error(12035): Error reading %s, %s\n", fn, err)
		return
	}
	// for each line in file - parse and apply
	SetValue("__input_file__", fn)
	SetValue("__line_no__", "0")
	m_line_no = 0

Loop:
	for line_no, iline := range strings.Split(string(fileBytes), g_line_sep) {
		if g_echo {
			fmt.Fprintf(xOut, "%s\n", iline)
		}
		SetValue("__line_no__", fmt.Sprintf("%d", line_no))
		// fmt.Fprintf ( xOut, "B: %s\n", line )
		// xyzzy72 - line := ExecuteATemplate (iline, g_data)
		line := iline
		raw = line
		// fmt.Fprintf ( xOut, "A: %s\n", line )
		f := ParseLineIntoWords(line)
		if len(f) <= 0 {

		} else {

			cmd = f[0]
			switch st {
			case 0:
				if sizlib.InArray(strings.ToLower(f[0]), []string{"select", "update", "insert", "delete", "save", "create", "drop", "alter"}) {
					raw2 = strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
					if strings.HasSuffix(raw2, ";") {
						runItSaveIt(raw, raw2)
					} else {
						multi = raw + "\n"
						st = 1
					}
				} else if sizlib.InArray(f[0], []string{"loop"}) {
					multi = raw
					if stack.Depth() == 0 {
						m_line_no = 1
						seqData = SeqOne{Op: "loop", Record: f, Raw: multi}
					} else {
						m_line_no++
					}
					//st = 3
					//stack.Push ( st, f[0] )
					raw2 = strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
					if strings.HasSuffix(raw2, ";") {
						g_prompt = fmt.Sprintf(g_prompt3_x, m_line_no)
						st = 3
					} else {
						g_prompt = g_prompt2_x
						st = 2
					}
					stack.Push(3, f[0])
				} else if sizlib.InArray(f[0], []string{"end-loop", "endloop"}) {
					if stack.Depth() == 0 {
						fmt.Fprintf(os.Stderr, "Error(): found %s when not inside a loop\n", f[0])
					} else {
						// exit the current script.
						st, _ = stack.Pop()
						if st == 0 && stack.Depth() == 0 {
							RunCmd(1, "drive", "...")
						}
					}
				} else if sizlib.InArray(f[0], []string{"if"}) {
				} else if sizlib.InArray(f[0], []string{"else"}) {
				} else if sizlib.InArray(f[0], []string{"elseif", "else-if", "elsif", "eif"}) {
				} else if sizlib.InArray(f[0], []string{"end-if", "endif", "eif", "fi"}) {
				} else if IfComment(len(f), f[0]) {
					// don't do much for comments for that is what makes comments comments.
				} else if IfQuit(len(f), f[0]) { // check stack depth ( are we in a script? )
					if stack.Depth() == 0 {
						break Loop
					} else {
						// exit the current script.
					}
				} else if _, ok := funcMap[f[0]]; ok { // See if a built-in function - you can not run scripts with  these names
					if stack.Depth() == 0 {
						RunCmd(1, cmd, raw)
					} else {
						SaveCmd(1, cmd, raw)
					}
				} else if _, ok := ExistsPath(f[0], g_path); ok { // See if file is in _path_
					if stack.Depth() == 0 {
						RunCmd(1, cmd, raw)
					} else {
						SaveCmd(1, cmd, raw)
					}
				} else { // I think that this is an oops...
					fmt.Fprintf(os.Stderr, "Error(): Invalid command, line:%d cmd:%s\n", line_no, f[0])
				}
			case 1: /* multi Line Statment, collecting statement */
				// fmt.Printf("352-a: f[0] == ->%s<-\n", f[0])
				if f[0] == "/" {
					// fmt.Printf("case 1\n")
					runItSaveIt(multi, multi)
					st = stack.PeekSt(0)
				} else if strings.HasSuffix(f[len(f)-1], ";") {
					// fmt.Printf("case 2\n")
					multi += raw + "\n"
					// fmt.Printf("Multi ->%s<-\n", multi)
					runItSaveIt(multi, multi)
					// fmt.Printf("About to stack.PeekSt(0)\n")
					// fmt.Printf("Depth=%d\n", stack.Depth())
					st = stack.PeekSt(0)
					// fmt.Printf("After\n")
				} else {
					// fmt.Printf("case 3\n")
					multi += raw + "\n"
				}
			case 2: /* multi Line LOOP Statment, collecting statement */
				// fmt.Printf ( "case 2: f=%v\n", f )
				if strings.HasSuffix(f[len(f)-1], ";") {
					multi += raw + "\n"
					// fmt.Printf ( "found ;, multi=>%s<-\n", multi )
					m_line_no = 1
					f2 := ParseLineIntoWords(multi)
					seqData = SeqOne{Op: "loop", Record: f2, Raw: multi}
					// SaveCmd ( 1, "loop", multi )
					st = 3
					g_prompt = fmt.Sprintf(g_prompt3_x, m_line_no)
				} else {
					multi += raw + "\n"
					g_prompt = g_prompt2_x
				}
			case 3: /* loop...end-loop */
				m_line_no++
				if sizlib.InArray(strings.ToLower(f[0]), []string{"select", "update", "insert", "delete", "save", "create", "drop", "alter"}) {
					raw2 := strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
					if strings.HasSuffix(raw2, ";") {
						SaveCmd(1, cmd, raw)
					} else {
						multi = raw
						st = 1
					}
				} else if sizlib.InArray(f[0], []string{"loop"}) {
					multi = raw
					st = 3
					stack.Push(st, f[0])
					SaveCmd(1, cmd, raw)
				} else if sizlib.InArray(f[0], []string{"end-loop", "endloop"}) {
					// exit the current script.
					SaveCmd(1, cmd, raw)
					st, _ = stack.Pop()
					if stack.Depth() == 0 {
						// fmt.Printf ( "Probably should run command at this point- call func with data structure (drive), %s\n", tr.SVarI(seqData) )
						_ = EndLoop()
						g_prompt = g_prompt0_x
						st = 0
					}
				} else if sizlib.InArray(f[0], []string{"if"}) {
				} else if sizlib.InArray(f[0], []string{"else"}) {
				} else if sizlib.InArray(f[0], []string{"elseif", "else-if", "elsif", "eif"}) {
				} else if sizlib.InArray(f[0], []string{"end-if", "endif", "eif", "fi"}) {
				} else if IfComment(len(f), f[0]) {
					// Much ado about noting, thus comments be comments.
				} else if IfQuit(len(f), f[0]) { // check stack depth ( are we in a script? )
					if stack.Depth() == 0 {
						break Loop
					} else {
						// exit the current script.
					}
				} else { // I think that this is an oops...
					SaveCmd(1, cmd, raw)
				}
			case 4: /* if...end-if */
			default:
			}

		}
	}
}

// =======================================================================================================================================================================
// =======================================================================================================================================================================

var seqData SeqOne
var g_sv int = 0

func getSv(d int) *[]SeqOne {
	t := &(seqData.Cmds)
	for i := 0; i < d; i++ {
		m := len(*t) - 1
		t = &(((*t)[m]).Cmds)
	}
	return t
}

func newSv(r []string, raw string) {
	sv := getSv(g_sv)
	(*sv) = append((*sv), SeqOne{Op: "loop", Record: r, Raw: raw})
	g_sv++
}
func appendSv(r []string, raw string) {
	sv := getSv(g_sv)
	(*sv) = append((*sv), SeqOne{Op: "cmd", Record: r, Raw: raw})
}
func popSv() {
	if g_sv > 0 {
		g_sv--
	}
}

func SaveCmd(has_cmd int, cmd string, raw string) {
	f := ParseLineIntoWords(raw)
	switch cmd {
	case "loop":
		newSv(f, raw)
	case "end-loop":
		popSv()
	default:
		appendSv(f, raw)
	}
}

// =======================================================================================================================================================================
// =======================================================================================================================================================================

func RunCmd(has_cmd int, cmd string, raw string) {

	if has_cmd <= 0 {
		return
	}
	if g_debug {
		fmt.Fprintf(xOut, "RunCmd: ->%s<- ->%s<-\n", cmd, raw)
	}

	if pthFile, ok := ExistsPath(cmd, g_path); ok { // See if file is in _path_
		// fmt.Fprintf ( xOut, "Found command at %s\n", pthFile )

		RunFile(pthFile, raw)
	} else {
		if fx, ok := funcMap[cmd]; ok {
			// xyzzy72 - this is the point where substitution should occure
			raw2 := ExecuteATemplate(raw, g_data)
			// fmt.Printf ( "Before/after: ->%s<- ->%s<-\n", raw, raw2 )
			f := ParseLineIntoWords(raw2)
			rv := fx.Fx(cmd, raw2, -1, f)
			fmt.Fprintf(xOut, "%s\n", rv)
		} else {
			fmt.Fprintf(xOut, "Error(12008): Invalid function to call (%s)\n", cmd)
		}
	}
}

func SetValue(name string, value interface{}) {
	// fmt.Fprintf ( xOut, "name=%s value=%v, %s\n", name, value, tr.LF() )
	switch value.(type) {
	case string:
		switch name {
		case "__path__":
			SetPath(value.(string))
			g_data[name] = value.(string)
		case "debug":
			fallthrough
		case "__debug__":
			g_debug = (value.(string) == "on")
			g_data[name] = value.(string)
		case "termtrunk":
			g_termtrunk = (value.(string) == "on")
			g_data[name] = value.(string)
		case "echo":
			fallthrough
		case "__echo__":
			g_echo = (value.(string) == "on")
			g_data[name] = value.(string)
		case "update_before_insert":
			fallthrough
		case "__update_before_insert__":
			g_update_before_insert = (value.(string) == "on")
			g_data[name] = value.(string)
		case "target":
			fallthrough
		case "__target__":
			g_target = value.(string)
			g_data[name] = value.(string)
		case "pk_col":
			fallthrough
		case "__pk_col__":
			g_pk_col = value.(string)
			g_data[name] = value.(string)
		case "fmt":
			fallthrough
		case "__fmt__":
			inArr := []string{"json", "JSON", "xml", "XML", "insert", "csv", "CSV", "text", "TEXT", "txt-fmt"}
			if sizlib.InArray(value.(string), inArr) {
				g_fmt = value.(string)
				g_data["__fmt__"] = value.(string)
			} else {
				fmt.Fprintf(xOut, "Error(): Invalid set fmt, fmt must be one of %v\n", inArr)
			}
			// fmt.Fprintf ( xOut, "fmt = %s\n", g_fmt )
		case "table":
			fallthrough
		case "__table__":
			g_table = value.(string)
			g_data["__table__"] = value.(string)
		case "__prompt__":
			g_prompt = value.(string)
			g_prompt0_x = value.(string)
		case "__prompt2__":
			g_prompt2_x = value.(string)
		default:
			g_data[name] = value.(string)
		}
	case int, int64, float32, float64:
		g_data[name] = fmt.Sprintf("%v", value)
	case time.Time:
		// fmt.Fprintf ( xOut, "found a time.Time, %s = %s\n", i, (v.(time.Time)).Format(ISO8601) )
		g_data[name] = (value.(time.Time)).Format(ISO8601)
	case bool:
		g_data[name] = fmt.Sprintf("%v", value)
	default:
		g_data[name] = value
	}
}

func PrintValue(name string) {
	x := g_data[name]
	switch x.(type) {
	case string:
		fmt.Fprintf(xOut, "name=%s\n", x.(string))
	case int, int64, float32, float64, time.Time, bool:
		fmt.Fprintf(xOut, "name=%v\n", x)
	default:
		fmt.Fprintf(xOut, "name=%s\n", tr.SVarI(x))
	}
}

func GetValue(name string) (rv string) {
	rv = ""
	x, ok := g_data[name]
	if !ok {
		return
	}
	switch x.(type) {
	case string:
		rv = x.(string)
	case int, int64, float32, float64, time.Time, bool:
		rv = fmt.Sprintf("%v", x)
	default:
		rv = fmt.Sprintf("%s", tr.SVar(x))
	}
	return
}

func ParseLineIntoWords(line string) []string {
	// rv := strings.Fields ( line )
	Pw := pw.NewParseWords()
	// Pw.SetOptions("C", true, true)
	Pw.SetOptions("Go", false, true)
	Pw.SetLine(line)
	rv := Pw.GetWords()
	return rv
}

//func ParseLineIntoWordsNOQ(line string) []string {
//	// rv := strings.Fields ( line )
//	Pw := pw.NewParseWords()
//	Pw.SetOptions("Go", false, true)
//	Pw.SetLine(line)
//	rv := Pw.GetWords()
//	return rv
//}

// =============================================================================================================================================================
func ExecuteATemplate(tmpl string, data map[string]interface{}) string {
	funcMapTmpl := template.FuncMap{
		"title":       strings.Title,  // The name "title" is what the function will be called in the template text.
		"g":           global_g,       //
		"set":         global_set,     //
		"Center":      ms.CenterStr,   //
		"PadR":        ms.PadOnRight,  //
		"PadL":        ms.PadOnLeft,   //
		"PicTime":     ms.PicTime,     //
		"FTime":       ms.StrFTime,    //
		"PicFloat":    ms.PicFloat,    //
		"nvl":         ms.Nvl,         //
		"Concat":      ms.Concat,      //
		"ifDef":       ms.IfDef,       //
		"ifIsDef":     ms.IfIsDef,     //
		"ifIsNotNull": ms.IfIsNotNull, //
	}
	t := template.New("line-template").Funcs(funcMapTmpl)
	t, err := t.Parse(tmpl)
	if err != nil {
		fmt.Fprintf(xOut, "Error(): Invalid template: %s\n", err)
		return tmpl
	}

	// Create an io.Writer to write to a string
	var b bytes.Buffer
	foo := bufio.NewWriter(&b)
	err = t.ExecuteTemplate(foo, "line-template", data)
	// err = t.ExecuteTemplate(os.Stdout, "line-template", data)
	if err != nil {
		fmt.Fprintf(xOut, "Error(): Invalid template processing: %s\n", err)
		return tmpl
	}
	foo.Flush()
	s := b.String() // Fetch the data back from the buffer
	// fmt.Fprintf ( xOut, "\nbuffer = %v s=->%s<-\n", b, s )
	return s
}

// =============================================================================================================================================================
func ExecuteATemplateByName(tmpl, tname string, data map[string]interface{}) string {
	funcMapTmpl := template.FuncMap{
		"title":       strings.Title,  // The name "title" is what the function will be called in the template text.
		"g":           global_g,       //
		"set":         global_set,     //
		"Center":      ms.CenterStr,   //
		"PadR":        ms.PadOnRight,  //
		"PadL":        ms.PadOnLeft,   //
		"PicTime":     ms.PicTime,     //
		"FTime":       ms.StrFTime,    //
		"PicFloat":    ms.PicFloat,    //
		"nvl":         ms.Nvl,         //
		"Concat":      ms.Concat,      //
		"ifDef":       ms.IfDef,       //
		"ifIsDef":     ms.IfIsDef,     //
		"ifIsNotNull": ms.IfIsNotNull, //
	}
	t := template.New("line-template").Funcs(funcMapTmpl)
	t, err := t.Parse(tmpl)
	if err != nil {
		fmt.Fprintf(xOut, "Error(): Invalid template: %s\n", err)
		return tmpl
	}

	// Create an io.Writer to write to a string
	var b bytes.Buffer
	foo := bufio.NewWriter(&b)
	err = t.ExecuteTemplate(foo, tname, data)
	// err = t.ExecuteTemplate(os.Stdout, "line-template", data)
	if err != nil {
		fmt.Fprintf(xOut, "Error(): Invalid template processing: %s\n", err)
		return tmpl
	}
	foo.Flush()
	s := b.String() // Fetch the data back from the buffer
	// fmt.Fprintf ( xOut, "\nbuffer = %v s=->%s<-\n", b, s )
	return s
}

// =============================================================================================================================================================
type StringLC []string

func (s StringLC) Len() int      { return len(s) }
func (s StringLC) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s StringLC) Less(i, j int) bool {
	if s[i] == "_row_" {
		return true
	}
	if s[j] == "_row_" {
		return false
	}
	return strings.ToLower(s[i]) < strings.ToLower(s[j])
}

// =============================================================================================================================================================
// Data To ... Section
// =============================================================================================================================================================

//
// Output the results of a select as a table.
//
// You can truncate to fit your terminal.  Example:
//
func dataToText(data []map[string]interface{}, hdr bool) string {
	rv := ""
	s := ""
	com := ""
	cols := ""
	com = ""
	if len(data) <= 0 {
		return "-- no data --\n"
	}
	nam := make([]string, len(data[0])+1)
	width := make([]int, len(data[0])+1)
	for j := 0; j < len(data[0])+1; j++ {
		width[j] = 1
	}
	n_cols := 1
	nam[0] = "_row_"
	width[0] = len("_row_")
	for j, _ := range data[0] {
		nam[n_cols] = j
		//if width[n_cols] < len(j) {
		//	width[n_cols] = len(j)
		//}
		n_cols++
	}

	// xyzzy100 - need to sort with _row_ first - custom sort comapre

	// sort.Strings(nam)
	namLC := make(StringLC, len(nam))
	namLC = nam
	sort.Sort(namLC)
	nam = namLC

	for j, v := range nam {
		if width[j] < len(v) {
			width[j] = len(v)
		}
	}
	for i, v := range data {
		v["_row_"] = fmt.Sprintf("%d", i+1)
		for j := 0; j < n_cols; j++ {
			w := v[nam[j]]
			s = ""
			switch w.(type) {
			case string:
				s = w.(string)
			default:
				s = fmt.Sprintf("%v", w)
			}
			if width[j] < len(s) {
				width[j] = len(s)
			}
		}
	}
	cols = ""
	for j, c := range nam {
		ff := fmt.Sprintf("%%-%ds", width[j])
		cols += com + fmt.Sprintf(ff, c)
		com = " | "
	}

	if g_termtrunk {
		if IsTerm {
			// fmt.Printf ( "IsTerm is true, %s\n", tr.LF() )
			if len(cols) > TermWidth && TermWidth > 5 {
				cols = cols[0:TermWidth-4] + "..."
			}
		}
	}

	rv += cols + "\n"
	for i, v := range data {
		// v["_row_"] = fmt.Sprintf ( "%d", i+1 )
		v["_row_"] = i + 1
		vals := ""
		com = ""
		// for _, w := range v {
		for j := 0; j < n_cols; j++ {
			w := v[nam[j]]
			switch w.(type) {
			case string:
				// fmt.Printf ( "its a string at %d\n", j )  // xyzzy101 - UniqueIdentifier returned as string
				ff := fmt.Sprintf("%%-%ds", width[j])
				vals += com + fmt.Sprintf(ff, w.(string))
			default:
				// fmt.Printf ( "its a default at %d\n", j )
				ff := fmt.Sprintf("%%%ds", width[j])
				s = fmt.Sprintf(ff, fmt.Sprintf("%v", w))
				vals += com + s
			}
			com = " | "
		}

		if g_termtrunk {
			// fmt.Printf ( "len(vals) = %d, %s\n", len(vals), tr.LF() )
			if IsTerm {
				// fmt.Printf ( "IsTerm is true, %s\n", tr.LF() )
				if len(vals) > TermWidth && TermWidth > 5 {
					vals = vals[0:TermWidth-4] + "..."
				}
			}
		}

		// mdata["_cols"] = cols
		// mdata["_vals"] = vals
		// mdata["_i"] = fmt.Sprintf ( "%d", i )
		// rv = rv + sizlib.Qt ( "insert into %{table%} ( %{_cols%} ) values ( %{_vals%} );\n", mdata )
		rv = rv + vals + "\n"
	}
	return rv
}

//
// Take data from a a select and convert it into inserts.  You can precede the insers with updates.  This allows for
// moving data from one database to another.
//
// Example:
//		-- Test 40 -
//		// Test selecting data to insert statements that are targeted to a different database with insert_before_update turned on
//		--
//		set target odbc
//		set fmt insert
//		set update_before_insert on
//		set table bob2
//		select * from "t_test_crud2";
//		quit
//
// "set target odbc" says the destination is MS-SQL SqlServer
// "set fmt insert" make the select statment generate inser statments
// "set update_before_insert" says generate update statments before the insert statments.
// "set table bob2" says the name of the table that will be inserted/update is called "bob2"
// "select..." - query the connected database.
//
func dataToInsertStmt(data []map[string]interface{}) string {

	bq := `"`
	eq := `"`
	g_data["_bq_"] = `"`
	g_data["_eq_"] = `"`
	switch g_target {
	default:
		fallthrough
	case "":
	case "oracle":

	case "odbc":
		fallthrough
	case "SqlServer":
		fallthrough
	case "ms-sql":
		g_data["_bq_"] = `[`
		g_data["_eq_"] = `]`
		bq = `[`
		eq = `]`

	}

	rv := ""
	s := ""
	com := ""
	com2 := ""
	nam := make([]string, len(data[0])+1)
	n_cols := 0
	if len(data) > 0 {
		for j, _ := range data[0] {
			nam[n_cols] = j
			n_cols++
		}
		sort.Strings(nam)
		for i, v := range data {
			cols := ""
			vals := ""
			set := ""
			pk := ""
			com = ""
			com2 = ""
			// Disadvantage - columns may not be in correct order, but if some are left out then that is ok.
			// for j, w := range v {
			for j := 0; j < len(nam); j++ {
				w := v[nam[j]]

				// --- insert columns and values ----------------------------------------------------------------
				if w != nil {
					cols += com + bq + nam[j] + eq
					switch w.(type) {
					case string:
						q := fmt.Sprintf("%s", w)
						if q == "<nil>" {
							vals += com + "NULL"
						} else {
							s = strings.Replace(w.(string), `'`, "''", -1)
							vals += com + `'` + s + `'`
						}
					default:
						s = fmt.Sprintf("%v", w)
						if s == "<nil>" {
							vals += com + "NULL"
						} else {
							vals += com + `'` + s + `'`
						}
					}
					com = ", "
				} // else {
				// 	cols += com + bq + nam[j] + eq
				// 	vals += com + "NULL"
				// 	com = ", "
				// }

				// --- update columns and values ----------------------------------------------------------------
				if w != nil {
					if nam[j] == g_pk_col {
						pk = bq + g_pk_col + eq
						switch w.(type) {
						case string:
							q := fmt.Sprintf("%s", w)
							if q == "<nil>" {
								pk += " = NULL"
							} else {
								s = strings.Replace(w.(string), `'`, "''", -1)
								pk += ` = '` + s + `'`
							}
						default:
							s = fmt.Sprintf("%v", w)
							if s == "<nil>" {
								pk += " = NULL"
							} else {
								pk += ` = '` + s + `'`
							}
						}
					} else {
						set += com2 + bq + nam[j] + eq
						switch w.(type) {
						case string:
							q := fmt.Sprintf("%s", w)
							if q == "<nil>" {
								set += " = NULL"
							} else {
								s = strings.Replace(w.(string), `'`, "''", -1)
								set += ` = '` + s + `'`
							}
						default:
							s = fmt.Sprintf("%v", w)
							if s == "<nil>" {
								set += " = NULL"
							} else {
								set += ` = '` + s + `'`
							}
						}
						com2 = ", "
					}
				} // else {
				//	set += set + bq + nam[j] + eq + " = NULL "
				//	com2 = ", "
				//}

			}

			g_data["_i_"] = fmt.Sprintf("%d", i)

			g_data["_cols_"] = cols
			g_data["_vals_"] = vals

			g_data["_update_set_"] = set
			g_data["_where_pk_"] = pk

			if g_update_before_insert {
				rv = rv + ExecuteATemplate("update {{._bq_}}{{.__table__}}{{._eq_}} set {{._update_set_}} where {{._where_pk_}};\n", g_data)
			}

			rv = rv + ExecuteATemplate("insert into {{._bq_}}{{.__table__}}{{._eq_}} ( {{._cols_}} ) values ( {{._vals_}} );\n", g_data)
			// should have a SetupATemplate ( tmpl ), RunATemplate ( g_data )

			// xyzzy - add in "update" -- ?set update_also on
		}
	}
	return rv
}

// Convert data set to XML.
func dataToXML(data []map[string]interface{}, hdr bool) string {
	rv := ""
	if hdr {
		rv = "<?xml version='1.0' encoding='UTF-8'?>\n"
	}
	rv += "<data>\n"
	s := ""
	nam := make([]string, len(data[0])+1)
	n_cols := 0
	for j, _ := range data[0] {
		nam[n_cols] = j
		n_cols++
	}
	sort.Strings(nam)
	for i, v := range data {
		g_data["_i"] = fmt.Sprintf("%d", i)
		rv += fmt.Sprintf("\t<row rowno=\"%d\">\n", i)
		// for j, w := range v {
		for j := 0; j < n_cols; j++ {
			w := v[nam[j]]
			g_data["_j"] = fmt.Sprintf("%d", j)
			switch w.(type) {
			case string:
				s = w.(string)
			default:
				s = fmt.Sprintf("%v", w)
			}
			rv += "\t\t<" + nam[j] + ">" + s + "</" + nam[j] + ">\n"
		}
		rv += "\t</row>\n"
	}
	rv += "</data>\n"
	return rv
}

// Convert data set to Common Seperated Values, CSV.
func dataToCsv(data []map[string]interface{}, hdr bool) string {
	s := ""
	rv := ""
	com := ""
	nam := make([]string, len(data[0])+1)
	n_cols := 0
	for j, _ := range data[0] {
		nam[n_cols] = j
		n_cols++
	}
	sort.Strings(nam)
	if hdr {
		if len(data) <= 0 {
			return "# -- no data --\n"
		}
		com = ""
		cols := ""
		for _, s := range nam {
			if strings.Contains(s, ",") {
				s = strings.Replace(s, `"`, `\"`, -1)
				s = `"` + s + `"`
			}
			cols += com + `"` + s + `"`
			com = ", "
		}
		rv = cols + "\n"
	}
	com = ""
	for i, v := range data {
		g_data["_i_"] = fmt.Sprintf("%d", i)
		com = ""
		// for j, w := range v {
		for j := 0; j < n_cols; j++ {
			w := v[nam[j]]
			g_data["_j_"] = fmt.Sprintf("%d", j)
			switch w.(type) {
			case string:
				s = w.(string)
			default:
				s = fmt.Sprintf("%v", w)
			}
			if strings.Contains(s, ",") {
				s = strings.Replace(s, `"`, `\"`, -1)
				s = `"` + s + `"`
			}
			rv += com + s
			com = ","
		}
		rv += "\n"
	}
	return rv
}

// ===================================================================================================================================================
// Back to
// Column Formatting
// ===================================================================================================================================================

type FmtCols struct {
	ColName         string // Required!!!
	Width           int    // max width of data and column name
	B_Width         bool   // true
	Format          string // %v
	Justify         string // Default is L for strings, R for numbers
	ColTitle        string // Default is column name
	ColTitleJustify string // Default is C

	col_no int
}

type FmtColsRaw struct {
	ColName         *string // Required!!!
	Width           *int    // max width of data and column name
	Format          *string // %v
	Justify         *string // Default is L for strings, R for numbers
	ColTitle        *string // Default is column name
	ColTitleJustify *string // Default is C
}

type FmtTxt struct {
	Cols            []FmtColsRaw // 			(ip)
	ColsData        []FmtCols
	SepLine         bool    // false	(done)
	RepHeader       bool    // false
	RepHeaderFreq   int     // 60
	RowNumColumn    *bool   // false	<<< next >>>
	Pageing         bool    // false
	PageingNumStart int     // 0
	QtHdrTmpl       string  //
	QtRowTmpl       string  //
	ColSep          *string // ' | '	(done)
	TitleLine       *bool   // True		(done)
	TitleChars      *string // '-+'		(done)
	HeadersOn       *bool   // True

	titleChars string // '-+'		(done)
	colSep     string // ' | '	(done)

	rowNumColumn bool // false
	titleLine    bool // True		(done)
	headersOn    bool // True		(done)
}

type PdfReportLayout struct {
	Paper      string  // A4 defulat
	PaperWidth float64 // Will be set from Paper based on that, in mm
	FontDir    string  // ./font

	HdrFontSize  float64   // 14
	HdrFillColor PdfIColor // red(255,0,0)
	HdrTextColor PdfIColor // white(255,255,255)

	BorderColor PdfIColor // white(128,0,0)
	BorderWidth float64   // 0.3

	BodyFontSize  float64   // 12
	BodyFillColor PdfIColor // teal(224,235,255)
	BodyTextColor PdfIColor // white(255,255,255)
	ZebraStripe   int       // 1, if 0 then no stripes

	HasTitle bool   // Turn on page title
	TitleStr string // String to use

	PageNo bool // Turn on page numbering

	PadWidth float64 // Defualt 5.0 pixels to pad

	ColsData []FmtCols
}

type PdfReportLayoutRaw struct {
	Paper *string // A4 defulat

	HdrFontSize  *float64   // 14
	HdrFillColor *PdfIColor // red(255,0,0)
	HdrTextColor *PdfIColor // white(255,255,255)

	BorderColor *PdfIColor // white(128,0,0)
	BorderWidth *float64   // 0.3

	BodyFontSize  *float64   // 12
	BodyFillColor *PdfIColor // teal(224,235,255)
	BodyTextColor *PdfIColor // white(255,255,255)
	ZebraStripe   *int       // 1, if 0 then no stripes

	HasTitle *bool   // Turn on page title
	TitleStr *string // String to use

	PageNo *bool // Turn on page numbering

	PadWidth *float64 // Defualt 5.0 pixels to pad

	ColsData []FmtColsRaw
}

var g_colspec FmtTxt

func processNewFormat() {
	g_colspec.ColsData = make([]FmtCols, 0, len(g_colspec.Cols))
	for i, v := range g_colspec.Cols {
		g_colspec.ColsData = append(g_colspec.ColsData, FmtCols{
			ColName:         sIfNil(v.ColName, fmt.Sprintf("col:%d", i)),
			Width:           jIfNil(v.Width, 1),
			B_Width:         jIfNil3(v.Width, false, true),
			Format:          sIfNil(v.Format, "Fmt %v"),
			Justify:         sIfNil(v.Justify, "L"),
			ColTitle:        sIfNil(v.ColTitle, strings.Title(sIfNil(v.ColName, fmt.Sprintf("col:%d", i)))),
			ColTitleJustify: sIfNil(v.ColTitleJustify, "C"),
		})
	}

	g_colspec.TitleLine = bIfNil2(g_colspec.TitleLine, true)
	g_colspec.titleLine = *g_colspec.TitleLine

	g_colspec.HeadersOn = bIfNil2(g_colspec.HeadersOn, true)
	g_colspec.headersOn = *g_colspec.HeadersOn

	g_colspec.RowNumColumn = bIfNil2(g_colspec.RowNumColumn, true)
	g_colspec.rowNumColumn = *g_colspec.RowNumColumn

	g_colspec.TitleChars = sIfNil2(g_colspec.TitleChars, "-+")
	g_colspec.titleChars = *g_colspec.TitleChars

	g_colspec.ColSep = sIfNil2(g_colspec.ColSep, " | ")
	g_colspec.colSep = *g_colspec.ColSep

}

// -----------------------------------------------------------------------------------------------------------------------------------------------------------------
func sIfNil(p *string, d string) string {
	if p == nil {
		return d
	}
	return *p
}
func iIfNil(p *int64, d int64) int64 {
	if p == nil {
		return d
	}
	return *p
}
func jIfNil(p *int, d int) int {
	if p == nil {
		return d
	}
	return *p
}
func bIfNil(p *bool, d bool) bool {
	if p == nil {
		return d
	}
	return *p
}

// g_colspec.RowNumColumn	= bIfNil2 ( g_colspec.RowNumColumn, true )
func bIfNil2(p *bool, d bool) *bool {
	if p == nil {
		var x bool
		x = d
		return &x
	}
	return p
}
func sIfNil2(p *string, d string) *string {
	if p == nil {
		var x string
		x = d
		return &x
	}
	return p
}
func fIfNil(p *float64, d float64) float64 {
	if p == nil {
		return d
	}
	return *p
}
func dIfNil(p *time.Time, d time.Time) (z time.Time) {
	if p == nil {
		return d
	}
	z = *p
	return
}

// --------------------------------------------------------------------------------------------------------------------------------------------------------------------------
// --------------------------------------------------------------------------------------------------------------------------------------------------------------------------
var g_PdfSpec PdfReportLayout

func SetPdfspec(cmd string, raw string, nth int, words []string) (rv string) {
	var err error
	rv = ""
	plist := ParseLineIntoWords(raw)
	if len(plist) > 1 {
		g_PdfSpec, err = ReadPdfCfg(plist[1])
		if err != nil {
			fmt.Fprintf(xOut, "Error(): Unable to read format from file, %s, error=%s\n", plist[1], err)
		}
	} else {
		fmt.Fprintf(xOut, "Error(): Missing Parameter\n")
	}
	return ""
}

var g_gofpdf_font = "./gofpdf/font" // xyzzy - should be pulled from global data
// , "gofpdf_font": "./gofpdf/font"

// --------------------------------------------------------------------------------------------------------------------------------------------------------------------------
// --------------------------------------------------------------------------------------------------------------------------------------------------------------------------
func ReadPdfCfg(fn string) (cfg PdfReportLayout, err error) {
	var cfgRaw PdfReportLayoutRaw
	s := sizlib.GetFile(fn)
	err = json.Unmarshal([]byte(s), &cfgRaw)
	if err != nil {
		fmt.Fprintf(os.Stdout, "Error(12001): Invalid format - %v\n", err)
	} else {
		cfg.FontDir = g_gofpdf_font // xyzzy - should be pulled from global data

		if cfgRaw.Paper == nil {
			cfg.Paper = "A4"
			cfg.PaperWidth = com.GetPaperWidth("")
		} else {
			valid := []string{"4A0", "2A0", "A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9", "A10"}
			if sizlib.InArray(*cfgRaw.Paper, valid) {
				cfg.Paper = *cfgRaw.Paper
				cfg.PaperWidth = com.GetPaperWidth(*cfgRaw.Paper)
			} else {
				fmt.Fprintf(xOut, "Error(): Invalid type of paper, %s found, must be one of: %v\n", *cfgRaw.Paper, valid)
			}
		}

		cfg.HdrFontSize = fIfNil(cfgRaw.HdrFontSize, 14)

		cfg.HdrFillColor = clrIfNil(cfgRaw.HdrFillColor, PdfIColor{0x91, 0xb2, 0xf5})
		cfg.HdrTextColor = clrIfNil(cfgRaw.HdrTextColor, PdfIColor{0, 0, 0})

		cfg.BorderColor = clrIfNil(cfgRaw.BorderColor, PdfIColor{12, 12, 12})
		cfg.BorderWidth = fIfNil(cfgRaw.BorderWidth, 0.2)

		cfg.BodyFontSize = fIfNil(cfgRaw.BodyFontSize, 12)
		cfg.BodyFillColor = clrIfNil(cfgRaw.BodyFillColor, PdfIColor{222, 234, 255})
		cfg.BodyTextColor = clrIfNil(cfgRaw.BodyTextColor, PdfIColor{0, 0, 0})
		cfg.ZebraStripe = jIfNil(cfgRaw.ZebraStripe, 1)

		cfg.PageNo = bIfNil(cfgRaw.PageNo, true)

		cfg.HasTitle = bIfNil(cfgRaw.HasTitle, true)
		cfg.TitleStr = sIfNil(cfgRaw.TitleStr, "Report")

		cfg.PadWidth = fIfNil(cfgRaw.PadWidth, 5.0)
		if len(cfgRaw.ColsData) <= 0 {
			fmt.Fprintf(xOut, "Error(): Probably not correct to have a report format with no columns of data specified.\n")
		} else {
			for i, v := range cfgRaw.ColsData {
				cfg.ColsData = append(cfg.ColsData, FmtCols{
					ColName:         sIfNil(v.ColName, fmt.Sprintf("col:%d", i)),
					Width:           jIfNil(v.Width, 1),
					B_Width:         jIfNil3(v.Width, false, true),
					Format:          sIfNil(v.Format, ""),
					Justify:         sIfNil(v.Justify, "L"),
					ColTitle:        sIfNil(v.ColTitle, strings.Title(sIfNil(v.ColName, fmt.Sprintf("col:%d", i)))),
					ColTitleJustify: sIfNil(v.ColTitleJustify, "C"),

					col_no: -1,
				})
			}
		}
	}
	return
}

// http://www.papersizes.org/a-paper-sizes.htm
func GenReport(layout PdfReportLayout, data []map[string]interface{}, output_pdf string) error {
	pdf := gofpdf.New("P", "mm", layout.Paper, layout.FontDir)

	var wSum float64
	var w []float64
	pad := layout.PadWidth

	// Colored table
	fancyTable := func() {
		PdfSetIFillColor(pdf, layout.HdrFillColor)
		PdfSetITextColor(pdf, layout.HdrTextColor)
		PdfSetIDrawColor(pdf, layout.BorderColor)
		pdf.SetLineWidth(layout.BorderWidth)

		// Calculate width of printed columns, set col_no
		pdf.SetFont("", "B", layout.HdrFontSize) // B is for Bold
		for i, cn := range layout.ColsData {
			cn.col_no = com.FindColNo(cn.ColName, data[0])
			if cn.col_no < 0 {
				fmt.Fprintf(xOut, "Error(): Invalid or missing column name, will not be printed on report, column_name=%s\n", cn.ColName)
			} else {
				w = append(w, math.Ceil(pad*2.0+pdf.GetStringWidth(cn.ColTitle)))
			}
			layout.ColsData[i] = cn
		}

		// Calculate width of data - go through all the data.
		PdfSetIFillColor(pdf, layout.BodyFillColor)
		PdfSetITextColor(pdf, layout.BodyTextColor)
		pdf.SetFont("", "", layout.BodyFontSize)
		for _, v := range data {
			j := 0
			for _, cn := range layout.ColsData {
				if cn.col_no >= 0 {
					str := FormatItALL(cn.ColName, v[cn.ColName], cn.Format)
					// pdf.CellFormat(w[ii], 6, sss, "LR", 0, just, fill, 0, "")
					if cn.B_Width {
						if cn.Width < len(str) {
							str = str[0:cn.Width]
						}
					}
					aw := math.Ceil(pad*2.0 + pdf.GetStringWidth(str))
					if aw > w[j] {
						w[j] = aw
					}
					j++
				}
			}
		}

		pdf.SetFont("", "", layout.HdrFontSize)
		wSum = com.SumVector(w)

		// ------------------------------------------------------------------------------------------------------------
		// Do the header for the form
		// ------------------------------------------------------------------------------------------------------------
		PdfSetIFillColor(pdf, layout.HdrFillColor)
		PdfSetITextColor(pdf, layout.HdrTextColor)
		PdfSetIDrawColor(pdf, layout.BorderColor)
		pdf.SetLineWidth(layout.BorderWidth)
		pdf.SetFont("", "B", layout.HdrFontSize) // B is for Bold
		pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report
		ii := 0
		for _, cn := range layout.ColsData {
			if cn.col_no >= 0 {
				pdf.CellFormat(w[ii], 7, cn.ColTitle, "1", 0, cn.ColTitleJustify, true, 0, "")
				ii++
			}
		}
		pdf.Ln(-1)

		// Color and font restoration
		PdfSetIFillColor(pdf, layout.BodyFillColor)
		PdfSetITextColor(pdf, layout.BodyTextColor)
		pdf.SetFont("", "", layout.BodyFontSize)
		// 	Data
		fill := false
		for _, c := range data {
			pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report

			ii := 0
			for _, cn := range layout.ColsData {
				if cn.col_no >= 0 {
					just := cn.Justify
					if just == "C" {
					} else if just == "L" {
						just = ""
					} else if just == "R" {
					} else {
						just = ""
					}
					sss := FormatItALL(cn.ColName, c[cn.ColName], cn.Format)
					if cn.B_Width {
						if cn.Width < len(sss) {
							sss = sss[0:cn.Width]
						}
					}
					pdf.CellFormat(w[ii], 6, sss, "LR", 0, just, fill, 0, "")
					ii++
				}
			}

			pdf.Ln(-1)
			fill = !fill
		}
		pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report
		pdf.CellFormat(wSum, 0, "", "T", 0, "", false, 0, "")
	}
	pdf.SetTitle(layout.TitleStr, false)
	pdf.SetHeaderFunc(func() {

		if layout.HasTitle {
			// Arial bold 15
			pdf.SetFont("Arial", "B", 16)                 // xyzzy - set font
			wd := pdf.GetStringWidth(layout.TitleStr) + 6 // Calculate width of title and position
			pdf.SetX((layout.PaperWidth - wd) / 2)
			// Colors of frame, background and text
			PdfSetIDrawColor(pdf, PdfIColor{0, 80, 180})    // xyzzy
			PdfSetIFillColor(pdf, PdfIColor{255, 255, 255}) // xyzzy
			PdfSetITextColor(pdf, PdfIColor{0, 0, 0})       // xyzzy
			// pdf.SetLineWidth(1) 								// Thickness of frame (1 mm)
			// pdf.CellFormat(wd, 9, layout.TitleStr, "1", 1, "C", true, 0, "")
			pdf.CellFormat(wd, 9, layout.TitleStr, "", 0, "C", true, 0, "") // Title
			pdf.Ln(10)                                                      // Line break
		}

		if len(w) > 0 {
			PdfSetIFillColor(pdf, layout.HdrFillColor)
			PdfSetITextColor(pdf, layout.HdrTextColor)
			PdfSetIDrawColor(pdf, layout.BorderColor)
			pdf.SetLineWidth(layout.BorderWidth)
			pdf.SetFont("", "B", layout.HdrFontSize) // B is for Bold
			pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report
			ii := 0
			for _, cn := range layout.ColsData {
				if cn.col_no >= 0 {
					pdf.CellFormat(w[ii], 7, cn.ColTitle, "1", 0, cn.ColTitleJustify, true, 0, "")
					ii++
				}
			}
			pdf.Ln(-1)
			pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report *** this is the line problem *** xyzzy7
			pdf.CellFormat(wSum, 0, "", "T", 0, "", false, 0, "")
			PdfSetIFillColor(pdf, layout.BodyFillColor)
			PdfSetITextColor(pdf, layout.BodyTextColor)
			pdf.SetFont("", "", layout.BodyFontSize)
		}

	})
	if layout.PageNo {
		pdf.SetFooterFunc(func() {
			pdf.SetX((layout.PaperWidth - wSum) / 2) // Center Report
			pdf.CellFormat(wSum, 0, "", "T", 0, "", false, 0, "")

			pdf.SetY(-15)                                                                         // Position at 1.5 cm from bottom
			pdf.SetFont("Arial", "I", 8)                                                          // Arial italic 8 - xyzzy - set font
			PdfSetITextColor(pdf, PdfIColor{128, 128, 128})                                       // Text color in gray			// xyzzy
			pdf.CellFormat(0, 10, fmt.Sprintf("Page %d", pdf.PageNo()), "", 0, "C", false, 0, "") // Page number	// xyzzy template
		})
	}
	pdf.SetFont("Arial", "", 14) // Move up so can know size of strings -- xyzzy - set format
	pdf.AddPage()
	fancyTable()
	fo, err := docWriter(pdf, output_pdf)
	if err != nil {
		return err
	}
	pdf.OutputAndClose(fo)
	return nil
}

func docWriter(pdf *gofpdf.Fpdf, fn string) (*pdfWriter, error) {
	pw := new(pdfWriter)
	pw.pdf = pdf
	pw.name = fn
	if pdf.Ok() {
		var err error
		fileStr := fn
		pw.fl, err = os.Create(fileStr)
		if err != nil {
			pdf.SetErrorf("Error(): opening output file %s", fileStr)
			return nil, err
		}
	}
	return pw, nil
}

type pdfWriter struct {
	pdf  *gofpdf.Fpdf
	fl   *os.File
	name string
}

func (pw *pdfWriter) Write(p []byte) (n int, err error) {
	if pw.pdf.Ok() {
		return pw.fl.Write(p)
	}
	return
}

func (pw *pdfWriter) Close() (err error) {
	if pw.fl != nil {
		pw.fl.Close()
		pw.fl = nil
	}
	if pw.pdf.Ok() {
		fmt.Fprintf(xOut, "Successfully generated %s\n", pw.name)
	} else {
		fmt.Fprintf(xOut, "%s\n", pw.pdf.Error())
	}
	return
}

type PdfIColor struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

func PdfSetIFillColor(pdf *gofpdf.Fpdf, clr PdfIColor) {
	pdf.SetFillColor(clr.R, clr.G, clr.B)
}
func PdfSetITextColor(pdf *gofpdf.Fpdf, clr PdfIColor) {
	pdf.SetTextColor(clr.R, clr.G, clr.B)
}
func PdfSetIDrawColor(pdf *gofpdf.Fpdf, clr PdfIColor) {
	pdf.SetDrawColor(clr.R, clr.G, clr.B)
}

func clrIfNil(p *PdfIColor, d PdfIColor) PdfIColor {
	if p == nil {
		return d
	}
	return *p
}

func jIfNil3(p *int, a bool, b bool) bool {
	if p == nil {
		return a
	} else {
		return b
	}
}

// -----------------------------------------------------------------------------------------------------------------------------------------------------------------
func ReadColspec(fn string) {
	if !sizlib.Exists(fn) {
		fmt.Fprintf(xOut, "Error(12038): Missing file %s\n", fn)
	} else {
		s := sizlib.GetFile(fn)
		err := json.Unmarshal([]byte(s), &g_colspec)
		if err != nil {
			// fmt.Fprintf ( xOut, "Data ->%s<-\n", s )
			fmt.Fprintf(xOut, "Error(12037): Invalid format - %v\n", err)
		} else {
			processNewFormat()
		}
	}
}

func dataToFormattedText(data []map[string]interface{}, hdr bool) string {
	rv := ""
	s := ""
	com := ""
	cols := ""
	com = ""
	if len(data) <= 0 {
		return "-- no data --\n"
	}
	nam := make([]string, len(data[0])+1)
	width := make([]int, len(data[0])+1)
	for j := 0; j < len(data[0])+1; j++ {
		width[j] = 1
	}
	n_cols := 1
	nam[0] = "_row_"
	width[0] = len("_row_")
	for j, _ := range data[0] {
		nam[n_cols] = j
		if width[n_cols] < len(j) {
			width[n_cols] = len(j)
		}
		n_cols++
	}
	for i, v := range data {
		v["_row_"] = fmt.Sprintf("%d", i+1)
		for j := 0; j < n_cols; j++ {
			w := v[nam[j]]
			s = ""
			switch w.(type) {
			case string:
				s = w.(string)
			default:
				s = fmt.Sprintf("%v", w)
			}
			if width[j] < len(s) {
				width[j] = len(s)
			}
		}
	}

	for k, c := range g_colspec.ColsData {
		t := ms.FindCol(c.ColName, nam)
		if t < 0 {
			fmt.Fprintf(xOut, "Invalid column name ->%s<- in colspec, k=%d\n", c.ColName, k)
			return ""
		} else {
			g_colspec.ColsData[k].col_no = ms.FindCol(c.ColName, nam)
		}
	}

	for _, cc := range g_colspec.ColsData {
		j := cc.col_no
		w := cc.Width
		if width[j] < w {
			width[j] = w
		} // else if g_colspec.TruncateToWidth {
		//	width[j] = w
		// }
	}

	if g_colspec.headersOn {

		/*
			cols = ""
			for j, c := range nam {
				ff := fmt.Sprintf ( "%%-%ds", width[j] )
				cols += com + fmt.Sprintf ( ff, c )
				// g_colspec.ColSep		= sIfNil2 ( g_colspec.ColSep, " | " )
				com = g_colspec.ColSep
			}
			rv += cols + "\n"

			// g_colspec.TitleChars	= sIfNil2 ( g_colspec.TitleChars, "-+" )
			com = ""
			sepLine := "-"
			comLine := "-+-"
			if len(g_colspec.TitleChars) >= 2 {
				sepLine = g_colspec.TitleChars[0:1]
				comLine = g_colspec.TitleChars[0:1] + g_colspec.TitleChars[1:2] + g_colspec.TitleChars[0:1]
			}
			if g_colspec.titleLine {
				cols = ""
				for j, _ := range nam {
					cols += com + ms.PadStr ( width[j], sepLine, "" )
					com = "-+-"
				}
				rv += cols + "\n"
			}
		*/

		// -------------------------------------------------------------

		cols = ""
		for _, cc := range g_colspec.ColsData {
			j := cc.col_no
			c := cc.ColTitle
			switch cc.ColTitleJustify {
			default:
				fallthrough
			case "L":
				ff := fmt.Sprintf("%%-%ds", width[j])
				cols += com + fmt.Sprintf(ff, c)
			case "C":
				// fmt.Fprintf ( xOut, "width[%d] = %d for column ->%s<- centered ->%s<-\n", j, width[j], c, ms.CenterStr ( width[j], c ))
				cols += com + ms.CenterStr(width[j], c)
			case "R":
				cols += com + ms.PadStr(width[j], " ", c)
			}
			com = g_colspec.colSep
		}
		rv += cols + "\n"

		com = ""
		sepLine := "-"
		comLine := "-+-"
		if len(g_colspec.titleChars) >= 2 {
			sepLine = g_colspec.titleChars[0:1]
			comLine = g_colspec.titleChars[0:1] + g_colspec.titleChars[1:2] + g_colspec.titleChars[0:1]
		}
		if g_colspec.titleLine {
			cols = ""
			for _, cc := range g_colspec.ColsData {
				j := cc.col_no
				cols += com + ms.PadStr(width[j], sepLine, "")
				// com = "-+-"
				com = comLine
			}
			rv += cols + "\n"
		}

	}

	for i, v := range data {
		v["_row_"] = i + 1
		vals := ""
		com = ""
		for _, cc := range g_colspec.ColsData {
			j := cc.col_no
			s = FormatItALL(cc.ColName, v[nam[j]], cc.Format)
			// fmt.Printf ( "ColName = %s B_width = %v, Width=%v\n", cc.ColName, cc.B_Width, cc.Width )
			if cc.B_Width {
				if cc.Width < len(s) {
					s = s[0:cc.Width]
				}
			}
			// 's' is now the data as a string - next L/R/C justification for output
			switch cc.Justify {
			default:
				fallthrough
			case "L":
				ff := fmt.Sprintf("%%-%ds", width[j])
				vals += com + fmt.Sprintf(ff, s)
			case "C":
				// fmt.Fprintf ( xOut, "width[%d] = %d for column ->%s<- centered ->%s<-\n", j, width[j], c, ms.CenterStr ( width[j], c ))
				vals += com + ms.CenterStr(width[j], s)
			case "R":
				vals += com + ms.PadStr(width[j], " ", s)
			}
			com = g_colspec.colSep
		}
		rv = rv + vals + "\n"
	}
	return rv
}

// =======================================================================================================================================================================
// =======================================================================================================================================================================
// Look in fmt_table for "str:type:fmt", if not found then "str:fmt", if not - then error + return "s"
// Apply Formata

// Split format on '|', for each compoent of fmt pipe
// if 0 elements after split - then return 's', warning
// split on words (CSV/words)
//   1st word is: - search "str:type:W[0]", then "str:W[0]", then err, if found then call with params
// iterate over entire set of fmts
func FormatItALL(ColName string, s interface{}, Format string) (rv string) {
	ff := "str"
	switch s.(type) {
	case string:
		rv = s.(string)
		ff = "str"
	case int: // xyzzy - missing cases in switch - do we need special handeling for uint/uint8?
		ff = "int"
		rv = fmt.Sprintf("%v", s)
	case int64:
		ff = "int"
		rv = fmt.Sprintf("%v", s)
	case float64:
		ff = "flt" // xyzzy - should be "float64"
		rv = fmt.Sprintf("%v", s)
	case bool:
		ff = "bool"
		rv = fmt.Sprintf("%v", s)
	case time.Time:
		ff = "time.Time"
		rv = (s.(time.Time)).Format(ISO8601)
	default:
		ff = "str"
		rv = fmt.Sprintf("%v", s)
	}
	if db_fmt_1 {
		fmt.Fprintf(xOut, "\nColName=%s Format=%v, at line:%s\n", ColName, Format, tr.LINE())
	}
	fs := strings.Split(Format, "|")
	if len(fs) <= 0 {
		fmt.Fprintf(xOut, "Warning(12012): Format for %s contained 0 formatting elements.  This may be an error. format=%s\n", ColName, Format)
		return
	}

	if db_fmt_1 {
		fmt.Fprintf(xOut, "fs=%v, at line:%s\n", tr.SVar(fs), tr.LINE())
	}
	for _, v := range fs {
		pX := ms.SplitOnWords(v) // xyzzy - should use our word parser
		if len(pX) <= 0 {
			pX = append(pX, "")
		}
		rz := CallFmtFunction(ColName, ff, pX[0], Format, rv, pX)
		if db_fmt_1 {
			fmt.Fprintf(xOut, "pX=%v, v=[%s] In[%s] Out[%s] at line: %s\n", tr.SVar(pX), v, rv, rz, tr.LINE())
		}
		rv = rz
	}

	return
}

// ===================================================================================================================================================
// Column Formatting
// ===================================================================================================================================================
var fmtMapNS map[string]func(data int, rv interface{}) (t string)
var fmtMapSI map[string]func(data string, rv interface{}) (t string)
var fmtMapSS map[string]func(data string, rv string) (t string)
var fmtMapST map[string]func(data string, rv time.Time) (t string)

func init() {
	fmtMapNS = map[string]func(data int, rv interface{}) (t string){
		"Pad":      ms.PadOnRight,
		"PadLeft":  ms.PadOnLeft,
		"PadRight": ms.PadOnRight,
		"Center":   ms.CenterStr,
	}
	fmtMapSI = map[string]func(data string, rv interface{}) (t string){
		"Fmt":     ms.FmtPrintfStr,
		"PicFmt":  ms.PicFloat,
		"PicTime": ms.PicTime,
		"PicDate": ms.PicTime,
	}
	fmtMapSS = map[string]func(data string, rv string) (t string){
		"Nvl": ms.Nvl,
	}
	fmtMapST = map[string]func(data string, rv time.Time) (t string){
		"FTime": ms.StrFTime,
	}
	//		"FmtFloat":			FmtFloat,
	//		"FmtDate":			FmtDate,
}

func CallFmtFunction(ColName string, ty string, p0 string, Format string, rv interface{}, pX []string) (rx string) {
	if fx, ok := fmtMapNS[p0]; ok {
		k, err := strconv.ParseInt(pX[1], 10, 32)
		if err != nil {
		}
		rx = fx(int(k), rv)
	} else if fx, ok := fmtMapSI[p0]; ok {
		rx = fx(pX[1], rv)
	} else if fx, ok := fmtMapSS[p0]; ok {
		rx = fx(pX[1], rv.(string))
	} else if fx, ok := fmtMapST[p0]; ok {
		switch rv.(type) {
		case string:
			s, err := time.Parse(ISO8601, rv.(string))
			if err != nil {
				fmt.Fprintf(xOut, "Error(12039): Invalid time, got ->%s<- expecting a format of %s\n", rv.(string), ISO8601)
			} else {
				rx = fx(pX[1], s)
			}
		case time.Time:
			rx = fx(pX[1], rv.(time.Time))
		default:
			fmt.Fprintf(xOut, "Error(12038): Invalid type - expecting a string with a time in it or a time.Time type, got %T\n", rv)
		}
	} else {
		// fmt.Fprintf ( xOut, "Warning(12014): Unable to find a format [%s] for column [%s], format=->%s<-\n", p0, ColName, Format )
		rx = fmt.Sprintf("%v", rv)
	}
	return
}

// Look in fmt_table for "str:type:fmt", if not found then "str:fmt", if not - then error + return "s"
// Apply Formata

// Split format on '|', for each compoent of fmt pipe
// if 0 elements after split - then return 's', warning
// split on words (CSV/words)
//   1st word is: - search "str:type:W[0]", then "str:W[0]", then err, if found then call with params

// ===================================================================================================================================================
// Run Tempalte
// ===================================================================================================================================================
func RunTemplate(TemplateFn string, outFn string) {

	rtFuncMap := template.FuncMap{
		"Center":      ms.CenterStr,
		"PadR":        ms.PadOnRight,
		"PadL":        ms.PadOnLeft,
		"PicTime":     ms.PicTime,
		"FTime":       ms.StrFTime,
		"PicFloat":    ms.PicFloat,
		"nvl":         ms.Nvl,
		"FormatTable": Tmpl_DataToFormattedText,
		"PdfTable":    Tmpl_DataToPdfFormattedText, // xyzzy
		"Concat":      ms.Concat,
		"XCol":        XCol,
		"title":       strings.Title, // The name "title" is what the function will be called in the template text.
		"g":           global_g,
		"set":         global_set,
		"ifDef":       ms.IfDef,
		"ifIsDef":     ms.IfIsDef,
		"ifIsNotNull": ms.IfIsNotNull,
	}

	// open output file: a-log.log
	fo, err := os.Create(outFn)
	// close fo on exit and check for its returned error
	defer func() {
		if err := fo.Close(); err != nil {
			panic(err)
		}
	}()

	t, err := template.New("simple-tempalte").Funcs(rtFuncMap).ParseFiles(TemplateFn)
	if err != nil {
		fmt.Fprintf(xOut, "Error(12004): parsing/reading template, %s\n", err)
		return
	}

	err = t.ExecuteTemplate(fo, "report", g_data)
	if err != nil {
		fmt.Fprintf(xOut, "Error(12005): running template, %s\n", err)
		return
	}
}

// Basically wrong. xyzzy
func Tmpl_DataToPdfFormattedText(fn string, data []map[string]interface{}) string {
	if !sizlib.Exists(fn) {
		fmt.Fprintf(xOut, "Error(12040): Missing file %s\n", fn)
	} else {
		layout, err := ReadPdfCfg(fn)
		if err != nil {
			return fmt.Sprintf("Error(): PDF Report Failed to generate, err=%s\n", err)
		}
		GenReport(layout, data, "out.pdf")
		return "PDF Report Generated in out.pdf\n"
	}
	return "error occured\n"
}

func Tmpl_DataToFormattedText(fn string, data []map[string]interface{}) string {
	ReadColspec(fn)
	s := dataToFormattedText(data, true)
	return s
}

// Args are in triplet sets of
//		"L"|"R"|"C" -- Left, Right Center
//		#	Width
//		Value - type variable
// Example:
// 		{{XCol "L" 40 "--- Customer" "L" 35 "--- All Freight Charges"}}
func XCol(args ...interface{}) string {
	s := ""

Loop:
	for i := 0; i < len(args); i += 3 {
		// fmt.Fprintf ( xOut, "i=%d len(args)=%d\n", i, len(args) )
		if (i + 2) >= len(args) {
			fmt.Fprintf(xOut, "XCol: error - not a set of 3 in args, should be { \"L\" 22 .Name } for example\n")
			break Loop
		}
		Ju := ""
		switch args[i].(type) {
		case string:
			Ju = args[i].(string)
		default:
			fmt.Fprintf(xOut, "%s\n", `XCol: Invalid justification type, should be "L", "R", or "C"`)
			break Loop
		}
		Wt := 1
		switch args[i+1].(type) {
		case int:
			Wt = args[i+1].(int)
		case int64:
			Wt = int(args[i+1].(int64))
		default:
			fmt.Fprintf(xOut, "%s\n", `XCol: Invalid width, shoudl be a number`)
			break Loop
		}
		ss := ""
		switch args[i+2].(type) {
		case string:
			ss = args[i+2].(string)
		case int, int64, float32, float64:
			ss = fmt.Sprintf("%v", args[i+2].(string))
		case time.Time:
			ss = (args[i+2].(time.Time)).Format(ISO8601)
		default:
			if args[i+2] != nil {
				fmt.Fprintf(xOut, "XCol: don't know what to do with: %dth arg ->%T<-\n", i, args[i+2])
			}
		}
		switch Ju {
		default:
			fallthrough
		case "L":
			ff := fmt.Sprintf("%%-%ds", Wt)
			s += fmt.Sprintf(ff, ss)
		case "C":
			// fmt.Fprintf ( xOut, "width[%d] = %d for column ->%s<- centered ->%s<-\n", j, width[j], c, ms.CenterStr ( width[j], c ))
			s += ms.CenterStr(Wt, ss)
		case "R":
			s += ms.PadStr(Wt, " ", ss)
		}
	}
	return s
}

// =======================================================================================================================================================================
// Do ... Section
// =======================================================================================================================================================================

var xOutStk [](*os.File)
var xSpoolState string = ""

func DoSpool(cmd string, raw string, nth int, words []string) (rv string) {
	// plist := ParseLineIntoWordsNOQ(raw)
	plist := ParseLineIntoWords(raw)
	if len(plist) > 1 {
		if plist[1] == "off" {
			return DoEndFile(cmd, raw, -1, []string{"--eof--"})
		} else {
			return DoFile(cmd, raw, -1, []string{"--eof--"})
		}
	} else {
		fmt.Printf("Currently %s\n", xSpoolState)
	}
	return ""
}

func DoFile(cmd string, raw string, nth int, words []string) (rv string) {
	var err error
	rv = ""
	//plist := ParseLineIntoWordsNOQ(raw)
	plist := ParseLineIntoWords(raw)
	xSpoolState = "Spooling"
	if len(plist) > 1 {
		xOutStk = append(xOutStk, xOut)
		// fmt.Printf ( "doing a push, len=%d ((after))\n", len(xOutStk) )
		xOut, err = os.Create(plist[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error(): Unable to open %s for output, err=\n", plist[1], err)
			xOut = xOutStk[len(xOutStk)-1]
			xOutStk = xOutStk[0 : len(xOutStk)-1]
		}
	} else {
		if plist[0] == "\\o" {
			// xOut = os.Stdout				// xyzzy - probably wrong - should be a pop
			// fmt.Printf ( "doing a pop, len=%d, %s\n", len(xOutStk), tr.LINE() )
			if len(xOutStk) > 0 {
				xOut = xOutStk[len(xOutStk)-1]
				xOutStk = xOutStk[0 : len(xOutStk)-1]
			}
		} else {
			rv = "Error(): A file name is requried\n"
		}
	}
	return
}
func DoEndFile(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	// fmt.Printf ( "doing a pop, len=%d, %s\n", len(xOutStk), tr.LINE() )
	xSpoolState = "Off"
	if len(xOutStk) > 0 {
		xOut = xOutStk[len(xOutStk)-1]
		xOutStk = xOutStk[0 : len(xOutStk)-1]
	} else {
		fmt.Fprintf(os.Stderr, "Error(): Can not end-file when not sending output to a file\n")
	}
	return
}

// =======================================================================================================================================================================
func DoVersion(cmd string, raw string, nth int, words []string) (rv string) {
	return Version + "\n"
}

// =======================================================================================================================================================================
func DoEcho(cmd string, raw string, nth int, words []string) (rv string) {
	// plist := ParseLineIntoWordsNOQ(raw)
	plist := ParseLineIntoWords(raw)
	if len(plist) > 0 {
		sm := strings.Join(plist[1:], " ")
		// fmt.Printf ( "->DoEcho >%s< <-\n", raw )
		// fmt.Printf ( "->DoEcho >%s< <-\n", sm )
		return sm + "\n"
	}
	return ""
}

// =======================================================================================================================================================================
// Example:
//	rcli> save dat1 select * from ...
//  rcli> r-pdf dat1 bob.pdf
func DoPdf(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	plist := ParseLineIntoWords(raw)
	if InsurePlist(plist, 2) {
		var data []map[string]interface{}
		// RunTemplate ( plist[1], plist[2] )
		data = g_data[plist[1]].([]map[string]interface{})
		// GenReport( g_PdfSpec, data []map[string]interface{}, plist[1] )
		GenReport(g_PdfSpec, data, plist[2])
	}
	return
}

// =======================================================================================================================================================================
// example
// rcli> rt xyzzy output.txt
func DoTemplate(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	plist := ParseLineIntoWords(raw)
	if InsurePlist(plist, 2) {
		//s := ExecuteATemplate ( plist[1], g_data )
		//t := ExecuteATemplate ( plist[2], g_data )
		//RunTemplate ( s, t )
		RunTemplate(plist[1], plist[2])
	}
	return
}

//// =======================================================================================================================================================================
//// save .name = value
//// save .name = ""
//// merge . select ...
//// save user_id select * from "img_user"
//// print data
//
//func DoSave(cmd string, raw string, nth int, words []string) (rv string) {
//	rv = ""
//	plist := ParseLineIntoWords(raw)
//	if !InsurePlist(plist, 3) {
//		return
//	}
//	dest := plist[1]
//	switch plist[2] {
//	case "select":
//		// raw := strings.Join ( plist[2:], " " )			// xyzzy an error
//		re := regexp.MustCompile("^[ \t]*save[ \t]*[^ \t]*[ \t]")
//		sm := re.ReplaceAllLiteralString(raw, "")
//		// fmt.Printf ( "orig ->%s<- mod ->%s<-\n", raw, sm )
//		data := sizlib.SelData(db, sm)
//		if dest == "." {
//			// g_data = data[0]		// (done) - should be a merge
//			// func ExtendData ( a map[string]interface{}, b map[string]interface{} ) ( rv map[string]interface{} ) {
//			ld := len(data)
//			if ld <= 0 {
//				fmt.Fprintf(xOut, "Info(12010): Select returnd 0 rows\n")
//				rv = "no rows"
//			} else {
//				if ld > 1 {
//					fmt.Fprintf(xOut, "Warning(12011): Select returnd %d rows, only the 1st one was used.\n", ld)
//				}
//				g_data = ms.ExtendData(g_data, data[0])
//			}
//			for i, v := range g_data {
//				switch v.(type) {
//				case string:
//					// fmt.Fprintf ( xOut, "found a string, %s = %s\n", i, v.(string) )
//					g_data[i] = v.(string)
//				case bool:
//					// fmt.Fprintf ( xOut, "found a bool, %s = %v\n", i, v )
//					if v.(bool) {
//						g_data[i] = "y"
//					} else {
//						g_data[i] = "n"
//					}
//				case int:
//					// fmt.Fprintf ( xOut, "found a int, %s = %d\n", i, v.(int) )
//					g_data[i] = fmt.Sprintf("%d", v.(int))
//				case int64:
//					// fmt.Fprintf ( xOut, "found a int64, %s = %d\n", i, v.(int64) )
//					g_data[i] = fmt.Sprintf("%d", v.(int64))
//				case float64:
//					// fmt.Fprintf ( xOut, "found a float64, %s = %f\n", i, v.(float64) )
//					g_data[i] = fmt.Sprintf("%f", v.(float64))
//				case time.Time:
//					// fmt.Fprintf ( xOut, "found a time.Time, %s = %s\n", i, (v.(time.Time)).Format(ISO8601) )
//					g_data[i] = (v.(time.Time)).Format(ISO8601)
//				}
//			}
//		} else {
//			/*
//			   xyzzy1909
//			*/
//			// fmt.Printf ( "dest=[%s]\n", dest )
//			g_data[dest] = data // merge of data sets??? how???		- What about PKs - Missing rows? -- Much better to build a select with a JOIN in it.
//		}
//	case "=":
//		if len(plist) >= 4 {
//			g_data[dest] = plist[3]
//		} else {
//			g_data[dest] = ""
//		}
//	}
//	return ""
//}
//
//// =======================================================================================================================================================================
//func DoSave2(cmd string, raw string, nth int, words []string) (rv string) {
//	// fmt.Printf ( "DoSave2: raw=>%s<=\n", raw )
//	rv = ""
//	plist := ParseLineIntoWords(raw)
//	if !InsurePlist(plist, 3) {
//		return
//	}
//	dest := plist[1]
//	switch plist[2] {
//	case "select":
//		// raw := strings.Join ( plist[2:], " " )			// xyzzy an error
//		re := regexp.MustCompile("^[ \t]*save[ \t][ \t]*[^ \t][^ \t]*[ \t]")
//		sm := re.ReplaceAllLiteralString(raw, "")
//		// fmt.Printf ( "orig ->%s<- mod ->%s<-\n", raw, sm )
//		data := sizlib.SelData(db, sm)
//		if dest == "." {
//			// g_data = data[0]		// (done) - should be a merge
//			// func ExtendData ( a map[string]interface{}, b map[string]interface{} ) ( rv map[string]interface{} ) {
//			ld := len(data)
//			if ld <= 0 {
//				fmt.Fprintf(xOut, "Info(12010): Select returnd 0 rows\n")
//				rv = "no rows"
//			} else {
//				if ld > 1 {
//					fmt.Fprintf(xOut, "Warning(12011): Select returnd %d rows, only the 1st one was used.\n", ld)
//				}
//				*x_data = ms.ExtendData(*x_data, data[0])
//			}
//			for i, v := range *x_data {
//				switch v.(type) {
//				case string:
//					// fmt.Fprintf ( xOut, "found a string, %s = %s\n", i, v.(string) )
//					(*x_data)[i] = v.(string)
//				case bool:
//					// fmt.Fprintf ( xOut, "found a bool, %s = %v\n", i, v )
//					if v.(bool) {
//						(*x_data)[i] = "y"
//					} else {
//						(*x_data)[i] = "n"
//					}
//				case int:
//					// fmt.Fprintf ( xOut, "found a int, %s = %d\n", i, v.(int) )
//					(*x_data)[i] = fmt.Sprintf("%d", v.(int))
//				case int64:
//					// fmt.Fprintf ( xOut, "found a int64, %s = %d\n", i, v.(int64) )
//					(*x_data)[i] = fmt.Sprintf("%d", v.(int64))
//				case float64:
//					// fmt.Fprintf ( xOut, "found a float64, %s = %f\n", i, v.(float64) )
//					(*x_data)[i] = fmt.Sprintf("%f", v.(float64))
//				case time.Time:
//					// fmt.Fprintf ( xOut, "found a time.Time, %s = %s\n", i, (v.(time.Time)).Format(ISO8601) )
//					(*x_data)[i] = (v.(time.Time)).Format(ISO8601)
//				}
//			}
//		} else {
//			/*
//			   xyzzy1909
//			*/
//			// fmt.Printf ( "dest=[%s]\n", dest )
//			(*x_data)[dest] = data // merge of data sets??? how???		- What about PKs - Missing rows? -- Much better to build a select with a JOIN in it.
//		}
//	case "=":
//		if len(plist) >= 4 {
//			(*x_data)[dest] = plist[3]
//		} else {
//			(*x_data)[dest] = ""
//		}
//	}
//	return ""
//}

// =======================================================================================================================================================================
// Convert from HTML to PDF using wkhtmltopdf command.
// Example:
//		rcli> topdf file output
//
func DoToPdf(cmd string, raw string, nth int, words []string) (rv string) {

	plist := ParseLineIntoWords(raw)
	if !InsurePlist(plist, 3) {
		return
	}

	// fmt.Printf("topdf Running: ->%s<- %s\n", ToPdf, tr.LF())
	xcmd := exec.Command(ToPdf, "--page-size", "Letter", plist[1], plist[2])
	xcmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	xcmd.Stdout = &out
	err := xcmd.Run()
	// fmt.Printf("Just tried to run %s\n", ToPdf)
	// fmt.Printf("topdf At: %s\n", tr.LF())
	if err != nil {
		fmt.Printf("ToPdf: %s, Error:%v\n", err)
	}
	return out.String()
}

// =======================================================================================================================================================================
func DoConnect(cmd string, plist []string) (rv string) {
	rv = ""
	// Xyzzy -- to be done
	return
}

// =======================================================================================================================================================================
func DoComment(cmd string, plist []string) (rv string) {
	rv = ""
	return
}

// =======================================================================================================================================================================
func DoSendEmail(cmd string, raw string, nth int, words []string) (rv string) {

	// plist := ParseLineIntoWords ( raw )
	rv = ""
	if !email_connected {
		email = em.NewEmFile(*EmailCfgFN, true)
		email_connected = true
	}

	rv = ""
	if xfn, ok := g_data["email_to"]; !ok || xfn == "" {
		return
	}

	//_ = email.To("pschlump@gmail.com", "P.J.Schlump").
	//	Cc("rbrown@vanaire.net", "R BROWN").
	//	From("pschlump@gmail.com","Go Philip").
	//	Subject("Go Test 2/em2").
	//	TextBody("Hellow From Philip\n").
	//	HtmlBody("To <b>Boldly</b> Go...").
	//	Attach ( "em/dsc_0003-1155-frozen-blossoms-in-spring-apple.x46.jpg" ).
	//	Attach ( "em/dsc_0017-1607-tiny-red-spider-on-a-jonny-jump-up.73x46.jpg" ).
	//	Attach ( "em/dsc_0058-1927-wild-flowers-and-cactus.x46.jpg" ).
	//	SendIt()

	// line := ExecuteATemplate (*result, g_data)

	if debug_no_email := GlobalCfg["debug_no_email"]; debug_no_email == "yes" {
		fmt.Printf("Skipped sending email due to debug_no_email=='y' in global config file\n")
	} else {

		_ = email.To(ExecuteATemplate("{{.email_to}}", g_data), ExecuteATemplate("{{.email_to_name}}", g_data)).
			From(ExecuteATemplate("{{.email_from}}", g_data), ExecuteATemplate("{{.email_from_name}}", g_data)).
			Subject(ExecuteATemplate("{{.email_report_subject}}", g_data)).
			TextBody(ExecuteATemplate("{{.email_text_body}}", g_data)).
			HtmlBody(ExecuteATemplate("{{.email_html_body}}", g_data)).
			Attach(ExecuteATemplate(ExecuteATemplate("{{.email_attach_report}}", g_data), g_data)).
			SendIt()
	}

	// fmt.Fprintf ( xOut, "Attach FN = %s\n", ExecuteATemplate(ExecuteATemplate("{{.email_attach_report}}", g_data ), g_data))

	return "Email Sent\n"
}

// =======================================================================================================================================================================
func DoCopyFile(cmd string, raw string, nth int, words []string) (rv string) {
	plist := ParseLineIntoWords(raw)

	rv = "Insufficient parameters..."
	if !InsurePlist(plist, 2) {
		return
	}

	err := com.CopyFile(plist[1], plist[2])
	if err != nil {
		fmt.Fprintf(xOut, "Error(12041): Unable to copy file %s to %s\n", plist[1], plist[2])
		return
	}

	return
}

// =======================================================================================================================================================================
// func NewFTP(debuglevel int) *FTP {

func DoSendFtp(cmd string, raw string, nth int, words []string) (rv string) {
	var err error

	plist := ParseLineIntoWords(raw)

	rv = "Insufficient parameters..."
	if !InsurePlist(plist, 1) {
		return
	}

	rv = ""
	if xfn, ok := g_data["ftp_file_to_send"]; !ok || xfn.(string) == "" {
		return
	}

	ftpUsername := FTPConfig.Username
	if x, ok := g_data["ftp_username"]; ok {
		ftpUsername = x.(string)
	}

	if !ftp_connected {
		ftp_connected = true

		ftpClient = ftp4go.NewFTP(0) // 1 for debugging

		ftpServer := "127.0.0.1"
		if FTPConfig.Server != "" {
			ftpServer = FTPConfig.Server
		}
		if x, ok := g_data["ftp_server"]; ok {
			ftpServer = x.(string)
		}

		ftpPort := ftp4go.DefaultFtpPort
		if FTPConfig.Port != 0 {
			ftpPort = FTPConfig.Port
		}
		if x, ok := g_data["ftp_port"]; ok {
			ftpPort = x.(int)
		}

		ftpPassword := FTPConfig.Password
		if x, ok := g_data["ftp_password"]; ok {
			ftpPassword = x.(string)
		}

		//connect
		_, err = ftpClient.Connect(ftpServer, ftpPort, "")
		if err != nil {
			fmt.Fprintf(xOut, "Error(12030): The FTP connection failed Username=(%s).\n", ftpUsername)
			return
		}

		_, err = ftpClient.Login(ftpUsername, ftpPassword, "")
		if err != nil {
			fmt.Fprintf(xOut, "Error(12031): The ftp login failed Username=(%s).\n", ftpUsername)
			return
		}
	}

	ftpCwd := FTPConfig.DefaultCwd
	if x, ok := g_data["ftp_cwd"]; ok {
		ftpCwd = x.(string)
	}
	if ftpCwd != "" {
		// change to directory x
		_, err = ftpClient.Cwd(ftpCwd)
		if err != nil {
			fmt.Fprintf(xOut, "Error(12032): The Cwd(Change directory) command failed Username=(%s) cwd=(%s).\n", ftpUsername, ftpCwd)
			return
		}
	}

	fn := ExecuteATemplate("{{.ftp_file_to_send}}", g_data) // xyzzy - probably not correct - check it
	fn_to := sizlib.BasenameExt(fn)
	err = ftpClient.UploadFile(fn_to, fn, true, nil)
	if err != nil {
		fmt.Fprintf(xOut, "Error(12033): The UploadFile command failed. %s, Username=(%s) file=(%s)\n", err, FTPConfig.Username, fn)
		os.Exit(1)
	}
	if ftpCwd != "" {
		_, err = ftpClient.Cwd(com.PathToRelativeInverse(ftpCwd))
		if err != nil {
			fmt.Fprintf(xOut, "Error(12032): The Cwd(Change directory) command failed Username=(%s) cwd=(%s).\n", ftpUsername, ftpCwd)
			return
		}
	}

	return
}

// =======================================================================================================================================================================
//		var seqData SeqFmt
//		err = json.Unmarshal( []byte(s), &seqData )
//
//		cmd := "#"
//		for i, v := range seqData.Cmds {
//			cmd = v.Record[0]
//			if strings.HasPrefix ( cmd, "#" ) || strings.HasPrefix ( cmd, "//" ) || strings.HasPrefix ( cmd, "--" ) {
//				cmd = "#"
//			}

type SeqOne struct {
	Op     string
	Record []string
	Raw    string
	Cmds   []SeqOne
}

var debug_echo_1 bool = false
var g_line_no = 0

func DoLoop(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	return
}

// End loop actually runs the loop over the data.
// Data could be saved or select.  See test t12.sel (select) and t28.sel (save select, then iterate over it with substiution)
func EndLoop() (rv string) {
	var cmd string
	rv = ""
	g_data_save := g_data
	qry := strings.Join(seqData.Record[1:], " ")
	qry = strings.TrimRight(qry, " \t\f\n") // trim the ';' from end!
	qry = strings.TrimSuffix(qry, ";")
	qry = strings.TrimRight(qry, " \t\f\n")
	var data []map[string]interface{}
	// fmt.Printf ( "->%s<-\n", qry )
	if strings.HasPrefix(qry, "select") {
		// fmt.Printf ( "Running Query\n" )
		// xyzzy222 data = sizlib.SelData(db, qry)
	} else {
		// fmt.Printf ( "Checking Data\n" )
		if _, ok := g_data[qry]; ok { // xyzzy = dynamic type check that this is a data set!!!  See g_type
			// fmt.Printf ( "Pulling Data\n" )
			data = (g_data[qry]).([]map[string]interface{})
		} else {
			fmt.Printf("Not defined in global data: %s\n", qry)
		}
	}
	x_data_stk = append(x_data_stk, x_data)
	for i, v := range data {
		x_data = &v
		g_data["_seq_"] = fmt.Sprintf("%d", i)
		g_data = ms.ExtendData(g_data_save, v) // Combine data for just this row!!!!!!!!!!!
		/*
		   xyzzy --- === ---
		   	Need to save/extend [i] data in 'v' for the row of data we are looking at.
		   	Need to have a pointer to 'v' - of the appropriate type - that DoSave( xyzzy1909 ) saves data to instead of global g_data
		   	'v' above is a map[string]interface{} just like g_data

		*/
		// Folds the row back into the top level
		for j, w := range g_data {
			switch w.(type) {
			case string:
				// fmt.Fprintf ( xOut, "found a string, %s = %s\n", j, w.(string) )
				g_data[j] = w.(string)
			case bool:
				// fmt.Fprintf ( xOut, "found a bool, %s = %v\n", j, v )
				if w.(bool) {
					g_data[j] = "y"
				} else {
					g_data[j] = "n"
				}
			case int:
				// fmt.Fprintf ( xOut, "found a int, %s = %d\n", j, w.(int) )
				g_data[j] = fmt.Sprintf("%d", w.(int))
			case int64:
				// fmt.Fprintf ( xOut, "found a int64, %s = %d\n", j, w.(int64) )
				g_data[j] = fmt.Sprintf("%d", w.(int64))
			case float64:
				// fmt.Fprintf ( xOut, "found a float64, %s = %f\n", j, w.(float64) )
				g_data[j] = fmt.Sprintf("%f", w.(float64))
			case time.Time:
				// fmt.Fprintf ( xOut, "found a time.Time, %s = %s\n", j, (w.(time.Time)).Format(ISO8601) )
				g_data[j] = (w.(time.Time)).Format(ISO8601)
			}
		}
		for _, w := range seqData.Cmds {
			cmd = w.Record[0]
			if IfComment(len(cmd), cmd) {
				// It's a comment boys
			} else if IfQuit(len(cmd), cmd) { // check stack depth ( are we in a script? )
				fmt.Fprintf(xOut, "Error(): quit found in middle of loop or if -- ignored.\n")
			} else {
				// fmt.Printf ( "About to CallFunction cmd(%s) w.Record(%v) w.Raw(%s)\n", cmd, w.Record, w.Raw )
				CallFunction(cmd, w.Record, w.Raw)
			}
		}
		// added to test type/syntax
		data[i] = v
	}
	/*
	   xyzzy --- === --- -- Need to check for stack under-flow - this is a sign of end-loop w/o before!!!
	*/
	top_i := len(x_data_stk) - 1     // pos of top
	x_data = x_data_stk[top_i]       // extract top item
	x_data_stk = x_data_stk[0:top_i] // pop-stack
	return
}

func CallFunction(fx_name string, plistArr []string, raw string) {
	// fmt.Printf ( "In CallFunction[%s]\n", fx_name )
	if fx, ok := funcMap[fx_name]; ok {
		// line := strings.Join ( plistArr, " " )
		line2 := ExecuteATemplate(raw, g_data)
		// fmt.Printf ( "Before/after(2): ->%s<- ->%s<-\n", line, line2 )
		f := ParseLineIntoWords(line2)
		rv := fx.Fx(fx_name, line2, -1, f)
		fmt.Fprintf(xOut, "%s\n", rv)
	} else if pthFile, ok := ExistsPath(fx_name, g_path); ok { // See if file is in _path_
		RunFile(pthFile, "")
	} else {
		fmt.Fprintf(xOut, "Error(12008): Invalid function to call (%s)\n", fx_name)
	}
}

// =======================================================================================================================================================================
func DoUse(cmd string, raw string, nth int, words []string) (rv string) {
	// plist := ParseLineIntoWords(raw)
	// g_schema = plist[1]
	// return DoCRUD(cmd, raw)
	return ""
}

// =======================================================================================================================================================================
func DoHelp(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	plist := ParseLineIntoWords(raw)
	if InsurePlist(plist, 1) {
		// ,"sql-help":"/home/pschlump/lib/go-lib/sql-help"
		pth := GlobalCfg["sql-help"]
		if len(plist) <= 1 {
			ShowHelp(pth, "")
		} else {
			ShowHelp(pth, plist[1])
		}
	}
	return ""
}

// =============================================================================================================================================================
func DoSet(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	plist := ParseLineIntoWords(raw)
	if InsurePlist(plist, 2) {
		SetValue(plist[1], plist[2])
	}
	return ""
}

// =======================================================================================================================================================================
func DoGet(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	PrintValue(cmd)
	return
}

// =======================================================================================================================================================================
func DoPrint(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	plist := ParseLineIntoWords(raw)
	if InsurePlist(plist, 1) {
		// var ok bool
		switch plist[1] {
		// case "colspec":
		// 	rv = tr.SVarI( g_colspec )
		// fmt.Printf ( "%s=%s\n", plist[1], rv )
		case "data":
			rv = tr.SVarI(g_data) + "\n"
		default:
			rv_i, ok := g_data[plist[1]]
			if ok {
				fmt.Printf("%s=%v\n", plist[1], rv_i)
				rv = fmt.Sprintf("%v", rv_i)
			} else {
				fmt.Printf("%s= -- undefined --\n")
				rv = "--undefined--"
			}
		}
	}
	return
}

// =======================================================================================================================================================================
func DoIf(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	return
}

// =======================================================================================================================================================================
func DoEndLoop(cmd string, raw string, nth int, words []string) (rv string) {
	rv = ""
	return
}

// =======================================================================================================================================================================
func init() {
	g_data = make(map[string]interface{})
	g_data["__path__"] = "."
	g_data["__debug__"] = "off"
	g_data["__echo__"] = "off"
	x_data = &g_data

	//	"use":            DispatchFunc{Fx: DoUse},		// mod to use different Redis database
	//	"save":           DispatchFunc{Fx: DoSave2},

	// "set":            DispatchFunc{Fx: DoRedis},

	funcMap = map[string]DispatchFunc{
		"scan":           DispatchFunc{Fx: DoRScan},     //
		"hscan":          DispatchFunc{Fx: DoRScan},     //
		"sscan":          DispatchFunc{Fx: DoRScan},     //
		"zscan":          DispatchFunc{Fx: DoRScan},     //
		"x.del":          DispatchFunc{Fx: DoXDel},      //
		"x.get":          DispatchFunc{Fx: DoXGet},      //	Get keys with pattern, x.get PATTERN - should work with all key types
		"x.upd":          DispatchFunc{Fx: DoXUpd},      //	Get keys with pattern, x.get PATTERN - should work with all key types
		"g.set":          DispatchFunc{Fx: DoSet},       //	Set global in-memory data with value
		"g.get":          DispatchFunc{Fx: DoGet},       // get global in-memory data
		"colspec":        DispatchFunc{Fx: SetColspec},  //
		"print":          DispatchFunc{Fx: DoPrint},     //
		"topdf":          DispatchFunc{Fx: DoToPdf},     //
		"runTemplate":    DispatchFunc{Fx: DoTemplate},  //
		"rt":             DispatchFunc{Fx: DoTemplate},  //
		"send_email":     DispatchFunc{Fx: DoSendEmail}, //
		"send_ftp":       DispatchFunc{Fx: DoSendFtp},   //
		"send_cp":        DispatchFunc{Fx: DoCopyFile},  //
		"help":           DispatchFunc{Fx: DoHelp},      //
		"set-txt-format": DispatchFunc{Fx: SetColspec},  //
		"set-pdf-format": DispatchFunc{Fx: SetPdfspec},  //
		"r-pdf":          DispatchFunc{Fx: DoPdf},       //
		"echo":           DispatchFunc{Fx: DoEcho},      //
		"version":        DispatchFunc{Fx: DoVersion},   //
		"file":           DispatchFunc{Fx: DoFile},      //
		"spool":          DispatchFunc{Fx: DoSpool},     //
		"\\o":            DispatchFunc{Fx: DoFile},      //
		"end-file":       DispatchFunc{Fx: DoEndFile},   //
		"loop":           DispatchFunc{Fx: DoLoop},      //
		"if":             DispatchFunc{Fx: DoIf},        //
		"end-loop":       DispatchFunc{Fx: DoEndLoop},   //
	}
	// g.extend KEY				-- get hash key form Redis and set in-memory data with it
	// g.extend Name KEY		-- get form Redis and set in-memory "Name" with it -- can be value/ set/ array hash etc.  JSON data.
	// x.get PAT 				- get data
	// x.pick PAT getFunc 		- get data, then run getFunc to extract chunk from JSON
	//								x.pick srp:U:pschlump@uwyo.edu `rv = key["auth"]`
	//								x.pick srp:U:* `rva.push = key["auth"]`
	// x.update PAT updFunc		- get data, update it (JSON), save it
	//								x.update srp:U:pschlump@uwyo.edu `key["auth"] = "y"`
	//								x.update srp:U:* `key["auth"] = "y"`
	// 1. if "type" of key is ... { ... }
	// 2. {{ import "lib.js" }} as a part of JS template
	// 3. Default JS template
	//		var rv, rva = [], rvh = {}, key = {{.data}};
	//		{{.code}};
	//
	xOut = os.Stdout
}

func AddRedisCmds() {
	for _, vv := range g_cfg.RedisCmds {
		if _, ok := funcMap[vv]; !ok {
			// fmt.Printf("Createing [%s]\n", vv)
			funcMap[vv] = DispatchFunc{Fx: DoRedis}
		}
	}
}

// =======================================================================================================================================================================
type StateStk struct {
	stStk  []int
	tokStk []string
}

func NewStateStk() *StateStk {
	return &StateStk{make([]int, 0, 50), make([]string, 0, 50)}
}

func (this *StateStk) Push(st int, tok string) {
	this.stStk = append(this.stStk, st)
	this.tokStk = append(this.tokStk, tok)
}
func (this *StateStk) Pop() (st int, tok string) {
	st = -1
	tok = ""
	if len(this.stStk) > 0 {
		st = this.stStk[len(this.stStk)-1]
		tok = this.tokStk[len(this.tokStk)-1]
		this.stStk = this.stStk[0 : len(this.stStk)-1]
		this.tokStk = this.tokStk[0 : len(this.tokStk)-1]
	}
	return
}
func (this *StateStk) PeekSt(n int) (rv int) {
	rv = 0
	// fmt.Printf("Peek(%d) !!critical!! len(.tokStk)=%d n=%d, len(.stStk)=%d\n", n, len(this.tokStk), n, len(this.stStk))
	// for ii, yy := range this.stStk {
	// 	fmt.Printf("  %d stStk=%d tok=%s\n", ii, yy, this.tokStk[ii])
	// }
	// fmt.Printf("**** mid\n")
	if len(this.stStk) > n {
		rv = this.stStk[len(this.stStk)-n-1]
	}
	// fmt.Printf("**** at bottom\n")
	return
}
func (this *StateStk) PeekTok(n int) (t string) {
	t = ""
	if len(this.tokStk) > n {
		t = this.tokStk[len(this.tokStk)-n]
	}
	return
}
func (this *StateStk) Depth() (n int) {
	return len(this.stStk)
}

func TrimCmd(s string) (rv string) {
	rv = s
	rv = strings.TrimRight(rv, " \t\f\n") // trim the ';' from end!
	rv = strings.TrimSuffix(rv, ";")
	rv = strings.TrimRight(rv, " \t\f\n") // trim the ';' from end!
	return
}

// =======================================================================================================================================================================

// ------------------------------------------------------------------------------------------------------------------
// Globals for Templates (oooh Ick!)
//		{{g "name"}}  Access a global and return its value from an "interface" of string
//		{{set "name=Value"}} Set a value to constant Value
//		{{ bla | set "name"}} Set a value to Value of pipe
// ------------------------------------------------------------------------------------------------------------------
//var global_data	map[string]string
//func global_init () {
//	global_data = make(map[string]string)
//}

func global_g(b string) string {
	// fmt.Printf ( "XYZZY Inside 'g' -[%s]-\n", g_data[b].(string) )
	return g_data[b].(string)
}

func global_set(args ...string) string {
	if len(args) == 1 {
		b := args[0]
		var re = regexp.MustCompile("([a-zA-Z_][a-zA-Z_0-9]*)=(.*)")
		x := re.FindAllStringSubmatch(b, -1)
		if len(x) == 0 {
			name := x[0][1]
			value := ""
			g_data[name] = value
		} else {
			name := x[0][1]
			value := x[0][2]
			g_data[name] = value
		}
	} else if len(args) == 2 {
		name := args[0]
		value := args[1]
		g_data[name] = value
	} else {
		name := args[0]
		value := strings.Join(args[1:], "")
		g_data[name] = value
	}
	return ""
}

// ==============================================================================================================================================================
//var opts struct {
//	GlobalCfgFN string `short:"g" long:"globaCfgFile"    description:"Full path to global config" default:"global-cfg.json"`
//	EmailCfgFN  string `short:"e" long:"emailCfgFile"    description:"Path to email config" default:"email-config.json"` // Relative path to $HOME
//	FTPCfgFN    string `short:"f" long:"ftpCfgFile"      description:"Path to ftp config" default:"ftp-config.json"`     // Relative path to $HOME
//	InputFile   string `short:"i" long:"input"           description:"Input file name" default:"--stdin--"`
//	OutputFile  string `short:"o" long:"output"          description:"Ouptut file name" default:"--stdout--"`
//	CliCfg      string `short:"c" long:"config"          description:"JSON string for config of data" default:""`
//	Search      string `short:"S" long:"searchPath"      description:"SearchPath to use for config files" default:"C:\\cfg;."`
//	AppName     string `short:"A" long:"application"     description:"Application to run"                  default:"go-sql"`
// }

type CfgType struct {
	RedisHost   *string `json:"RedisHost"`
	RedisPort   *string `json:"RedisPort"`
	RedisAuth   *string `json:"RedisAuth"`
	EmailCfgFN  *string `json:"EmailCfgFN"`
	GlobalCfgFN string
	FTPCfgFN    string
	InputFile   string
	OutputFile  string
	CliCfg      string
	Search      string
	AppName     string
	RedisCmds   []string `json:"RedisCmds"`
}

// email = em.NewEmFile(*EmailCfgFN, true)
var RedisHost = flag.String("host", "127.0.0.1", "Redis connection info")            //
var RedisPort = flag.String("port", "6379", "Redis connection info")                 //
var RedisAuth = flag.String("auth", "", "Redis connection info")                     //
var Cfg = flag.String("cfg", "cfg.json", "configureation info")                      //
var EmailCfgFN = flag.String("email", "xyzzycfg.json", "Email SMTP connection info") //
func init() {
	flag.StringVar(RedisHost, "h", "127.0.0.1", "Redis connection info")           //
	flag.StringVar(RedisPort, "p", "6379", "Redis connection info")                //
	flag.StringVar(RedisAuth, "a", "", "Redis connection info")                    //
	flag.StringVar(Cfg, "c", "cfg.json", "configureation info")                    //
	flag.StringVar(EmailCfgFN, "e", "xyzzycfg.json", "Email SMTP connection info") //
}

var dbFlag map[string]bool
var client *redis.Client

var g_cfg CfgType

//func init() {
//	InitCfg(&g_cfg)
//}

func InitCfg(cfg *CfgType) {
	cfg.GlobalCfgFN = "global-cfg.json"
	cfg.FTPCfgFN = "ftp-config.json" // Relative path to $HOME
	cfg.InputFile = "--stdin--"
	cfg.OutputFile = "--stdout--"
	cfg.CliCfg = ""
	cfg.Search = "~/cfg:./cfg:."
	if string(os.PathSeparator) != "/" {
		cfg.Search = "C:\\cfg;."
	}
	cfg.RedisCmds = []string{
		"append",
		"asking",
		"auth",
		"bgrewriteaof",
		"bgsave",
		"bitcount",
		"bitop",
		"bitpos",
		"blpop",
		"brpop",
		"brpoplpush",
		"client",
		"cluster",
		"command",
		"config",
		"dbsize",
		"debug",
		"decr",
		"decrby",
		"del",
		"discard",
		"dump",
		"echo",
		"eval",
		"evalsha",
		"exec",
		"exists",
		"expire",
		"expireat",
		"flushall",
		"flushdb",
		"get",
		"getbit",
		"getrange",
		"getset",
		"hdel",
		"hexists",
		"hget",
		"hgetall",
		"hincrby",
		"hincrbyfloat",
		"hkeys",
		"hlen",
		"hmget",
		"hmset",
		"hscan",
		"hset",
		"hsetnx",
		"hvals",
		"incr",
		"incrby",
		"incrbyfloat",
		"info",
		"keys",
		"lastsave",
		"latency",
		"lindex",
		"linsert",
		"llen",
		"lpop",
		"lpush",
		"lpushx",
		"lrange",
		"lrem",
		"lset",
		"ltrim",
		"mget",
		"migrate",
		"monitor",
		"move",
		"mset",
		"msetnx",
		"multi",
		"object",
		"persist",
		"pexpire",
		"pexpireat",
		"pfadd",
		"pfcount",
		"pfdebug",
		"pfmerge",
		"pfselftest",
		"ping",
		"psetex",
		"psubscribe",
		"psync",
		"pttl",
		"publish",
		"pubsub",
		"punsubscribe",
		"randomkey",
		"readonly",
		"readwrite",
		"rename",
		"renamenx",
		"replconf",
		"restore",
		"restore-asking",
		"role",
		"rpop",
		"rpoplpush",
		"rpush",
		"rpushx",
		"sadd",
		"save",
		"scan",
		"scard",
		"script",
		"sdiff",
		"sdiffstore",
		"select",
		"set",
		"setbit",
		"setex",
		"setnx",
		"setrange",
		"shutdown",
		"sinter",
		"sinterstore",
		"sismember",
		"slaveof",
		"slowlog",
		"smembers",
		"smove",
		"sort",
		"spop",
		"srandmember",
		"srem",
		"sscan",
		"strlen",
		"subscribe",
		"substr",
		"sunion",
		"sunionstore",
		"sync",
		"time",
		"ttl",
		"type",
		"unsubscribe",
		"unwatch",
		"wait",
		"watch",
		"zadd",
		"zcard",
		"zcount",
		"zincrby",
		"zinterstore",
		"zlexcount",
		"zrange",
		"zrangebylex",
		"zrangebyscore",
		"zrank",
		"zrem",
		"zremrangebylex",
		"zremrangebyrank",
		"zremrangebyscore",
		"zrevrange",
		"zrevrangebylex",
		"zrevrangebyscore",
		"zrevrank",
		"zscan",
		"zscore",
		"zunionstore",
	}
	cfg.AppName = "redix-x-cli"
}

// ==============================================================================================================================================================
func main() {
	var err error
	st := 0
	var line_no int
	var g_if_cli bool = true
	var m_line_no int
	var multi string
	var cmd string
	var raw string
	var raw2 string

	stack := NewStateStk()
	SetPath(".;..")
	//	if string(os.PathSeparator) != "/" {
	//		ToPdf = "C:/Program Files/wkhtmltopdf/bin/wkhtmltopdf.exe"
	//
	//		// Example:
	//		// > wkhtmltopdf.exe file:///e:/tt.html e:/tt.pdf
	//	}

	// var err error
	dbFlag = make(map[string]bool)
	dbFlag["show-cfg"] = false

	flag.Parse()

	args := flag.Args()

	// Xyzzy - Usage/Help

	InitCfg(&g_cfg)
	if *Cfg != "" {
		g_cfg = ReadConfigFile(*Cfg)
		// fmt.Printf("Reading in [%s] = %s\n", *Cfg, lib.SVarI(g_cfg))
		if dbFlag["show-cfg"] {
			fmt.Printf("read in config file, data=%s\n", lib.SVarI(g_cfg))
		}
		if g_cfg.RedisHost != nil {
			RedisHost = g_cfg.RedisHost
		}
		if g_cfg.RedisPort != nil {
			RedisPort = g_cfg.RedisPort
		}
		if g_cfg.RedisAuth != nil {
			RedisAuth = g_cfg.RedisAuth
		}
		if g_cfg.EmailCfgFN != nil {
			EmailCfgFN = g_cfg.EmailCfgFN
		}
	}

	AddRedisCmds()
	if false {
		fmt.Printf("%s g_cfg.FTPCfgFN = [%s], %s%s\n", MiscLib.ColorRed, g_cfg.FTPCfgFN, tr.LF(), MiscLib.ColorReset)
	}

	client, _ = RedisClient()

	// ------------------ Find and open the configuration files, global-cfg.json and sql-cfg.json -------------------------------
	if globalCfgFN, ok := sizlib.SearchPathApp(g_cfg.GlobalCfgFN, g_cfg.AppName, g_cfg.Search); ok {
		fmt.Printf("\nglobal config: %s\n", globalCfgFN)
		GlobalCfg = com.ReadInGlobalConfig(globalCfgFN)
	} else {
		fmt.Printf("Error (14121): Fatal.  Unable to find the %s file using %s path.\n", g_cfg.GlobalCfgFN, g_cfg.Search)
		os.Exit(1)
	}

	if s, ok := GlobalCfg["ChdirTo"]; ok {
		os.Chdir(s)
	}

	// ==============================================================================================================================================================
	// xyzzy - needs work - Port for Windows
	// ==============================================================================================================================================================
	//if string(os.PathSeparator) == "/" {
	FTPConfig = com.ReadFTPConfig(com.AddHomeDir(g_cfg.FTPCfgFN))
	//} else {
	//	if opts.FTPCfgFN == "~/.ftp/ftp-config.json" {
	//		opts.FTPCfgFN = "ftp-config.json"
	//	}
	//	FTPConfig = com.ReadFTPConfig(com.AddHomeDir(opts.FTPCfgFN))
	//}
	rawRptCfgCli := g_cfg.CliCfg

	if g_cfg.OutputFile != "--stdout--" {
		xOut, err = os.Create(g_cfg.OutputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error(): Unable to open %s for output, err=\n", g_cfg.OutputFile, err)
			os.Exit(1)
		}
		defer xOut.Close()
	}

	if s, ok := GlobalCfg["ToPdf"]; ok {
		ToPdf = s
	}

	if s, ok := GlobalCfg["go-sql-search-path"]; ok {
		SetPath(s)
	}

	// var g_gofpdf_font = "./gofpdf/font" // xyzzy - should be pulled from global data
	// , "gofpdf_font": "./gofpdf/font"
	if s, ok := GlobalCfg["gofpdf_font"]; ok && s != "" {
		g_gofpdf_font = s
	}

	// xyzzy - windows error
	if string(os.PathSeparator) == "/" {
		IsTerm = terminal.IsTerminal(0)
		if IsTerm {
			TermWidth, TermHeight, err = terminal.GetSize(0)
			// fmt.Printf ( "Term H, W = %d %d\n", TermHeight, TermWidth )
			if err != nil {
				fmt.Printf("Error(00000): Unable to get the temrineal height/width, %s\n", err)
			}
		}
	}

	// tr.TraceSetConfig(0, false, false) // Turn SQL tracing off - Xyzzy -should have commands to turn this on

	if s, ok := GlobalCfg["path_to_wkhtmltopdf"]; ok {
		ToPdf = s
	}

	// ----------------------------------- Catch Signals -----------------------------------------

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		s := <-sigc
		// ... do something ...
		_ = s
		// sizlib.SetInterFlag()
		fmt.Printf("\nCaught: %v\n", s)
	}()

	runItSaveIt := func(hist, line string) {
		// fmt.Printf ( "runItSaveIt: hist->%s<- line->%s<- stk dpth=%d\n", hist, line, stack.Depth() )
		if stack.Depth() == 0 {
			myreadline.AddHistory(hist) //allow user to recall this line
			if g_if_cli {
				g_prompt = g_prompt0_x
			}
		}

		line = TrimCmd(line)
		// xyzzy72 - line = ExecuteATemplate (line, g_data)
		f := ParseLineIntoWords(line)
		cmd := f[0]

		if stack.Depth() == 0 {
			RunCmd(1, cmd, line)
		} else {
			SaveCmd(1, cmd, line)
		}
	}

	// ----------------------------------------------------------------------------------------------------
	// Main Input Loop - if -i specified - then read from file, else os.Stdin
	// ----------------------------------------------------------------------------------------------------

	for i := 0; i < 20; i++ {
		SetValue(fmt.Sprintf("__%d__", i), "")
	}
	for i, v := range args { // set Args into global data
		SetValue(fmt.Sprintf("__%d__", i), v)
	}
	SetValue("__now__", time.Now())
	for i, v := range GlobalCfg { // set rpt_* from GlobalCfg into global data
		if strings.HasPrefix(i, "rpt_") {
			SetValue(i, v)
		}
	}

	// xyzzy222 - fmt.Printf("__run_id__=%d\n", data[0]["x"]) // Do not remove this printout - /api/gen-report depends on it.

	if rawRptCfgCli != "" {
		rptCfgCli, err := sizlib.JSONStringToData(rawRptCfgCli)
		if err != nil {
			fmt.Printf("Error(14013): invalid -c option ->%s<- did not parse.", rawRptCfgCli)
		} else {
			for i, v := range rptCfgCli {
				// fmt.Printf ( "i=[%v] v=[%v]\n", i, v )
				SetValue(i, v)
			}
		}
	}

	if g_cfg.InputFile != "--stdin--" {

		g_if_cli = false

		if pthFile, ok := ExistsPath(g_cfg.InputFile, g_path); ok { // See if file is in _path_
			RunFile(pthFile, "")
		} else {
			fmt.Fprintf(xOut, "Error(12036): File not found %s\n", g_cfg.InputFile)
			os.Exit(1)
		}

	} else {

		g_if_cli := true

		//loop until ReadLine returns nil (signalling EOF)
		SetValue("__input_file__", "--stdin--")
		SetValue("__line_no__", "0")
		line_no = 0
		m_line_no = 0
		g_prompt = g_prompt0_x
	Loop:
		for {
			result := myreadline.ReadLine(&g_prompt)
			line_no++
			SetValue("__line_no__", fmt.Sprintf("%d", line_no))
			if result == nil {
				println()
				break Loop // exit loop when EOF(^D) is entered
			} else {
				raw = *result
				// xyzzy72 - line := ExecuteATemplate (*result, g_data)
				line := raw
				f := ParseLineIntoWords(line)
				if len(f) <= 0 {

				} else {

					// fmt.Printf ( "top: st=%2d, f=%v\n", st, f )

					cmd = f[0]
					switch st {
					case 0:
						if sizlib.InArray(strings.ToLower(f[0]), []string{"select", "update", "insert", "delete", "save", "create", "drop", "alter", "use"}) {
							raw2 = strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
							if strings.HasSuffix(raw2, ";") {
								runItSaveIt(raw, raw2)
							} else {
								multi = raw + "\n"
								if g_if_cli {
									g_prompt = g_prompt2_x
								}
								st = 1
							}
						} else if sizlib.InArray(f[0], []string{"loop"}) {
							multi = raw + "\n"
							if stack.Depth() == 0 {
								m_line_no = 1
								seqData = SeqOne{Op: "loop", Record: f, Raw: multi}
							} else {
								m_line_no++
							}
							raw2 = strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
							if strings.HasSuffix(raw2, ";") {
								if g_if_cli {
									g_prompt = fmt.Sprintf(g_prompt3_x, m_line_no)
								}
								st = 3
							} else {
								if g_if_cli {
									g_prompt = g_prompt2_x
								}
								st = 2
							}
							stack.Push(3, f[0])
						} else if sizlib.InArray(f[0], []string{"end-loop", "endloop"}) {
							if stack.Depth() == 0 {
								fmt.Fprintf(os.Stderr, "Error(): found %s when not inside a loop\n", f[0])
							} else {
								// exit the current script.
								st, _ = stack.Pop()
								if st == 0 && stack.Depth() == 0 {
									RunCmd(1, "drive", "...")
								}
							}
						} else if sizlib.InArray(f[0], []string{"if"}) {
						} else if sizlib.InArray(f[0], []string{"else"}) {
						} else if sizlib.InArray(f[0], []string{"elseif", "else-if", "elsif", "eif"}) {
						} else if sizlib.InArray(f[0], []string{"end-if", "endif", "eif", "fi"}) {
						} else if IfComment(len(f), f[0]) {
							// don't do much for comments for that is what makes comments comments.
						} else if IfQuit(len(f), f[0]) { // check stack depth ( are we in a script? )
							if stack.Depth() == 0 {
								break Loop
							} else {
								// exit the current script.
							}
						} else if _, ok := funcMap[f[0]]; ok { // See if a built-in function - you can not run scripts with  these names
							if stack.Depth() == 0 {
								RunCmd(1, cmd, raw)
							} else {
								SaveCmd(1, cmd, raw)
							}
						} else if _, ok := ExistsPath(f[0], g_path); ok { // See if file is in _path_
							if stack.Depth() == 0 {
								RunCmd(1, cmd, raw)
							} else {
								SaveCmd(1, cmd, raw)
							}
						} else { // I think that this is an oops...
							fmt.Fprintf(os.Stderr, "Error(): Invalid command, line:%d cmd:%s\n", line_no, f[0])
						}
					case 1: /* multi Line Statment, collecting statement */
						if f[0] == "/" {
							runItSaveIt(multi, multi)
							st = stack.PeekSt(0)
						} else if strings.HasSuffix(f[len(f)-1], ";") {
							multi += raw + "\n"
							runItSaveIt(multi, multi)
							st = stack.PeekSt(0)
						} else {
							multi += raw + "\n"
							if g_if_cli {
								g_prompt = g_prompt2_x
							}
						}
					case 2: /* multi Line LOOP Statment, collecting statement */
						if strings.HasSuffix(f[len(f)-1], ";") {
							multi += raw + "\n"
							m_line_no = 1
							f2 := ParseLineIntoWords(multi)
							seqData = SeqOne{Op: "loop", Record: f2, Raw: multi}
							// SaveCmd ( 1, "loop", multi )
							st = 3
							if g_if_cli {
								g_prompt = fmt.Sprintf(g_prompt3_x, m_line_no)
							}
						} else {
							multi += raw + "\n"
							if g_if_cli {
								g_prompt = g_prompt2_x
							}
						}
					case 3: /* loop...end-loop */
						m_line_no++
						if g_if_cli {
							g_prompt = fmt.Sprintf(g_prompt3_x, m_line_no)
						}
						if sizlib.InArray(strings.ToLower(f[0]), []string{"select", "update", "insert", "delete", "save", "create", "drop", "alter"}) {
							raw2 := strings.TrimRight(raw, " \t\f\n") // trim the ';' from end!
							if strings.HasSuffix(raw2, ";") {
								SaveCmd(1, cmd, raw)
							} else {
								multi = raw
								st = 1
							}
						} else if sizlib.InArray(f[0], []string{"loop"}) {
							multi = raw
							st = 3
							stack.Push(st, f[0])
							SaveCmd(1, cmd, raw)
						} else if sizlib.InArray(f[0], []string{"end-loop", "endloop"}) {
							// exit the current script.
							SaveCmd(1, cmd, raw)
							st, _ = stack.Pop()
							if stack.Depth() == 0 {
								// fmt.Printf ( "Probably should run command at this point- call func with data structure (drive), %s\n", tr.SVarI(seqData) )
								_ = EndLoop()
								if g_if_cli {
									g_prompt = g_prompt0_x
								}
								st = 0
							}
						} else if sizlib.InArray(f[0], []string{"if"}) {
						} else if sizlib.InArray(f[0], []string{"else"}) {
						} else if sizlib.InArray(f[0], []string{"elseif", "else-if", "elsif", "eif"}) {
						} else if sizlib.InArray(f[0], []string{"end-if", "endif", "eif", "fi"}) {
						} else if IfComment(len(f), f[0]) {
							// Much ado about noting, thus comments be comments.
						} else if IfQuit(len(f), f[0]) { // check stack depth ( are we in a script? )
							if stack.Depth() == 0 {
								break Loop
							} else {
								// exit the current script.
							}
						} else { // I think that this is an oops...
							SaveCmd(1, cmd, raw)
						}
					case 4: /* if...end-if */
					default:
					}

				}

			}
			// fmt.Printf ( "bot: st=%2d\n\n", st )
		}
	}
	if ftp_connected {
		ftpClient.Quit()
	}
}

//------------------------------------------------------------------------------------------------------------------------------
func RedisClient() (client *redis.Client, conFlag bool) {
	var err error
	client, err = redis.Dial("tcp", (*RedisHost)+":"+(*RedisPort))
	if err != nil {
		log.Fatal(err)
	}
	if (*RedisAuth) != "" {
		err = client.Cmd("AUTH", (*RedisAuth)).Err
		if err != nil {
			log.Fatal(err)
		} else {
			conFlag = true
		}
	} else {
		conFlag = true
	}
	return
}

func ReadConfigFile(fn string) (cfg CfgType) {
	InitCfg(&cfg)
	data, err := ioutil.ReadFile(fn)
	// fmt.Printf("fn [%s] body [%s]\n", fn, data)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		fmt.Printf("Syntax error in JSON file, %s\n", err)
		return
	}
	return
}

// scan - speical command
var retArr map[string]bool

func init() {
	retArr = make(map[string]bool)
	retArr["scan"] = true
	retArr["hscan"] = true
	retArr["sscan"] = true
	retArr["zscan"] = true
	retArr["command"] = true
}

func CmdReturnsArray(cmd string) bool {
	// lock not necessary -data- is static
	return retArr[cmd]
}

type ArrTreeType struct {
	Str string
	Arr []ArrTreeType
}

// scan MATCH pat COUNT n
func DoRScan(cmd string, raw string, nth int, words []string) (rv string) {
	ix := make([]interface{}, 0, len(words))
	ix = append(ix, 0)
	for _, vv := range words[1:] {
		ix = append(ix, vv)
	}
	rr := client.Cmd(cmd, ix...)
	err := rr.Err
	if err != nil {
		fmt.Printf("Error: %s, %s\n", err, tr.LF())
	} else {
		var e0 error
		var cursor int
		if rr.IsType(redis.Array) { // scan returns a 2 deep array/tree
			tt, err := rr.Array()
			if err != nil {
				fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
			} else {
				cursor, e0 = tt[0].Int() // this is the cursor
				fmt.Printf("Cursor = %d, %s\n", cursor, e0)
				t1, e1 := tt[1].List()
				fmt.Printf("List = %s, %s\n", t1, e1)
				rv = tr.SVar(t1)
			}
			// } else if rr.IsType(redis.Str) {
			//	rv, err = rr.Str()
			//	if err != nil {
			//		fmt.Printf("Error: %s, %s\n", err, tr.LF())
			//	}
			// -------------------- loop ------------------
			for ii := 0; cursor != 0; ii++ {
				ix[0] = cursor
				rr := client.Cmd(cmd, ix...)
				err := rr.Err
				if err != nil {
					fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
				} else {
					tt, err := rr.Array()
					if err != nil {
						fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
					} else {
						cursor, e0 = tt[0].Int() // this is the cursor
						fmt.Printf("[loop %d] Cursor = %d, %s\n", ii, cursor, e0)
						t1, e1 := tt[1].List()
						fmt.Printf("List = %s, %s\n", t1, e1)
					}
				}
			}
		} else {
			fmt.Printf("Error: didn't know what to do with type %x, cmd = %s\n", rr.GetType(), words)
		}
	}
	return
}

type ApplyAct func(ss []string, err error, data interface{})

// x.del pat
func DoXDel(cmd string, raw string, nth int, words []string) (rv string) {
	return DoApplyCmd(cmd, raw, nth, words, func(t1 []string, err error, _ interface{}) {
		if err != nil {
			return
		}
		for _, ww := range t1 {
			ex := client.Cmd("DEL", ww).Err
			if ex != nil {
				fmt.Printf("Delete Info: %s,  %s\n", err, tr.LF())
			}
		}
	}, nil)
}

// x.get pat
// DoXGet
func DoXGet(cmd string, raw string, nth int, words []string) (rv string) {
	type ListType struct {
		Elem []string
	}
	var AList ListType
	_ = DoApplyCmd(cmd, raw, nth, words, func(t1 []string, err error, ap interface{}) {
		if err != nil {
			fmt.Printf("error: %s\n", err)
			return
		}
		AListP, ok := ap.(*ListType)
		if !ok {
			fmt.Printf("error: invalid type\n")
			return
		}
		for _, ww := range t1 {
			// fmt.Printf("Doing get [%s]\n", ww)
			ss, e1 := client.Cmd("GET", ww).Str()
			if e1 != nil {
				// if "wrong type" -- TYPE key -- "set", do "set operation"
				if strings.HasPrefix(fmt.Sprintf("%s", e1), "WRONGTYPE") {
					ty, e1 := client.Cmd("TYPE", ww).Str()
					if e1 != nil {
						fmt.Printf("Unable to get type of %s, error=%s\n", ww, e1)
					} else {
						switch ty {
						case "set":
							ll, e1 := client.Cmd("SMEMBERS", ww).List()
							if e1 != nil {
								fmt.Printf("Unable to get set members of %s, error=%s\n", ww, e1)
							} else {
								AListP.Elem = append(AListP.Elem, fmt.Sprintf("set:%s", ll))
							}
						default:
							fmt.Printf("Key=%s Type=%s - unable to handle\n", ww, ty)
						}
					}
				} else {
					fmt.Printf("GET Error: %s\n", e1)
				}
			} else {
				AListP.Elem = append(AListP.Elem, ss)
			}
		}
	}, &AList)
	rv = tr.SVar(AList.Elem)
	return
}

// x.upd pat `js`
// DoXUpd
func DoXUpd(cmd string, raw string, nth int, words []string) (rv string) {
	type ListType struct {
		Elem []string
	}
	var AList ListType
	_ = DoApplyCmd(cmd, raw, nth, words, func(t1 []string, err error, ap interface{}) {
		if err != nil {
			fmt.Printf("error: %s\n", err)
			return
		}
		AListP, ok := ap.(*ListType)
		if !ok {
			fmt.Printf("error: invalid type\n")
			return
		}
		for _, ww := range t1 {
			// fmt.Printf("Doing get [%s]\n", ww)
			ty := "KEY"
			ss, e1 := client.Cmd("GET", ww).Str()
			if e1 != nil {
				// if "wrong type" -- TYPE key -- "set", do "set operation"
				if strings.HasPrefix(fmt.Sprintf("%s", e1), "WRONGTYPE") {
					ty, e1 := client.Cmd("TYPE", ww).Str()
					if e1 != nil {
						fmt.Printf("Unable to get type of %s, error=%s\n", ww, e1)
					} else {
						switch ty {
						case "set":
							ll, e1 := client.Cmd("SMEMBERS", ww).List()
							if e1 != nil {
								fmt.Printf("Unable to get set members of %s, error=%s\n", ww, e1)
							} else {
								AListP.Elem = append(AListP.Elem, fmt.Sprintf("set:%s", ll))
							}
						default:
							fmt.Printf("Key=%s Type=%s - unable to handle\n", ww, ty)
						}
					}
				} else {
					fmt.Printf("GET Error: %s\n", e1)
				}
			} else {
				o_ss := ss
				AListP.Elem = append(AListP.Elem, ss)

				data := make(map[string]interface{})
				err_flag := false

				// -----------------------------------------------------------------------------------------------------------
				// update 'ss'
				// (critical)
				// 	*0. allow for ` quoted ` code in words.
				// 	0. XyzzyUpd - Quoting of JSON code needs to be better understood - words -> quotes
				//					set abc "{\"x\":\"y\"}" did not work as expected
				//
				// (later - or {{import "file"}} in template)
				// 	0. xyzzyUpd - allow for using of stored JavaScript code chunks, or functions
				// 	0. xyzzyUpd - allow for loading of JavaScript code

				// -----------------------------------------------------------------------------------------------------------
				//  0. Parse 'ss' to verify it is correct JSON before template substitute!
				tmp := make(map[string]interface{})
				err := json.Unmarshal([]byte(ss), &tmp)
				if err != nil {
					fmt.Printf("Key=%s Type=%s - unable to set in x.upd, error=%s, will not update due to non-JSON data\n", ww, ty, err)
					err_flag = true
				}

				// -----------------------------------------------------------------------------------------------------------
				//  0. XyzzyUpd - read in upd_key template -- if read in template can have functions and code in it -- also {{import "file"}}
				upd_tmpl := `
{{define "upd_template"}}
// Update Template
var data = {{.data}};
{{.updcode}};
var rv = JSON.stringify(data);
{{end}}
`
				//  1. template substitute 'ss' -> upd_key template
				data["data"] = ss

				// data["updcode"] = `data["xxx111xxx"] = 12` // -dummy placeholder for moment
				data["updcode"] = words[2]
				// fmt.Printf("updcode = [%s]\n", words[2])

				if !err_flag {
					// -----------------------------------------------------------------------------------------------------------
					//  2. run it //		 func ExecuteATemplateByName(tmpl, tname string, data map[string]interface{}) string {
					code := ExecuteATemplateByName(upd_tmpl, "upd_template", data)
					vm := otto.New()
					_, err := vm.Run(code)
					if err != nil {
						fmt.Printf("Syntax Error in JavaScript update code: %s, code run =%s\n", err, code)
						err_flag = true
					}

					if !err_flag {
						// -----------------------------------------------------------------------------------------------------------
						//  3. get 'rv' from template
						//  4. -- already done in JS -- SVar(rv) -> string, set in ss
						if value, err := vm.Get("rv"); err == nil {
							if value_str, err := value.ToString(); err == nil {
								fmt.Printf("updated value = %v\n", value_str)
								ss = value_str
							} else {
								fmt.Printf("Error in getting results from update - err=%v\n", err)
							}
						}
					}

				}

				// -----------------------------------------------------------------------------------------------------------

				if !err_flag {
					if o_ss != ss {
						// set 'ss'
						e2 := client.Cmd("SET", ww, ss).Err
						if e2 != nil {
							fmt.Printf("Key=%s Type=%s - unable to set in x.upd, error=%s\n", ww, ty, err)
						}
					}
				}
			}
		}
	}, &AList)
	rv = tr.SVar(AList.Elem)
	return
}

// DoAplyCmd uses "scan" to walk the fApply function across a cursor
func DoApplyCmd(cmd string, raw string, nth int, words []string, fApply ApplyAct, fData interface{}) (rv string) {
	rv = "Err"
	// xyzzy - clean this up
	if cmd == "x.upd" {
		if len(words) != 3 {
			fmt.Printf("Usage: x.upd pattern, you supplied ->%s<- ->%s<-\n", raw, tr.SVar(words))
			return
		}
	} else {
		if len(words) != 2 {
			fmt.Printf("Usage: x.del pattern, you supplied ->%s<-\n", raw)
			return
		}
	}
	ix := make([]interface{}, 0, 4)
	ix = append(ix, 0)
	ix = append(ix, "MATCH")
	ix = append(ix, words[1])
	cmd = "scan"
	rr := client.Cmd(cmd, ix...)
	err := rr.Err
	if err != nil {
		fmt.Printf("Error: %s, %s\n", err, tr.LF())
	} else {
		rv = "OK"
		var e0 error
		var cursor int
		if rr.IsType(redis.Array) { // scan returns a 2 deep array/tree
			tt, err := rr.Array()
			if err != nil {
				fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
			} else {
				cursor, e0 = tt[0].Int() // this is the cursor
				// fmt.Printf("Cursor = %d, %s\n", cursor, e0)
				if e0 == nil {
					t1, e1 := tt[1].List()
					// fmt.Printf("List = %s, %s\n", t1, e1)
					fApply(t1, e1, fData)
				}
				// xyzzy - report error, e0
			}
			// -------------------- loop ------------------
			for ii := 0; cursor != 0; ii++ {
				ix[0] = cursor
				rr := client.Cmd(cmd, ix...)
				err := rr.Err
				if err != nil {
					fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
				} else {
					tt, err := rr.Array()
					if err != nil {
						fmt.Printf("Error: %s, type=%x (%s), %s\n", err, rr.GetTypeUint(), rr.GetType(), tr.LF())
					} else {
						cursor, e0 = tt[0].Int() // this is the cursor
						// fmt.Printf("[loop %d] Cursor = %d, %s\n", ii, cursor, e0)
						if e0 == nil {
							t1, e1 := tt[1].List()
							fApply(t1, e1, fData)
						}
						// xyzzy - report error, e0
					}
				}
			}
		} else {
			fmt.Printf("Error: didn't know what to do with type %x, cmd = %s\n", rr.GetType(), words)
		}
	}
	return
}

//------------------------------------------------------------------------------------------------------------------------------
//------------------------------------------------------------------------------------------------------------------------------
// look at Cmd and figure out
// 1. Params are array of interface{}, if so create and
// 2. How the "help" info in Redis works to extract # of params
// 3. Return values - it is a data type -- If a List then JSON ify it to a string
//------------------------------------------------------------------------------------------------------------------------------
/*
// Cmd calls the given Redis command.
func (c *Client) Cmd(cmd string, args ...interface{}) *Resp {
	err := c.writeRequest(request{cmd, args})
	if err != nil {
		return newRespIOErr(err)
	}
	return c.ReadResp()
}
*/
//------------------------------------------------------------------------------------------------------------------------------
func DoRedis(cmd string, raw string, nth int, words []string) (rv string) {
	// fmt.Printf("in DoRedis, %s\n", words)
	ix := make([]interface{}, 0, len(words))
	for _, vv := range words[1:] {
		ix = append(ix, vv)
	}
	rr := client.Cmd(words[0], ix)
	err := rr.Err
	if err != nil {
		fmt.Printf("Error: %s, %s\n", err, tr.LF())
	} else {
		if rr.IsType(redis.Array) {
			tt, err := rr.List()
			if err != nil {
				fmt.Printf("Error: %s, %s\n", err, tr.LF())
			} else {
				rv = tr.SVar(tt)
			}
		} else if rr.IsType(redis.Str) {
			rv, err = rr.Str()
			if err != nil {
				fmt.Printf("Error: %s, %s\n", err, tr.LF())
			}
		} else {
			fmt.Printf("Error: didn't know what to do with type %x, cmd = %s\n", rr.GetType(), words)
		}
	}
	return
}

func hold_import_otto() {
	vm := otto.New()
	_ = vm
}

// I bid you adieu!

/* vim: set noai ts=4 sw=4: */
