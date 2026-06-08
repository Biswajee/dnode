package display

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biswajee/dnode/monitor"
)

const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiGreen = "\033[32m"
	ansiRed   = "\033[31m"
	ansiCyan  = "\033[36m"

	// Maximum recent event lines shown at the bottom.
	maxLogLines = 10

	// How long a newly attached device stays highlighted green.
	attachHighlight = 4 * time.Second

	// How long a removed device stays visible in red before being dropped.
	removeVisible = 2 * time.Second
)

type entry struct {
	evt       monitor.DeviceEvent
	highlight time.Time // zero = no highlight active
	removing  bool
}

type logLine struct {
	t   time.Time
	evt monitor.DeviceEvent
}

// Tree renders a live device tree to the terminal, updating in-place using
// ANSI escape codes. It is safe for concurrent use.
type Tree struct {
	mu        sync.Mutex
	devices   map[string]*entry
	recentLog []logLine
	verbosity int // 1=minimal 2=standard 3=verbose
}

// New initialises the tree display and starts the background highlight ticker.
// verbosity controls how much device detail is shown (1–3).
func New(verbosity int) *Tree {
	if verbosity < 1 {
		verbosity = 1
	} else if verbosity > 3 {
		verbosity = 3
	}
	enableANSI()
	t := &Tree{
		devices:   make(map[string]*entry),
		verbosity: verbosity,
	}
	go t.ticker()
	return t
}

// Update applies a device event to the tree state and redraws.
func (t *Tree) Update(evt monitor.DeviceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := evt.DeviceID
	if evt.Action == monitor.Attached {
		t.devices[key] = &entry{
			evt:       evt,
			highlight: time.Now().Add(attachHighlight),
		}
	} else {
		if e, ok := t.devices[key]; ok {
			e.removing = true
			e.highlight = time.Now().Add(removeVisible)
		}
	}

	t.recentLog = append(t.recentLog, logLine{time.Now(), evt})
	if len(t.recentLog) > maxLogLines {
		t.recentLog = t.recentLog[1:]
	}

	t.redraw()
}

// ticker fires every 250 ms and expires highlights / removes faded devices.
func (t *Tree) ticker() {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		t.mu.Lock()
		changed := false
		for key, e := range t.devices {
			if !e.highlight.IsZero() && time.Now().After(e.highlight) {
				if e.removing {
					delete(t.devices, key)
				} else {
					e.highlight = time.Time{}
				}
				changed = true
			}
		}
		if changed {
			t.redraw()
		}
		t.mu.Unlock()
	}
}

// redraw builds the full screen string and writes it atomically.
// Must be called with t.mu held.
func (t *Tree) redraw() {
	var b strings.Builder

	// Hide cursor, move to top-left, clear screen.
	b.WriteString("\033[?25l\033[H\033[2J")

	// Header.
	b.WriteString("\n  " + ansiBold + "dnode - device monitor" + ansiReset)
	b.WriteString("  " + ansiDim + time.Now().Format("2006-01-02  15:04:05") + ansiReset + "\n")
	b.WriteString("  " + ansiDim + strings.Repeat("─", 64) + ansiReset + "\n\n")

	// Device tree.
	keys := make([]string, 0, len(t.devices))
	for k := range t.devices {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		b.WriteString("  " + ansiDim + "(no devices attached)" + ansiReset + "\n")
	} else {
		for i, key := range keys {
			renderDevice(&b, t.devices[key], i == len(keys)-1, t.verbosity)
		}
	}

	// Recent events footer.
	b.WriteString("\n  " + ansiDim + strings.Repeat("─", 64) + ansiReset + "\n")
	b.WriteString("  " + ansiBold + "recent events" + ansiReset + "\n")
	for _, l := range t.recentLog {
		sym := ansiGreen + "[+]" + ansiReset
		if l.evt.Action == monitor.Removed {
			sym = ansiRed + "[-]" + ansiReset
		}
		name := l.evt.Name
		if name == "" {
			name = l.evt.DeviceID
		}
		if t.verbosity == 1 {
			fmt.Fprintf(&b, "  %s  %s\n", sym, name)
			continue
		}
		line := fmt.Sprintf("  %s  %s  %s  %s",
			ansiDim+l.t.Format("15:04:05")+ansiReset,
			sym,
			string(l.evt.Action),
			name,
		)
		if l.evt.Port != "" {
			line += "  " + ansiCyan + l.evt.Port + ansiReset
		}
		b.WriteString(line + "\n")
	}

	// Show cursor again.
	b.WriteString("\033[?25h")

	fmt.Fprint(os.Stdout, b.String())
}

func renderDevice(b *strings.Builder, e *entry, isLast bool, verbosity int) {
	evt := e.evt
	branch := "  ├── "
	child := "  │   "
	if isLast {
		branch = "  └── "
		child = "      "
	}

	// Choose colour for the device header.
	nameColor := ""
	nameReset := ""
	if !e.highlight.IsZero() {
		if e.removing {
			nameColor = ansiRed
		} else {
			nameColor = ansiGreen
		}
		nameReset = ansiReset
	}

	port := ""
	if evt.Port != "" {
		port = "  " + ansiCyan + evt.Port + ansiReset
	}
	name := evt.Name
	if name == "" {
		name = "(unknown)"
	}

	b.WriteString(ansiDim + branch + ansiReset +
		nameColor + ansiBold + evt.DeviceID + ansiReset +
		"  " + nameColor + name + nameReset +
		port + "\n")

	if verbosity < 2 {
		b.WriteString(ansiDim + child + ansiReset + "\n")
		return
	}

	field := func(key, val, valColor string) {
		if val == "" {
			return
		}
		b.WriteString(ansiDim + child + "│  " + fmt.Sprintf("%-14s", key) + ansiReset)
		b.WriteString(valColor + val + ansiReset + "\n")
	}

	field("manufacturer", evt.Manufacturer, "")
	field("product", evt.Product, "")
	field("serial", evt.SerialNumber, "")
	if evt.Port != "" {
		field("port", evt.Port, ansiCyan)
	}

	if verbosity >= 3 && len(evt.Endpoints) > 0 {
		b.WriteString(ansiDim + child + "│  endpoints" + ansiReset + "\n")
		for j, ep := range evt.Endpoints {
			epBranch := child + "│    ├── "
			if j == len(evt.Endpoints)-1 {
				epBranch = child + "│    └── "
			}
			epStr := fmt.Sprintf("EP0x%02X  %-3s  %-14s  %dB",
				ep.Address, ep.Direction, string(ep.Type), ep.MaxPacket)
			if ep.Interval > 0 {
				epStr += fmt.Sprintf("  interval=%d", ep.Interval)
			}
			b.WriteString(ansiDim + epBranch + ansiReset + epStr + "\n")
		}
	}

	b.WriteString(ansiDim + child + ansiReset + "\n")
}
