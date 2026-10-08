package main

// keyBytes are what a terminal sends for each named key.
var keyBytes = map[string]string{
	"enter": "\r", "esc": "\x1b", "tab": "\t", "backspace": "\x7f", "space": " ",
	"ctrl-c": "\x03", "ctrl-d": "\x04", "ctrl-j": "\n", "ctrl-o": "\x0f", "ctrl-r": "\x12", "ctrl-u": "\x15",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
	"shift-tab": "\x1b[Z", "f1": "\x1bOP", "f2": "\x1bOQ",
	"1": "1", "2": "2", "3": "3", "4": "4", "a": "a", "y": "y", "n": "n",
}
