// Compatability package for readline on Linux, with dummy package on Windows
// This is the windows dummy code.

// +build windows

package myreadline

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

/*

This is a dummy package that works on Windows 7, 8, 8.1, 8.2 - it is not intended
as a replacement for readline.  It is inteded as a dummy that maintains the
go readline interface - and just reads from the terminal one line at a time.

*/

func AddHistory(hist string) {
}

var reader *bufio.Reader

func init() {
	reader = bufio.NewReader(os.Stdin)
}

// result := readline.ReadLine(&g_prompt)
func ReadLine(prompt *string) *string {
	fmt.Printf("%s", *prompt)
	text, _ := reader.ReadString('\n')
	// fmt.Println(text)
	s := string(text)
	s = strings.TrimRight(s, "\r\n")
	return &s
}
