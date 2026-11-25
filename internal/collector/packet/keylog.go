package packet

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/sirupsen/logrus"
)

type Keylog struct {
	writer *pcapgo.NgWriter
	mu     *sync.Mutex
	offset int64
}

func NewKeylog(writer *pcapgo.NgWriter, mu *sync.Mutex) *Keylog {
	return &Keylog{writer: writer, mu: mu}
}

func (k *Keylog) Embed(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.writer.WriteDecryptionSecretsBlock(pcapgo.DSB_SECRETS_TYPE_TLS, content); err != nil {
		return err
	}
	k.offset = int64(len(content))
	return nil
}

func (k *Keylog) Watch(ctx context.Context, path string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := k.appendFile(path); err != nil && !os.IsNotExist(err) {
				logrus.StandardLogger().Warnf("failed to append TLS keylog DSB: %v", err)
			}
		}
	}
}

func (k *Keylog) appendFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(k.offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.writer.WriteDecryptionSecretsBlock(pcapgo.DSB_SECRETS_TYPE_TLS, data); err != nil {
		return err
	}
	k.offset += int64(len(data))
	return nil
}

// ParseKeylogLines reads NSS key log format lines and removes comments and blanks.
func ParseKeylogLines(r io.Reader) ([]byte, error) {
	var output strings.Builder
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	return []byte(output.String()), scanner.Err()
}
