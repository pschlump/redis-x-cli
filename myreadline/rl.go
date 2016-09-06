// Compatability package for readline on Linux, with dummy package on Windows

// +build linux darwin dragonfly freebsd netbsd openbsd

package myreadline

import (
	"www.2c-why.com/readline" // "github.com/shavac/readline"
)

func AddHistory(hist string) {
	readline.AddHistory(hist)
}

func ReadLine(prompt *string) *string {
	return readline.ReadLine(prompt)
}
