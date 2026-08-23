package sysinfo

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type MemoryInfo struct {
	TotalBytes     uint64
	AvailableBytes uint64
}

func GetMemoryInfo() MemoryInfo {
	info := MemoryInfo{}

	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if fields[0] == "MemTotal:" {
						val, _ := strconv.ParseUint(fields[1], 10, 64)
						info.TotalBytes = val * 1024
					} else if fields[0] == "MemAvailable:" {
						val, _ := strconv.ParseUint(fields[1], 10, 64)
						info.AvailableBytes = val * 1024
					}
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			val, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
			info.TotalBytes = val
			// macOS doesn't easily expose available memory via simple sysctl without parsing vm_stat
			info.AvailableBytes = 0
		}
	}

	return info
}

func HasDocker() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func HasPodman() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}
