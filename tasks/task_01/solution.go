package main

import (
	"strings"
)

func greet(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Hello, World!"
	} else {
		return "Hello, " + strings.TrimSpace(name) + "!"
	}
}
