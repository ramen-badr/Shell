package main

import (
	"bufio"
	"container/list"
	"fmt"
	"os"
	"os/signal"
	"shell/internal/jobs"
	"shell/internal/parser"
	"shell/internal/prompt"
	"syscall"
)

func main() {
	s := bufio.NewScanner(os.Stdin)

	fgPid := make(chan int, 1)
	signChan := make(chan os.Signal)
	signal.Notify(signChan, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTSTP)

	jm := jobs.JobManager{
		Jobs: list.New(),
	}

	go jm.SignalHandler(fgPid)

	var tmpPipe, readPipe, writePipe *os.File
	var cmdPipe []string
	var groupPid int

	for {
		prompt.Out()

		line := parser.Read(s)
		if line == nil {
			fmt.Println("\nexit")
			return
		}

		commands := parser.Parse(line)
		if len(commands) == 0 {
			jm.WriteDoneJobs()
		}

		for i := 0; i < len(commands); i++ {
			commands[i].ForkAndExec(&jm, &cmdPipe, &groupPid, &readPipe, &tmpPipe, &writePipe, fgPid)
		}
	}
}
