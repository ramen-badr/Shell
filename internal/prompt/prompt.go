package prompt

import (
	"fmt"
	"os"
	"os/user"
	"strings"
)

func Out() {
	userName, hostName, cwd := "username", "hostname", "~"
	homeDir, ok := os.LookupEnv("HOME")

	if curUser, err := user.Current(); err == nil {
		userName = curUser.Username
	}

	if curHostName, err := os.Hostname(); err == nil {
		hostName = curHostName
	}

	if curCwd, err := os.Getwd(); err == nil && ok && strings.HasPrefix(curCwd, homeDir) {
		cwd = strings.Replace(curCwd, homeDir, "~", 1)
	}

	fmt.Print(userName + "@" + hostName + ":" + cwd + "$ ")
}
