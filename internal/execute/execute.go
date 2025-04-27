package exec

import (
	"fmt"
	"os"
	"os/exec"
	"shell/internal/jobs"
	"strconv"
	"syscall"
)

type Command struct {
	CmdArgs                  []string
	InFile, OutFile, AppFile string
	Background               bool
	CmdFlag                  byte
}

func (cmd *Command) ForkAndExec(jm *jobs.JobManager, cmdPipe *[]string, groupPid *int, readPipe **os.File, tmpPipe **os.File, writePipe **os.File, fgPid chan int) {
	switch cmd.CmdArgs[0] {
	case "jobs":
		if len(cmd.CmdArgs) == 1 {
			for i := 1; i <= jm.IdLastJob; i++ {
				for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
					if elem.Value.(jobs.Job).Id == i {
						jm.Write(elem.Value.(jobs.Job).Pid)
						break
					}
				}
			}
		} else {
			for i := 1; i < len(cmd.CmdArgs); i++ {
				var flag bool
				for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
					if strconv.Itoa(elem.Value.(jobs.Job).Id) == cmd.CmdArgs[i] {
						jm.Write(elem.Value.(jobs.Job).Pid)
						flag = true
						break
					}
				}
				if !flag {
					fmt.Println("jobs: No such job:", cmd.CmdArgs[i])
				}
			}
		}
		return
	case "cd":
		if len(cmd.CmdArgs) == 2 {
			if err := os.Chdir(cmd.CmdArgs[1]); err != nil {
				fmt.Println("cd: No such file or directory:", cmd.CmdArgs[1])
			}
		} else if len(cmd.CmdArgs) != 1 {
			fmt.Println("cd: Too many arguments")
		}
		jm.WriteDoneJobs()
		return
	case "fg":
		var pid int
		if len(cmd.CmdArgs) == 1 {
			switch {
			case jm.Jobs.Back() == nil:
				fmt.Println("fg: No such job: current")
			case jm.Jobs.Back().Value.(jobs.Job).Status == "Done":
				fmt.Println("fg: Job has terminated")
			default:
				pid = jm.Jobs.Back().Value.(jobs.Job).Pid
				jm.Fg(pid)
			}
		} else {
			var flag bool
			for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
				if strconv.Itoa(elem.Value.(jobs.Job).Id) == cmd.CmdArgs[1] {
					flag = true
					if elem.Value.(jobs.Job).Status == "Done" {
						fmt.Println("fg: Job has terminated")
					} else {
						pid = elem.Value.(jobs.Job).Pid
						jm.Fg(pid)
					}
					break
				}
			}
			if !flag {
				fmt.Println("fg: No such job:", cmd.CmdArgs[1])
			}
		}
		jm.WriteDoneJobs()
		return
	case "bg":
		var pid int
		if len(cmd.CmdArgs) == 1 {
			switch {
			case jm.Jobs.Back() == nil:
				fmt.Println("bg: No such job: current")
			case jm.Jobs.Back().Value.(jobs.Job).Status == "Done":
				fmt.Println("bg: Job has terminated")
			case jm.Jobs.Back().Value.(jobs.Job).Background:
				fmt.Println("bg: Job", jm.Jobs.Back().Value.(jobs.Job).Id, "already in background")
			default:
				pid = jm.Jobs.Back().Value.(jobs.Job).Pid
				jm.Bg(pid)
			}
		} else {
			for i := 1; i < len(cmd.CmdArgs); i++ {
				var flag bool
				for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
					if strconv.Itoa(elem.Value.(jobs.Job).Id) == cmd.CmdArgs[i] {
						flag = true
						switch {
						case elem.Value.(jobs.Job).Status == "Done":
							fmt.Println("bg: Job has terminated")
						case elem.Value.(jobs.Job).Background:
							fmt.Println("bg: Job", cmd.CmdArgs[i], "already in background")
						default:
							pid = elem.Value.(jobs.Job).Pid
							jm.Bg(pid)
						}
						break
					}
				}
				if !flag {
					fmt.Println("bg: No such job:", cmd.CmdArgs[i])
				}
			}
		}
		jm.WriteDoneJobs()
		return
	}

	binary, err := exec.LookPath(cmd.CmdArgs[0])
	if err != nil {
		fmt.Println("Command not found:", cmd.CmdArgs[0])
		jm.WriteDoneJobs()
		return
	}

	if cmd.CmdFlag == 2 || cmd.CmdFlag == 3 {
		*tmpPipe, *writePipe, err = os.Pipe()
		if err != nil {
			fmt.Println("Error creating pipe")
		}
		*cmdPipe = append(*cmdPipe, cmd.CmdArgs...)
		*cmdPipe = append(*cmdPipe, "|")
	}

	stdin := os.Stdin
	switch {
	case cmd.InFile != "":
		stdin, err = os.Open(cmd.InFile)
		defer func() { _ = stdin.Close() }()
		if err != nil {
			fmt.Println("No such file or directory:", cmd.InFile)
			jm.WriteDoneJobs()
			return
		}
	case cmd.CmdFlag == 1 || cmd.CmdFlag == 3:
		stdin = *readPipe
		defer func() { _ = stdin.Close() }()
	}

	stdout := os.Stdout
	switch {
	case cmd.OutFile != "":
		stdout, err = os.Create(cmd.OutFile)
		defer func() { _ = stdout.Close() }()
		if err != nil {
			fmt.Println("Error opening output file")
			return
		}
	case cmd.AppFile != "":
		stdout, err = os.OpenFile(cmd.AppFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer func() { _ = stdout.Close() }()
		if err != nil {
			fmt.Println("Error opening append file")
			return
		}
	case cmd.CmdFlag == 2 || cmd.CmdFlag == 3:
		stdout = *writePipe
		defer func() { _ = stdout.Close() }()
	}

	pid, err := syscall.ForkExec(binary, cmd.CmdArgs, &syscall.ProcAttr{
		Dir:   "",
		Files: []uintptr{stdin.Fd(), stdout.Fd(), os.Stderr.Fd()},
		Sys: &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    *groupPid,
		},
	})
	if err != nil {
		fmt.Println("Error during ForkExec")
		return
	}

	if cmd.CmdFlag != 0 && *groupPid == 0 {
		*groupPid = pid
	}

	switch cmd.CmdFlag {
	case 0:
		jm.Add(pid, cmd.CmdArgs, cmd.Background, false)
		if cmd.Background {
			go jm.WaitForBackground(pid)
		} else {
			jm.WaitForForeground(pid, fgPid)
		}
		jm.WriteDoneJobs()
	case 1:
		*cmdPipe = append(*cmdPipe, cmd.CmdArgs...)

		jm.Add(*groupPid, *cmdPipe, cmd.Background, true)
		if cmd.Background {
			go jm.WaitForBackground(*groupPid)
		} else {
			jm.WaitForForeground(*groupPid, fgPid)
		}

		*groupPid = 0
		*cmdPipe = nil
		jm.WriteDoneJobs()
	default:
		*readPipe = *tmpPipe
	}
}
