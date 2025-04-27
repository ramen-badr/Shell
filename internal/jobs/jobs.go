package jobs

import (
	"container/list"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"shell/internal/prompt"
	"strings"
	"sync"
	"syscall"
)

type Job struct {
	Pid        int
	Status     string
	CmdArgs    []string
	Background bool
	Id         int
	PipeFlag   bool
}

type JobManager struct {
	Jobs      *list.List
	IdLastJob int
	jobsMutex sync.Mutex
}

func (jm *JobManager) SignalHandler(fgPid chan int) {
	signChan := make(chan os.Signal)
	signal.Notify(signChan, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTSTP)

	for sig := range signChan {
		switch sig {
		case syscall.SIGINT:
			fmt.Println()
			select {
			case pid := <-fgPid:
				_ = syscall.Kill(jm.PgId(pid), syscall.SIGINT)
				jm.Update(pid, "Done")
				fgPid <- 0
			default:
				prompt.Out()
			}
		case syscall.SIGTSTP:
			fmt.Println()
			select {
			case pid := <-fgPid:
				if err := syscall.Kill(jm.PgId(pid), syscall.SIGSTOP); err != nil {
					fmt.Println("Error stopping process")
				}
				jm.Update(pid, "Stopped")
				jm.Write(pid)
				fgPid <- 0
			default:
				prompt.Out()
			}
		}
	}
}

func (jm *JobManager) WaitForForeground(pid int, fgPid chan int) {
	fgPid <- pid
	defer func() { <-fgPid }()

	var ws syscall.WaitStatus

	for {
		_, err := syscall.Wait4(jm.PgId(pid), &ws, syscall.WUNTRACED, nil)

		switch {
		case ws.Stopped() || (ws.Signaled() && err != nil):
			return
		case err != nil:
			jm.Update(pid, "Done")
			return
		}
	}
}

func (jm *JobManager) WaitForBackground(pid int) {
	var ws syscall.WaitStatus

	for {
		_, err := syscall.Wait4(jm.PgId(pid), &ws, syscall.WUNTRACED, nil)

		switch {
		case ws.Stopped() || (ws.Signaled() && jm.PgId(pid) < 0):
			return
		case ws.Signaled() && jm.PgId(pid) >= 0:
			fmt.Println()
			return
		case err != nil:
			jm.Update(pid, "Done")
			return
		}
	}
}

func (jm *JobManager) WriteDoneJobs() {
	for i := 1; i <= jm.IdLastJob; i++ {
		for e := jm.Jobs.Front(); e != nil; e = e.Next() {
			if e.Value.(Job).Id == i {
				if e.Value.(Job).Status == "Done" {
					jm.Write(e.Value.(Job).Pid)
				}
				break
			}
		}
	}
}

func (jm *JobManager) Add(pid int, cmdArgs []string, flag bool, pipeFlag bool) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	jm.IdLastJob++

	if jm.Jobs.Front() == nil {
		jm.Jobs.PushBack(Job{pid, "Running", cmdArgs, flag, jm.IdLastJob, pipeFlag})
		if flag {
			fmt.Printf("[%d] %d\n", jm.IdLastJob, pid)
		}
	} else {
	loop:
		for e := jm.Jobs.Front(); e != nil; e = e.Next() {
			job := e.Value.(Job)

			switch {
			case flag && job.Status == "Stopped":
				jm.Jobs.InsertBefore(Job{pid, "Running", cmdArgs, flag, jm.IdLastJob, pipeFlag}, e)
				fmt.Printf("[%d] %d\n", jm.IdLastJob, pid)
				break loop
			case flag && e.Next() == nil:
				jm.Jobs.PushBack(Job{pid, "Running", cmdArgs, flag, jm.IdLastJob, pipeFlag})
				fmt.Printf("[%d] %d\n", jm.IdLastJob, pid)
				break loop
			case !flag && job.Background || job.Status == "Stopped":
				jm.Jobs.InsertBefore(Job{pid, "Running", cmdArgs, flag, jm.IdLastJob, pipeFlag}, e)
				break loop
			case !flag && e.Next() == nil:
				jm.Jobs.PushBack(Job{pid, "Running", cmdArgs, flag, jm.IdLastJob, pipeFlag})
				break loop
			}
		}
	}

	if (cmdArgs[0] == "cat" || cmdArgs[0] == "vim") && flag {
		if err := syscall.Kill(jm.PgId(pid), syscall.SIGSTOP); err != nil {
			fmt.Println("Error stopping process")
		}
		jm.jobsMutex.Unlock()
		jm.Update(pid, "Running")
		jm.jobsMutex.Lock()
	}
}

func (jm *JobManager) Write(pid int) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
		job := elem.Value.(Job)

		if pid == job.Pid {
			stat := " "
			if elem.Next() == nil {
				stat = "+"
			} else if elem.Next().Next() == nil {
				stat = "-"
			}

			if job.Background {
				fmt.Printf("[%d]%s    %s    %s &\n", job.Id, stat, job.Status, strings.Join(job.CmdArgs, " "))
			} else if job.Status != "Done" {
				fmt.Printf("[%d]%s    %s    %s\n", job.Id, stat, job.Status, strings.Join(job.CmdArgs, " "))
			}

			if job.Status == "Done" {
				if job.Id == jm.IdLastJob {
					maxId := 0
					for e := jm.Jobs.Front(); e != nil; e = e.Next() {
						if e.Value.(Job).Status != "Done" && maxId < e.Value.(Job).Id {
							maxId = e.Value.(Job).Id
						}
					}
					jm.IdLastJob = maxId
				}
				jm.Jobs.Remove(elem)
			}
			break
		}
	}
}

func (jm *JobManager) PgId(pid int) int {
	for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
		job := elem.Value.(Job)

		if pid == job.Pid {
			if job.PipeFlag {
				return -pid
			}
			break
		}
	}
	return pid
}

func (jm *JobManager) Bg(pid int) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
		job := elem.Value.(Job)

		if job.Pid == pid {
			if job.CmdArgs[0] == "cat" || job.CmdArgs[0] == "vim" {
				if err := syscall.Kill(jm.PgId(pid), syscall.SIGSTOP); err != nil {
					fmt.Println("Error stopping process")
				}
				return
			}

			if err := syscall.Kill(jm.PgId(pid), syscall.SIGCONT); err != nil {
				fmt.Println("Error continuing job")
				return
			}

			job.Background = true
			elem.Value = job

			stat := " "
			if elem.Next() == nil {
				stat = "+"
			} else if elem.Next().Next() == nil {
				stat = "-"
			}

			fmt.Printf("[%d]%s    %s &\n", job.Id, stat, strings.Join(job.CmdArgs, " "))

			jm.jobsMutex.Unlock()
			jm.Update(pid, "Running")
			go jm.WaitForBackground(pid)
			jm.jobsMutex.Lock()

			break
		}
	}
}

func (jm *JobManager) FgWait(pid int) {
	var ws syscall.WaitStatus

	for {
		_, err := syscall.Wait4(jm.PgId(pid), &ws, syscall.WUNTRACED|syscall.WNOHANG, nil)

		switch {
		case ws.Stopped():
			fmt.Println()
			jm.Update(pid, "Stopped")
			jm.Write(pid)
			if err = syscall.Kill(jm.PgId(pid), syscall.SIGSTOP); err != nil {
				fmt.Println()
			}
			return
		case ws.Signaled():
			jm.Update(pid, "Done")
			if err = syscall.Kill(jm.PgId(pid), syscall.SIGINT); err != nil {
				fmt.Println()
				return
			}
		case err != nil:
			jm.Update(pid, "Done")
			return
		}
	}
}

func (jm *JobManager) Fg(pid int) {
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
		job := elem.Value.(Job)

		if job.Pid == pid {
			if err := syscall.Kill(jm.PgId(pid), syscall.SIGCONT); err != nil {
				fmt.Println("Error continuing job")
				return
			}

			job.Background = false
			elem.Value = job

			fmt.Println(strings.Join(job.CmdArgs, " "))

			_ = unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, jm.PgId(pid))

			jm.jobsMutex.Unlock()
			jm.Update(pid, "Running")
			jm.FgWait(pid)
			jm.jobsMutex.Lock()

			shellPgId, _ := syscall.Getpgid(os.Getpid())
			_ = unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, shellPgId)

			break
		}
	}
}

func (jm *JobManager) Update(pid int, status string) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	for elem := jm.Jobs.Front(); elem != nil; elem = elem.Next() {
		job := elem.Value.(Job)

		if pid == job.Pid {
			if (job.CmdArgs[0] == "cat" || job.CmdArgs[0] == "vim") && job.Background && status == "Running" {
				job.Status = "Stopped"
			} else {
				job.Status = status
			}

			elem.Value = job

			if job.Status == "Running" {
				for e := jm.Jobs.Front(); e != nil; e = e.Next() {
					if e.Value.(Job).Background || e.Value.(Job).Status == "Stopped" {
						jm.Jobs.MoveBefore(elem, e)
						break
					} else if e.Next() == nil {
						jm.Jobs.MoveToBack(elem)
						break
					}
				}
			} else if job.Status == "Stopped" {
				jm.Jobs.MoveToBack(elem)
			}

			break
		}
	}
}
