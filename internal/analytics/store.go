package analytics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Entry is one recorded command execution, stored as NDJSON.
type Entry struct {
	Cmd      string `json:"c"`
	FilterID string `json:"f"`
	OrigB    int    `json:"o"`
	OutB     int    `json:"b"`
	TS       int64  `json:"t"`
}

func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "tok")
	return dir, os.MkdirAll(dir, 0700)
}

func historyPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.ndjson"), nil
}

func Record(cmd, filterID string, origBytes, outBytes int) {
	path, err := historyPath()
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	e := Entry{Cmd: cmd, FilterID: filterID, OrigB: origBytes, OutB: outBytes, TS: time.Now().Unix()}
	b, _ := json.Marshal(e)
	f.Write(append(b, '\n'))
}

func Load() ([]Entry, error) {
	path, err := historyPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, sc.Err()
}
