package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"

	"integration-suite/pkg/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("ERROR: Failed loading configuration: %v\n", err)
		os.Exit(1)
	}

	podmanBin := cfg.GetPodmanBin()
	if _, err := os.Stat(podmanBin); err != nil {
		if resolved, err := exec.LookPath("podman"); err == nil {
			podmanBin = resolved
		}
	}

	images := cfg.Podman.Images
	if len(images) == 0 {
		fmt.Println("No images defined in configuration.")
		return
	}

	// Sort services for predictable ordering
	var services []string
	for svc := range images {
		services = append(services, svc)
	}
	sort.Strings(services)

	if len(services) == 0 {
		fmt.Println("No public infrastructure images to pull.")
		return
	}

	fmt.Printf(">>> Pre-pulling %d public infrastructure images using [%s]...\n\n", len(services), podmanBin)

	var failed []string
	for _, svc := range services {
		img := images[svc]
		fmt.Printf("=== Pulling [%s]: %s ===\n", svc, img)

		cmd := exec.Command(podmanBin, "pull", img)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("FAILED pulling image [%s]: %v\n\n", img, err)
			failed = append(failed, fmt.Sprintf("%s (%s)", svc, img))
		} else {
			fmt.Printf("SUCCESS [%s]\n\n", img)
		}
	}

	if len(failed) > 0 {
		fmt.Printf("Completed with %d error(s):\n", len(failed))
		for _, f := range failed {
			fmt.Printf(" - %s\n", f)
		}
		os.Exit(1)
	}

	fmt.Println(">>> All configured public infrastructure container images are ready.")
	fmt.Println(">>> NOTE: Local application images (e.g. sort-mistake, sort-service) must be built locally using 'make build-image'.")
}
