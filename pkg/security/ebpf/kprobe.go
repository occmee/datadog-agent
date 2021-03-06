// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// KProbe describes a Linux Kprobe
type KProbe struct {
	Name      string
	EntryFunc string
	ExitFunc  string
}

// RegisterKprobe registers a Kprobe
func (m *Module) RegisterKprobe(k *KProbe) error {
	if k.EntryFunc != "" {
		if err := m.EnableKprobe(k.EntryFunc, 512); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.EntryFunc, err)
		}
	}
	if k.ExitFunc != "" {
		if err := m.EnableKprobe(k.ExitFunc, 512); err != nil {
			return fmt.Errorf("failed to load Kretprobe %v: %s", k.ExitFunc, err)
		}
	}

	return nil
}

// UnregisterKprobe unregisters a Kprobe
func (m *Module) UnregisterKprobe(k *KProbe) error {
	if k.EntryFunc != "" {
		funcName := strings.TrimPrefix(k.EntryFunc, "kprobe/")
		if err := disableKprobe("r" + funcName); err != nil {
			return fmt.Errorf("failed to unregister Kprobe %v: %s", k.EntryFunc, err)
		}
	}
	if k.ExitFunc != "" {
		funcName := strings.TrimPrefix(k.ExitFunc, "kretprobe/")
		if err := disableKprobe("r" + funcName); err != nil {
			return fmt.Errorf("failed to unregister Kprobe %v: %s", k.ExitFunc, err)
		}
	}

	return nil
}

func disableKprobe(eventName string) error {
	kprobeEventsFileName := "/sys/kernel/debug/tracing/kprobe_events"
	f, err := os.OpenFile(kprobeEventsFileName, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("cannot open kprobe_events: %v", err)
	}
	defer f.Close()
	cmd := fmt.Sprintf("-:%s\n", eventName)
	if _, err = f.WriteString(cmd); err != nil {
		pathErr, ok := err.(*os.PathError)
		if ok && pathErr.Err == syscall.ENOENT {
			// This can happen when for example two modules
			// use the same elf object and both call `Close()`.
			// The second will encounter the error as the
			// probe already has been cleared by the first.
		} else {
			return fmt.Errorf("cannot write %q to kprobe_events: %v", cmd, err)
		}
	}
	return nil
}
