package collector

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/netobserv/network-observability-cli/internal/collector/cleanup"
)

func printBanner() {
	fmt.Print(`
------------------------------------------------------------------------
         _  _     _       _                       ___ _    ___
        | \| |___| |_ ___| |__ ___ ___ _ ___ __  / __| |  |_ _|
        | .' / -_)  _/ _ \ '_ (_-</ -_) '_\ V / | (__| |__ | |
        |_|\_\___|\__\___/_.__/__/\___|_|  \_/   \___|____|___|

------------------------------------------------------------------------
`)
}

func showKernelVersion() {
	output, err := exec.Command("uname", "-r").Output()
	if err != nil {
		log.Errorf("Can't get kernel version: %v", err)
	}
	if len(output) == 0 {
		log.Debug("Kernel version not found")
		return
	}
	log.Debugf("Kernel version: %s", strings.TrimSpace(string(output)))
}

func onLimitReached() bool {
	if captureEnded {
		return false
	}
	captureEnded = true
	log.Trace("Capture ended")
	if ui.App != nil && ui.err == nil {
		ui.App.Stop()
	}
	if !isBackground {
		return true
	}
	if err := cleanup.DeleteDaemonSet(context.Background(), namespace); err != nil {
		log.Error(err)
	}
	fmt.Print("Thank you for using...")
	printBanner()
	if capture == Metric {
		fmt.Print("\n\n  - Open NetObserv / On Demand dashboard to see generated metrics\n\n\t- Once finished, remove everything using 'oc netobserv cleanup'\n\n                                                      See you soon !\n")
	} else {
		fmt.Print("\n\n\t- Download the generated output using 'oc netobserv copy' command\n\n\t- Once finished, clean the collector pod using 'oc netobserv cleanup'\n\n                                                      See you soon !\n")
	}
	return false
}

func optionEnabled(name string) bool {
	for _, part := range strings.Split(options, "|") {
		part = strings.TrimSpace(strings.TrimPrefix(part, "--"))
		key, value, hasValue := strings.Cut(part, "=")
		if key == name {
			return !hasValue || value != "false"
		}
	}
	return false
}
