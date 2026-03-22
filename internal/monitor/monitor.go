package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/ccmux/ccmux/internal/state"
)

// Monitor polls Claude JSONL transcripts and emits ParsedEntry values.
type Monitor struct {
	cfg          MonitorConfig
	stateMgr     *state.Manager
	fileMtimes   map[string]int64 // session_id → last seen mtime (ns)
	parsers      map[string]*Parser // session_id → parser (preserves pending tool state)
	mu           sync.Mutex
	Entries      chan ParsedEntry
	done         chan struct{}
}

type MonitorConfig struct {
	PollInterval   time.Duration
	SessionMapFile string
	TmuxSession    string
}

func New(cfg MonitorConfig, stateMgr *state.Manager) *Monitor {
	return &Monitor{
		cfg:        cfg,
		stateMgr:   stateMgr,
		fileMtimes: make(map[string]int64),
		parsers:    make(map[string]*Parser),
		Entries:    make(chan ParsedEntry, 100),
		done:       make(chan struct{}),
	}
}

// Start begins the polling loop in a background goroutine.
func (m *Monitor) Start() {
	go m.loop()
}

// Stop signals the polling loop to exit.
func (m *Monitor) Stop() {
	close(m.done)
}

func (m *Monitor) loop() {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) poll() {
	// Re-read session_map to pick up new sessions written by hooks
	if err := m.stateMgr.LoadSessionMap(m.cfg.SessionMapFile, m.cfg.TmuxSession); err != nil {
		log.Warn().Err(err).Msg("loading session map")
	}

	windows := m.stateMgr.AllWindowStates()
	for windowID, ws := range windows {
		if ws.SessionID == "" {
			continue
		}
		// Find the JSONL file for this session
		filePath := m.findJSONLFile(ws.SessionID)
		if filePath == "" {
			continue
		}

		// Check mtime before opening
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		mtime := info.ModTime().UnixNano()

		m.mu.Lock()
		tracked, hasTracked := m.stateMgr.GetTracked(ws.SessionID)
		lastMtime := m.fileMtimes[ws.SessionID]

		// Skip if file unchanged and we have an offset
		if hasTracked && mtime == lastMtime && info.Size() <= tracked.LastByteOffset {
			m.mu.Unlock()
			continue
		}
		log.Debug().Str("session", ws.SessionID[:8]).
			Int64("size", info.Size()).Int64("offset", tracked.LastByteOffset).
			Bool("hasTracked", hasTracked).Msg("reading JSONL")
		m.fileMtimes[ws.SessionID] = mtime

		// Handle truncation (e.g. after /clear)
		if hasTracked && tracked.LastByteOffset > info.Size() {
			tracked.LastByteOffset = 0
			m.stateMgr.SetTracked(ws.SessionID, tracked)
			// Reset parser state
			delete(m.parsers, ws.SessionID)
		}

		parser, ok := m.parsers[ws.SessionID]
		if !ok {
			parser = NewParser()
			m.parsers[ws.SessionID] = parser
		}

		offset := tracked.LastByteOffset
		m.mu.Unlock()

		entries, newOffset := m.readNewEntries(ws.SessionID, filePath, offset, parser)
		log.Debug().Str("session", ws.SessionID[:8]).
			Int("entries", len(entries)).Int64("newOffset", newOffset).Msg("parsed JSONL")

		if newOffset > offset {
			m.stateMgr.SetTracked(ws.SessionID, state.TrackedSession{
				SessionID:      ws.SessionID,
				FilePath:       filePath,
				LastByteOffset: newOffset,
			})
		}

		_ = windowID
		for _, entry := range entries {
			select {
			case m.Entries <- entry:
			case <-m.done:
				return
			}
		}
	}
}

func (m *Monitor) readNewEntries(sessionID, path string, offset int64, parser *Parser) ([]ParsedEntry, int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, offset
	}

	// Only process the new portion
	if int(offset) >= len(data) {
		return nil, offset
	}
	newData := data[offset:]

	var entries []ParsedEntry
	newOffset := offset
	lines := strings.Split(string(newData), "\n")
	for i, line := range lines {
		// Skip trailing empty string from Split when content ends with \n
		// Don't advance offset — the next real line starts right here
		if i == len(lines)-1 && line == "" {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			newOffset += int64(len(lines[i])) + 1
			continue
		}
		// Check if this is a complete JSON line
		if !json.Valid([]byte(line)) {
			// Partial write — stop here, don't advance offset past this line
			break
		}
		parsed := parser.ParseLine(sessionID, []byte(line))
		entries = append(entries, parsed...)
		newOffset += int64(len(lines[i])) + 1
	}
	return entries, newOffset
}

// findJSONLFile locates the JSONL transcript file for a session ID.
// Claude stores sessions under ~/.claude/projects/<hash>/<session-id>.jsonl
func (m *Monitor) findJSONLFile(sessionID string) string {
	// First check if we already know the path
	if ts, ok := m.stateMgr.GetTracked(sessionID); ok && ts.FilePath != "" {
		if _, err := os.Stat(ts.FilePath); err == nil {
			return ts.FilePath
		}
	}

	// Walk ~/.claude/projects looking for the session file
	// The directory structure is: projects/<project-hash>/<session-id>-<session-id>.jsonl
	// or projects/-<path>/<session-id>.jsonl — varies by claude version
	claudeDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	if envDir := os.Getenv("CLAUDE_CONFIG_DIR"); envDir != "" {
		claudeDir = filepath.Join(envDir, "projects")
	}

	var found string
	_ = filepath.Walk(claudeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".jsonl") && strings.Contains(info.Name(), sessionID) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
