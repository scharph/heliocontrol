package protocol

import (
	"regexp"
	"strings"
)

// readCommands matches every command that only reads from the controller.
// Anything else (setup point writes, overrides, trend control, clock set, time
// program writes, ...) changes controller state.
var readCommands = []*regexp.Regexp{
	regexp.MustCompile(`^~(LIN|LOUT|RID|RM|RS|CLK|TRS|PRL);$`),
	regexp.MustCompile(`^~(MP|SP),NR=\d+;$`),
	regexp.MustCompile(`^~(MR|TR)(,\d+)+;$`),
	regexp.MustCompile(`^~(PRI|PRD)\d+;$`),
	regexp.MustCompile(`^~PRE,PR=\d+,DAY=\d+,EV=\d+;$`),
}

// IsReadCommand reports whether cmd is a pure read. Commands are normalized
// with a trailing ';' first, as the transport adds it when missing.
func IsReadCommand(cmd string) bool {
	if !strings.HasSuffix(cmd, ";") {
		cmd += ";"
	}
	for _, re := range readCommands {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}
