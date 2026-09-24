package cmd

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/assert"
)

func TestCollectorsFinishWithoutTraffic(t *testing.T) {
	for _, mode := range []string{"flows", "packets", "plaintext"} {
		t.Run(mode, func(t *testing.T) {
			t.Chdir(t.TempDir())
			oldTime, oldStartup, oldMax, oldPort := currentTime, startupTime, maxTime, port
			oldName, oldOptions, oldBackground := filename, options, isBackground
			oldKeylog := tlsKeylogPath
			oldEnded, oldStarted, oldCollector, oldStop := captureEnded, captureStarted, collectorStarted, stopReceived
			t.Cleanup(func() {
				currentTime, startupTime, maxTime, port = oldTime, oldStartup, oldMax, oldPort
				filename, options, isBackground = oldName, oldOptions, oldBackground
				tlsKeylogPath = oldKeylog
				captureEnded, captureStarted, collectorStarted, stopReceived = oldEnded, oldStarted, oldCollector, oldStop
			})
			currentTime, startupTime, maxTime, port = time.Now, time.Now(), 100*time.Millisecond, 0
			filename, options, isBackground = "empty", "headless|background=true", true
			captureEnded, captureStarted, collectorStarted, stopReceived = false, false, false, false
			tlsKeylogPath = ""
			if mode == "plaintext" {
				options += "|enable_openssl"
				tlsKeylogPath = filepath.Join(t.TempDir(), "keys.log")
				if err := os.WriteFile(tlsKeylogPath, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				if mode == "flows" {
					startFlowCollector()
				} else {
					startPacketCollector()
				}
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("collector blocked without incoming records")
			}
			assert.True(t, captureEnded)
			if mode == "plaintext" {
				b, err := os.ReadFile(filepath.Join("output", "plaintext", "empty.jsonl"))
				assert.NoError(t, err)
				assert.Empty(t, b)
			}
			if mode == "flows" {
				b, err := os.ReadFile(filepath.Join("output", "flow", "empty.txt"))
				if err != nil {
					t.Fatal(err)
				}
				assert.Empty(t, b)
				db, err := sql.Open("sqlite3", filepath.Join("output", "flow", "empty.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var count int
				if err := db.QueryRow("SELECT count(*) FROM flow").Scan(&count); err != nil {
					t.Fatal(err)
				}
				assert.Zero(t, count)
			} else {
				f, err := os.Open(filepath.Join("output", "pcap", "empty.pcapng"))
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = r.ReadPacketData()
				assert.ErrorIs(t, err, io.EOF)
			}
		})
	}
}
