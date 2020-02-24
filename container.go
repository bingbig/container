package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/reexec"
)

func init() {
	reexec.Register("nsInitialisation", nsInitialisation)
	if reexec.Init() {
		os.Exit(0)
	}
}

func nsInitialisation() {
	fmt.Printf("\n>> namespace setup code goes here <<\n\n")

	setMount("/root/containerFS")

	if err := waitForNetwork(); err != nil {
		fmt.Printf("Error waiting for network - %s\n", err)
		os.Exit(1)
	}

	if err := syscall.Sethostname([]byte("container")); err != nil {
		fmt.Printf("Error setting hostname - %s\n", err)
		os.Exit(1)
	}

	nsRun()
}

func setMount(root string) error {
	if err := syscall.Chroot(root); err != nil {
		return err
	}
	// 设置容器里面的当前工作目录
	if err := syscall.Chdir("/"); err != nil {
		return err
	}

	if err := syscall.Mount("proc", "proc", "proc", 0, ""); err != nil {
		return err
	}

	return nil
}

func waitForNetwork() error {
	maxWait := time.Second * 3
	checkInterval := time.Second
	timeStarted := time.Now()

	for {
		interfaces, err := net.Interfaces()
		if err != nil {
			return err
		}

		// pretty basic check ...
		// > 1 as a lo device will already exist
		if len(interfaces) > 1 {
			return nil
		}

		if time.Since(timeStarted) > maxWait {
			return fmt.Errorf("Timeout after %s waiting for network", maxWait)
		}

		time.Sleep(checkInterval)
	}
}

func nsRun() {
	cmd := exec.Command(os.Args[1], os.Args[2:]...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = []string{"PS1=-[container]- # "}

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running the %s command - %s\n", os.Args[1], err)
		os.Exit(1)
	}

	syscall.Unmount("/proc", 0)
}

func main() {
	if len(os.Args) < 2 {
		panic("pass me an argument please")
	}

	switch os.Args[1] {
	case "run":
		if len(os.Args) < 3 {
			panic("pass me a cmd to run in container please")
		}
		run()
	default:
		panic("pass me an argument please")
	}
}

func run() {
	cmd := reexec.Command(append([]string{"nsInitialisation"},
		os.Args[2:]...)...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER,
		Unshareflags: syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting the reexec.Command - %s\n", err)
		os.Exit(1)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	netsetgoCmd := exec.Command("/usr/local/bin/netsetgo", "-pid", pid)
	if err := netsetgoCmd.Run(); err != nil {
		fmt.Printf("Error running netsetgo - %s\n", err)
		os.Exit(1)
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("Error running the reexec.Command - %s\n", err)
		os.Exit(1)
	}
}
