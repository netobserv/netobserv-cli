package packet

import (
	"os"
	"sync"

	"github.com/gopacket/gopacket/pcapgo"
)

// Output owns the active pcapng writer and its synchronized shutdown state.
type Output struct {
	mu     sync.Mutex
	writer *pcapgo.NgWriter
	file   *os.File
	stop   func()
}

func NewOutput() *Output { return &Output{} }

func (o *Output) Mutex() *sync.Mutex { return &o.mu }

func (o *Output) SetWriter(writer *pcapgo.NgWriter, file *os.File) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writer = writer
	o.file = file
}

func (o *Output) SetStop(stop func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stop = stop
}

func (o *Output) Clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writer = nil
	o.file = nil
	o.stop = nil
}

func (o *Output) Stop() bool {
	o.mu.Lock()
	stop := o.stop
	o.mu.Unlock()
	if stop == nil {
		return false
	}
	stop()
	return true
}

func (o *Output) Flush() (writerErr, fileErr error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writer != nil {
		writerErr = o.writer.Flush()
	}
	if o.file != nil {
		fileErr = o.file.Sync()
	}
	return writerErr, fileErr
}
