package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// PodmanStatsEntry represents an entry from `podman stats --no-stream --format json`
type PodmanStatsEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CPUPercent string `json:"cpu_percent"`
	AvgCPU     string `json:"avg_cpu"`
	MemUsage   string `json:"mem_usage"`
	MemPercent string `json:"mem_percent"`
	NetIO      string `json:"net_io"`
	BlockIO    string `json:"block_io"`
	PIDs       string `json:"pids"`
}

// PodmanDFEntry represents an entry from `podman system df --format json`
type PodmanDFEntry struct {
	Type    string `json:"Type"`
	RawSize int64  `json:"RawSize"`
}

// ContainerInfo stores metadata for an observed container
type ContainerInfo struct {
	ID        string
	Name      string
	Image     string
	ShortName string
	Service   string
}

// ContainerMetrics tracks aggregated and per-container resource metrics over time
type ContainerMetrics struct {
	mu sync.Mutex

	Containers         map[string]*ContainerInfo // keyed by container ID
	PeakContainerMem   map[string]int64          // keyed by container ID
	PeakTotalMemBytes  int64
	PeakTotalCPU       float64

	// Cumulative/latest I/O
	LatestNetRxBytes   int64
	LatestNetTxBytes   int64
	LatestBlockRead    int64
	LatestBlockWrite   int64
	PeakConcurrentPIDs int64
}

func newContainerMetrics() *ContainerMetrics {
	return &ContainerMetrics{
		Containers:       make(map[string]*ContainerInfo),
		PeakContainerMem: make(map[string]int64),
	}
}

func extractImageShortName(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return "container"
	}

	// 1. Remove tag or sha256 digest
	if atIdx := strings.Index(image, "@"); atIdx != -1 {
		image = image[:atIdx]
	}
	lastSlash := strings.LastIndex(image, "/")
	if lastSlash != -1 {
		pathPart := image[lastSlash+1:]
		if tagIdx := strings.Index(pathPart, ":"); tagIdx != -1 {
			image = image[:lastSlash+1+tagIdx]
		}
	} else {
		if tagIdx := strings.Index(image, ":"); tagIdx != -1 {
			image = image[:tagIdx]
		}
	}

	// 2. Extract base name (last path element)
	parts := strings.Split(image, "/")
	base := parts[len(parts)-1]

	// Strip tag if still present
	if tagIdx := strings.Index(base, ":"); tagIdx != -1 {
		base = base[:tagIdx]
	}

	// 3. Normalize common vendor prefixes (e.g. cp-kafka -> kafka)
	if strings.HasPrefix(base, "cp-") {
		base = strings.TrimPrefix(base, "cp-")
	}

	return strings.ToLower(base)
}

func inspectContainer(podmanBin, podmanURL, containerID string) (image, service string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var args []string
	if podmanURL != "" {
		args = append(args, "--url", podmanURL)
	}
	args = append(args, "inspect", containerID, "--format", "{{.Config.Image}}|{{index .Config.Labels \"harness.service\"}}")

	cmd := exec.CommandContext(ctx, podmanBin, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) > 0 {
		image = parts[0]
	}
	if len(parts) > 1 {
		service = parts[1]
	}
	return image, service
}

func (cm *ContainerMetrics) update(entries []PodmanStatsEntry, podmanBin, podmanURL string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var currentTotalMem int64
	var currentTotalCPU float64
	var currentTotalPIDs int64
	var totalNetRx, totalNetTx int64
	var totalBlockRead, totalBlockWrite int64

	for _, e := range entries {
		cid := strings.TrimSpace(e.ID)
		if cid == "" {
			cid = e.Name
		}

		// Ensure container metadata is resolved
		if _, exists := cm.Containers[cid]; !exists {
			img, svc := inspectContainer(podmanBin, podmanURL, cid)
			shortName := extractImageShortName(img)
			if shortName == "container" || shortName == "" {
				shortName = extractImageShortName(e.Name)
			}
			cm.Containers[cid] = &ContainerInfo{
				ID:        cid,
				Name:      e.Name,
				Image:     img,
				ShortName: shortName,
				Service:   svc,
			}
		}

		// Memory usage: format is "4.231MB / 3.793GB"
		parts := strings.Split(e.MemUsage, " / ")
		if len(parts) > 0 {
			memBytes := parseHumanBytes(parts[0])
			currentTotalMem += memBytes
			if memBytes > cm.PeakContainerMem[cid] {
				cm.PeakContainerMem[cid] = memBytes
			}
		}

		// CPU percentage: format is "0.18%"
		cpuStr := strings.TrimSuffix(strings.TrimSpace(e.CPUPercent), "%")
		if cpuVal, err := strconv.ParseFloat(cpuStr, 64); err == nil {
			currentTotalCPU += cpuVal
		}

		// PIDs
		if pids, err := strconv.ParseInt(strings.TrimSpace(e.PIDs), 10, 64); err == nil {
			currentTotalPIDs += pids
		}

		// Network I/O: format "388.5kB / 318.7kB"
		netParts := strings.Split(e.NetIO, " / ")
		if len(netParts) == 2 {
			totalNetRx += parseHumanBytes(netParts[0])
			totalNetTx += parseHumanBytes(netParts[1])
		}

		// Block I/O: format "11.42MB / 56.3MB"
		blockParts := strings.Split(e.BlockIO, " / ")
		if len(blockParts) == 2 {
			totalBlockRead += parseHumanBytes(blockParts[0])
			totalBlockWrite += parseHumanBytes(blockParts[1])
		}
	}

	if currentTotalMem > cm.PeakTotalMemBytes {
		cm.PeakTotalMemBytes = currentTotalMem
	}
	if currentTotalCPU > cm.PeakTotalCPU {
		cm.PeakTotalCPU = currentTotalCPU
	}
	if currentTotalPIDs > cm.PeakConcurrentPIDs {
		cm.PeakConcurrentPIDs = currentTotalPIDs
	}
	if totalNetRx > cm.LatestNetRxBytes {
		cm.LatestNetRxBytes = totalNetRx
	}
	if totalNetTx > cm.LatestNetTxBytes {
		cm.LatestNetTxBytes = totalNetTx
	}
	if totalBlockRead > cm.LatestBlockRead {
		cm.LatestBlockRead = totalBlockRead
	}
	if totalBlockWrite > cm.LatestBlockWrite {
		cm.LatestBlockWrite = totalBlockWrite
	}
}

func parseHumanBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0
	}

	units := []struct {
		suffix string
		factor float64
	}{
		{"GiB", 1024 * 1024 * 1024},
		{"GB", 1000 * 1000 * 1000},
		{"MiB", 1024 * 1024},
		{"MB", 1000 * 1000},
		{"KiB", 1024},
		{"kB", 1000},
		{"KB", 1000},
		{"B", 1},
	}

	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			val, err := strconv.ParseFloat(numStr, 64)
			if err == nil {
				return int64(val * u.factor)
			}
		}
	}

	val, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return int64(val)
	}
	return 0
}

func formatBytes(bytes int64) string {
	if bytes < 0 {
		return "-" + formatBytes(-bytes)
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), units[exp])
}

func formatDuration(d time.Duration) string {
	d = d.Round(10 * time.Millisecond)
	m := d / time.Minute
	s := (d % time.Minute).Seconds()
	if m > 0 {
		return fmt.Sprintf("%dm %.2fs", m, s)
	}
	return fmt.Sprintf("%.2fs", s)
}

func findPodmanBin() string {
	if bin := os.Getenv("PODMAN_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	if bin, err := exec.LookPath("podman"); err == nil {
		return bin
	}
	commonPaths := []string{
		"/opt/podman/bin/podman",
		"/usr/local/bin/podman",
		"/opt/homebrew/bin/podman",
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "podman"
}

func getPodmanURL(podmanBin string) string {
	if ch := os.Getenv("CONTAINER_HOST"); ch != "" {
		return ch
	}
	if dh := os.Getenv("DOCKER_HOST"); dh != "" {
		return dh
	}
	out, err := exec.Command(podmanBin, "machine", "inspect", "--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output()
	if err == nil {
		sock := strings.TrimSpace(string(out))
		if sock != "" {
			return "unix://" + sock
		}
	}
	return ""
}

func fetchPodmanDF(podmanBin, podmanURL string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var args []string
	if podmanURL != "" {
		args = append(args, "--url", podmanURL)
	}
	args = append(args, "system", "df", "--format", "json")

	cmd := exec.CommandContext(ctx, podmanBin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 0, err
	}

	var entries []PodmanDFEntry
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return 0, err
	}

	var totalStorage int64
	for _, e := range entries {
		if e.Type == "Containers" || e.Type == "Local Volumes" || e.Type == "Images" {
			totalStorage += e.RawSize
		}
	}
	return totalStorage, nil
}

func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func getDiskFree(path string) int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		return int64(stat.Bavail) * int64(stat.Bsize)
	}
	return 0
}

func pollPodmanStats(podmanBin, podmanURL string) []PodmanStatsEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var args []string
	if podmanURL != "" {
		args = append(args, "--url", podmanURL)
	}
	args = append(args, "stats", "--no-stream", "--format", "json")

	cmd := exec.CommandContext(ctx, podmanBin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	var entries []PodmanStatsEntry
	_ = json.Unmarshal(stdout.Bytes(), &entries)
	return entries
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test-runner <command> [args...]")
		os.Exit(1)
	}

	targetCmd := os.Args[1]
	targetArgs := os.Args[2:]

	podmanBin := findPodmanBin()
	podmanURL := getPodmanURL(podmanBin)

	// Initial snapshots
	initPodmanStorage, errDF := fetchPodmanDF(podmanBin, podmanURL)
	podmanAvailable := errDF == nil
	initWorkspaceSize, _ := getDirSize(".")
	initDiskFree := getDiskFree(".")

	containerMetrics := newContainerMetrics()

	// Background ticker for container metrics
	stopSampling := make(chan struct{})
	var wg sync.WaitGroup

	if podmanAvailable {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(600 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-stopSampling:
					return
				case <-ticker.C:
					entries := pollPodmanStats(podmanBin, podmanURL)
					if len(entries) > 0 {
						containerMetrics.update(entries, podmanBin, podmanURL)
					}
				}
			}
		}()
	}

	// Prepare child command
	cmd := exec.Command(targetCmd, targetArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Trap signals to guarantee reporting even if terminated
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		close(stopSampling)
		wg.Wait()
		fmt.Fprintf(os.Stderr, "Failed to start command: %v\n", err)
		os.Exit(1)
	}

	var terminated bool
	var termSignal os.Signal

	// Handle signal forwarding
	go func() {
		sig, ok := <-sigChan
		if !ok {
			return
		}
		terminated = true
		termSignal = sig
		if cmd.Process != nil {
			_ = cmd.Process.Signal(sig)
		}
	}()

	// Wait for target command
	waitErr := cmd.Wait()
	duration := time.Since(startTime)

	// Stop sampling
	close(stopSampling)
	signal.Stop(sigChan)
	wg.Wait()

	// Final snapshots
	finalWorkspaceSize, _ := getDirSize(".")
	finalDiskFree := getDiskFree(".")
	finalPodmanStorage, _ := fetchPodmanDF(podmanBin, podmanURL)

	// Determine status and exit code
	exitCode := 0
	statusStr := "PASSED"
	if terminated {
		statusStr = fmt.Sprintf("TERMINATED (%s)", termSignal)
		exitCode = 130
	} else if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		statusStr = fmt.Sprintf("FAILED (exit code %d)", exitCode)
	}

	// Capture child process rusage
	var userCPUTime, sysCPUTime time.Duration
	var peakHostRSS int64
	if cmd.ProcessState != nil {
		userCPUTime = cmd.ProcessState.UserTime()
		sysCPUTime = cmd.ProcessState.SystemTime()
		if rusage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			// On Darwin, Maxrss is in bytes; on Linux, it is in KiB
			if runtime.GOOS == "darwin" {
				peakHostRSS = int64(rusage.Maxrss)
			} else {
				peakHostRSS = int64(rusage.Maxrss) * 1024
			}
		}
	}

	// Print summary report
	printReport(reportData{
		Command:            strings.Join(os.Args[1:], " "),
		Status:             statusStr,
		Duration:           duration,
		PeakHostRSS:        peakHostRSS,
		UserCPUTime:        userCPUTime,
		SysCPUTime:         sysCPUTime,
		PodmanAvailable:    podmanAvailable,
		ContainerMetrics:   containerMetrics,
		StorageDeltaPodman: finalPodmanStorage - initPodmanStorage,
		WorkspaceDelta:     finalWorkspaceSize - initWorkspaceSize,
		DiskFreeDelta:      finalDiskFree - initDiskFree,
	})

	os.Exit(exitCode)
}

type reportData struct {
	Command            string
	Status             string
	Duration           time.Duration
	PeakHostRSS        int64
	UserCPUTime        time.Duration
	SysCPUTime         time.Duration
	PodmanAvailable    bool
	ContainerMetrics   *ContainerMetrics
	StorageDeltaPodman int64
	WorkspaceDelta     int64
	DiskFreeDelta      int64
}

type containerDisplayItem struct {
	DisplayName string
	PeakMemory  int64
}

func printReport(d reportData) {
	width := 80
	sep := strings.Repeat("=", width)
	dash := strings.Repeat("-", width)

	fmt.Println()
	fmt.Println(sep)
	fmt.Printf("%*s\n", (width+36)/2, "TEST RUN RESOURCE CONSUMPTION REPORT")
	fmt.Println(sep)
	fmt.Printf("  Status:            %s\n", d.Status)
	fmt.Printf("  Duration:          %s\n", formatDuration(d.Duration))
	fmt.Printf("  Command:           %s\n", d.Command)

	// Section 1: Host Process
	fmt.Println(dash)
	fmt.Println("  HOST TEST RUNNER (Process Tree)")
	fmt.Printf("    Peak Memory (RSS):       %s\n", formatBytes(d.PeakHostRSS))
	totalHostCPU := d.UserCPUTime + d.SysCPUTime
	cpuPercent := 0.0
	if d.Duration > 0 {
		cpuPercent = (totalHostCPU.Seconds() / d.Duration.Seconds()) * 100
	}
	fmt.Printf("    CPU Time (User / Sys):   %s / %s (Total: %s)\n",
		formatDuration(d.UserCPUTime),
		formatDuration(d.SysCPUTime),
		formatDuration(d.UserCPUTime+d.SysCPUTime),
	)
	fmt.Printf("    Avg Host CPU Usage:      %.1f%%\n", cpuPercent)

	// Section 2: Containers
	fmt.Println(dash)
	fmt.Println("  CONTAINERS (Podman Workloads)")
	if !d.PodmanAvailable {
		fmt.Println("    Podman not available or responsive during this run.")
	} else {
		d.ContainerMetrics.mu.Lock()
		observedCount := len(d.ContainerMetrics.Containers)
		peakTotalMem := d.ContainerMetrics.PeakTotalMemBytes
		peakCPU := d.ContainerMetrics.PeakTotalCPU
		netRx := d.ContainerMetrics.LatestNetRxBytes
		netTx := d.ContainerMetrics.LatestNetTxBytes
		blockR := d.ContainerMetrics.LatestBlockRead
		blockW := d.ContainerMetrics.LatestBlockWrite
		peakPIDs := d.ContainerMetrics.PeakConcurrentPIDs

		// Group containers by ShortName to disambiguate if multiple instances of the same image exist
		shortNameCounts := make(map[string]int)
		for _, info := range d.ContainerMetrics.Containers {
			shortNameCounts[info.ShortName]++
		}

		var items []containerDisplayItem
		var distinctDisplayNames []string

		// Track seen names to assign numeric indices if necessary
		nameIndexTracker := make(map[string]int)

		for cid, info := range d.ContainerMetrics.Containers {
			displayName := info.ShortName
			if shortNameCounts[info.ShortName] > 1 {
				if info.Service != "" && info.Service != info.ShortName {
					displayName = fmt.Sprintf("%s (%s)", info.ShortName, info.Service)
				} else {
					nameIndexTracker[info.ShortName]++
					displayName = fmt.Sprintf("%s (#%d)", info.ShortName, nameIndexTracker[info.ShortName])
				}
			}
			peakMem := d.ContainerMetrics.PeakContainerMem[cid]
			items = append(items, containerDisplayItem{
				DisplayName: displayName,
				PeakMemory:  peakMem,
			})
			distinctDisplayNames = append(distinctDisplayNames, displayName)
		}
		d.ContainerMetrics.mu.Unlock()

		if observedCount == 0 {
			fmt.Println("    No active containers observed during test run.")
		} else {
			sort.Strings(distinctDisplayNames)
			// Sort breakdown by peak memory descending
			sort.Slice(items, func(i, j int) bool {
				return items[i].PeakMemory > items[j].PeakMemory
			})

			fmt.Printf("    Containers Observed:     %d (%s)\n", observedCount, strings.Join(distinctDisplayNames, ", "))
			fmt.Printf("    Peak Total Memory:       %s\n", formatBytes(peakTotalMem))
			for _, item := range items {
				if item.PeakMemory > 0 {
					fmt.Printf("      └─ %-22s %s\n", item.DisplayName+":", formatBytes(item.PeakMemory))
				}
			}
			fmt.Printf("    Peak Container CPU:      %.1f%%\n", peakCPU)
			fmt.Printf("    Peak Concurrent PIDs:    %d\n", peakPIDs)
			fmt.Printf("    Network I/O (RX / TX):   %s / %s\n", formatBytes(netRx), formatBytes(netTx))
			fmt.Printf("    Block I/O (Read / Write): %s / %s\n", formatBytes(blockR), formatBytes(blockW))
		}
	}

	// Section 3: Disk & Storage Delta
	fmt.Println(dash)
	fmt.Println("  DISK & STORAGE DELTA")
	if d.PodmanAvailable {
		podmanDeltaSign := "+"
		if d.StorageDeltaPodman < 0 {
			podmanDeltaSign = ""
		}
		fmt.Printf("    Podman Storage Delta:    %s%s (images, containers, volumes)\n", podmanDeltaSign, formatBytes(d.StorageDeltaPodman))
	}
	wsDeltaSign := "+"
	if d.WorkspaceDelta < 0 {
		wsDeltaSign = ""
	}
	fmt.Printf("    Workspace Delta:         %s%s (reports & local files)\n", wsDeltaSign, formatBytes(d.WorkspaceDelta))

	if d.DiskFreeDelta != 0 {
		freeDeltaSign := "+"
		if d.DiskFreeDelta < 0 {
			freeDeltaSign = ""
		}
		fmt.Printf("    Host Disk Free Delta:    %s%s\n", freeDeltaSign, formatBytes(d.DiskFreeDelta))
	}

	fmt.Println(sep)
	fmt.Println()
}
